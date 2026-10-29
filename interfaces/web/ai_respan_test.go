package web

// ai_respan_test.go — La pasarela de Respan (ai_respan.go) no puede cambiar a
// quién se llama, con qué clave ni con qué modelo.
//
// POR QUÉ SE PRUEBA CON UN SERVIDOR FALSO.
//
// Lo que hay que demostrar no es que la red funcione, sino a DÓNDE va la
// petición y con qué credencial: que con RESPAN_API_KEY la llamada del informe
// salga por la pasarela con esa clave (y sin la del proveedor), y que sin la
// variable —o con ORC24, que es infraestructura propia— todo siga exactamente
// como antes. Eso se ve en la petición que recibe el servidor, no en una traza
// de producción.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// peticionIA es lo que el servidor de prueba alcanza a ver de la llamada.
type peticionIA struct {
	Ruta     string
	Auth     string
	Cuerpo   map[string]interface{}
	Recibido bool
}

// servidorDeChat imita a un proveedor OpenAI-compatible: guarda la petición y
// contesta un informe con su uso de tokens.
func servidorDeChat(t *testing.T, capturada *peticionIA) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturada.Recibido = true
		capturada.Ruta = r.URL.Path
		capturada.Auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &capturada.Cuerpo)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"informe de prueba"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// proveedorDePrueba arma la fila tal como la deja loadAIProviders.
func proveedorDePrueba(vendor, base, clave, modelo string) aiProvider {
	return aiProvider{
		Name:        aiVendorLabel[vendor],
		Vendor:      vendor,
		BaseURL:     base,
		APIKey:      clave,
		Model:       modelo,
		Temperature: 0.7,
		MaxTokens:   200,
	}
}

// TestLaPasarelaTomaLaLlamadaDelInforme: con la clave de Respan, el informe de
// IA sale por la pasarela, autenticado con ella y SIN la clave del proveedor.
// RESPAN_BASE_URL existe para Enterprise; aquí es la forma de apuntar la prueba
// a un servidor local sin tocar el host real.
func TestLaPasarelaTomaLaLlamadaDelInforme(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	var peticion peticionIA
	srv := servidorDeChat(t, &peticion)
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	p := proveedorDePrueba("openai", "https://api.openai.com/v1", "clave-del-proveedor", "gpt-5.5")
	respuesta, uso, err := callChatCompletion(p, "¿cuánto se vendió este mes?")
	if err != nil {
		t.Fatalf("la llamada por la pasarela falló: %v", err)
	}
	if respuesta != "informe de prueba" {
		t.Errorf("la respuesta del modelo no llegó intacta: %q", respuesta)
	}
	if !peticion.Recibido || peticion.Ruta != "/chat/completions" {
		t.Errorf("la pasarela recibe /chat/completions: ruta=%q", peticion.Ruta)
	}
	if peticion.Auth != "Bearer clave-de-pasarela" {
		t.Errorf("la pasarela se autentica con la clave de Respan, no con otra: %q", peticion.Auth)
	}
	if strings.Contains(peticion.Auth, "clave-del-proveedor") {
		t.Errorf("la clave del proveedor no debe viajar a la pasarela: %q", peticion.Auth)
	}
	if modelo, _ := peticion.Cuerpo["model"].(string); modelo != "gpt-5.5" {
		t.Errorf("el modelo del proveedor activo viaja tal cual: %q", modelo)
	}
	if uso.Prompt != 11 || uso.Completion != 7 {
		t.Errorf("el consumo informado por la pasarela se pierde: %+v", uso)
	}
}

// TestSinClaveDeRespanTodoSigueIgual: sin RESPAN_API_KEY la integración queda
// apagada. Es la garantía de que el ERP no puede quedarse sin IA por no tener
// una clave de observabilidad.
func TestSinClaveDeRespanTodoSigueIgual(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "")
	var peticion peticionIA
	srv := servidorDeChat(t, &peticion)

	p := proveedorDePrueba("openai", srv.URL+"/v1", "clave-del-proveedor", "gpt-5.5")
	if _, _, err := callChatCompletion(p, "resumen"); err != nil {
		t.Fatalf("sin clave de Respan la llamada directa al proveedor falló: %v", err)
	}
	if peticion.Ruta != "/v1/chat/completions" {
		t.Errorf("sin clave se llama al proveedor: ruta=%q", peticion.Ruta)
	}
	if peticion.Auth != "Bearer clave-del-proveedor" {
		t.Errorf("sin clave se usa la del proveedor: %q", peticion.Auth)
	}
}

// TestOrc24NoSalePorLaPasarela: ORC24 es infraestructura propia y su modelo no
// existe en Respan. Aunque haya clave, su tráfico se sirve desde casa.
func TestOrc24NoSalePorLaPasarela(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	var peticion peticionIA
	srv := servidorDeChat(t, &peticion)
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	p := proveedorDePrueba("orc24", srv.URL+"/v1", "clave-orc24", "orc24")
	if _, _, err := callChatCompletion(p, "resumen"); err != nil {
		t.Fatalf("la llamada a ORC24 falló: %v", err)
	}
	if peticion.Ruta != "/v1/chat/completions" {
		t.Errorf("ORC24 no pasa por la pasarela: ruta=%q", peticion.Ruta)
	}
	if peticion.Auth != "Bearer clave-orc24" {
		t.Errorf("ORC24 conserva su propia clave: %q", peticion.Auth)
	}
}

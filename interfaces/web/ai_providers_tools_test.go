package web

// ai_providers_tools_test.go — El cliente agéntico (aiCallChatWithTools) no
// puede colar un "éxito" sin texto: el engaño de «Respuesta del agente: ""» en
// /reports/custom/add nacía de devolver content vacío sin error, y de un
// max_tokens 400 —el default del análisis simple— que cortaba el informe
// antes de que el modelo escribiera nada.
//
// En 2026 el modelo puede PENSAR (reasoning_content fuera del texto) y ese
// razonamiento consume presupuesto: si se corta, el bucle repite con más
// holgura hasta que quepa, igual para un modelo pensante que para uno normal.
//
// POR QUÉ SE PRUEBA CON UN SERVIDOR FALSO: igual que ai_respan_test.go, lo que
// hay que demostrar es el CONTRATO del cliente ante respuestas raras y el
// SUELO de tokens que manda a la petición, que se ve en el cuerpo que recibe
// el servidor de prueba.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// respuestaCortada es lo que devuelve un modelo pensante al que se le acabó el
// presupuesto de respuesta ANTES de escribir el informe.
const respuestaCortada = `{"choices":[{"finish_reason":"length","message":{"content":"","reasoning_content":"pensando..."}}],"usage":{"prompt_tokens":10,"completion_tokens":400,"total_tokens":410}}`

// TestElBucleAgenteRepiteConMasHolguraSiElModeloSeCortaPensando: primera
// respuesta cortada → el cliente repite con un presupuesto mayor y trae el
// informe; los tokens de AMBOS intentos llegan al metering.
func TestElBucleAgenteRepiteConMasHolguraSiElModeloSeCortaPensando(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "")
	var presupuestos []float64
	ncalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ncalls++
		raw, _ := io.ReadAll(r.Body)
		var cuerpo struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.Unmarshal(raw, &cuerpo)
		presupuestos = append(presupuestos, float64(cuerpo.MaxTokens))
		w.Header().Set("Content-Type", "application/json")
		if ncalls == 1 {
			_, _ = w.Write([]byte(respuestaCortada))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"informe completo"}}],"usage":{"prompt_tokens":10,"completion_tokens":900,"total_tokens":910}}`))
	}))
	t.Cleanup(srv.Close)

	p := proveedorDePrueba("orc24", srv.URL, "clave", "orc24")
	content, _, uso, err := aiCallChatWithTools(p, "orc24", []map[string]string{{"role": "user", "content": "prueba"}}, "")
	if err != nil {
		t.Fatalf("la repetición con más holgura debía traer el informe: %v", err)
	}
	if content != "informe completo" {
		t.Errorf("el informe no llegó intacto: %q", content)
	}
	if len(presupuestos) != 2 {
		t.Fatalf("debía repetir exactamente una vez, hizo %d peticiones (presupuestos %v)", len(presupuestos), presupuestos)
	}
	if presupuestos[1] <= presupuestos[0] {
		t.Errorf("la repetición debía crecer el presupuesto: %v", presupuestos)
	}
	if uso.Completion != 400+900 {
		t.Errorf("el metering debe sumar TODOS los intentos (400+900), llegó %d", uso.Completion)
	}
}

// TestElBucleAgenteSeRindeConDiagnosticoAlLlegarAlTope: si ni creciendo hasta
// el tope el modelo suelta el informe, el error dice qué ajustar.
func TestElBucleAgenteSeRindeConDiagnosticoAlLlegarAlTope(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "")
	ncalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ncalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respuestaCortada))
	}))
	t.Cleanup(srv.Close)

	p := proveedorDePrueba("orc24", srv.URL, "clave", "orc24")
	content, _, _, err := aiCallChatWithTools(p, "orc24", []map[string]string{{"role": "user", "content": "prueba"}}, "")
	if err == nil {
		t.Fatalf("content vacío tras agotar la holgura debe ser error, llegó content=%q", content)
	}
	if !strings.Contains(err.Error(), "orc24") {
		t.Errorf("el error debe nombrar al proveedor: %v", err)
	}
	if !strings.Contains(err.Error(), "Max Tokens") {
		t.Errorf("el error debe decir qué ajustar en Configuración IA: %v", err)
	}
	if ncalls < 2 {
		t.Errorf("debía intentarlo con más de un presupuesto antes de rendirse, hizo %d", ncalls)
	}
}

// TestElSueloDeTokensDelBucleAgente: un proveedor con los 400/200 de la
// pantalla de Configuración IA no puede cortar el informe; el bucle agéntico
// manda un suelo mayor en la petición.
func TestElSueloDeTokensDelBucleAgente(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "")
	var cuerpo map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &cuerpo)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"informe"}}],"usage":{}}`))
	}))
	t.Cleanup(srv.Close)

	p := proveedorDePrueba("orc24", srv.URL, "clave", "orc24") // MaxTokens: 200
	if _, _, _, err := aiCallChatWithTools(p, "orc24", []map[string]string{{"role": "user", "content": "prueba"}}, ""); err != nil {
		t.Fatalf("la llamada de prueba falló: %v", err)
	}
	maxTokens, _ := cuerpo["max_tokens"].(float64)
	if maxTokens < float64(aiAgenticMinTokens) {
		t.Errorf("el bucle agéntico debe pedir al menos %d tokens, pidió %g", aiAgenticMinTokens, maxTokens)
	}
}

// TestElClienteAgenteSalePorLaMismaPasarela: el informe de IA y la corrida de
// agente iban a destinos distintos (la pasarela y la base URL). Hoy los dos
// clientes hablan con el mismo canal y con la misma clave.
func TestElClienteAgenteSalePorLaMismaPasarela(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	var peticion peticionIA
	srv := servidorDeChat(t, &peticion)
	t.Setenv(aiRespanAPIKeyEnv, "clave-de-pasarela")
	t.Setenv(aiRespanBaseURLEnv, srv.URL)

	p := proveedorDePrueba("openai", "https://api.openai.com/v1", "clave-del-proveedor", "gpt-5.5")
	if _, _, _, err := aiCallChatWithTools(p, "gpt-5.5", []map[string]string{{"role": "user", "content": "prueba"}}, ""); err != nil {
		t.Fatalf("la llamada por la pasarela falló: %v", err)
	}
	if !peticion.Recibido {
		t.Fatal("la llamada del agente nunca llegó a la pasarela")
	}
	if peticion.Auth != "Bearer clave-de-pasarela" {
		t.Errorf("el agente debe salir por la pasarela con su clave, no con la del proveedor: %q", peticion.Auth)
	}
}

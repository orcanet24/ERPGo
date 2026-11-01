package web

// ai_providers_tools_test.go — El cliente agéntico (aiCallChatWithTools) no
// puede colar un "éxito" sin texto: el engaño de «Respuesta del agente: ""» en
// /reports/custom/add nacía de devolver content vacío sin error, y de un
// max_tokens 400 —el default del análisis simple— que cortaba el informe
// antes de que el modelo escribiera nada.
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

// TestElClienteAgenteNoAceptaSilencioDelModelo: content vacío y sin tool calls
// es un ERROR con el nombre del proveedor, no un informe vacío para extraer.
func TestElClienteAgenteNoAceptaSilencioDelModelo(t *testing.T) {
	t.Setenv(aiDNSGuardEnv, "off")
	t.Setenv(aiRespanAPIKeyEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Forma válida OpenAI-compatible pero sin contenido: lo que devuelven
		// algunos modelos cortados a mitad de razonamiento.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"","reasoning_content":"pensando..."}}],"usage":{"prompt_tokens":10,"completion_tokens":400,"total_tokens":410}}`))
	}))
	t.Cleanup(srv.Close)

	p := proveedorDePrueba("orc24", srv.URL, "clave", "orc24")
	content, _, _, err := aiCallChatWithTools(p, "orc24", []map[string]string{{"role": "user", "content": "prueba"}}, "")
	if err == nil {
		t.Fatalf("content vacío sin tool calls debe ser error, llegó content=%q y sin error", content)
	}
	if !strings.Contains(err.Error(), "orc24") || !strings.Contains(err.Error(), "finish_reason length") {
		t.Errorf("el error debe nombrar al proveedor y el finish_reason: %v", err)
	}
}

// TestElSueloDeTokensDelBucleAgente: un proveedor con los 400 de la pantalla
// de Configuración IA no puede cortar el informe; el bucle agéntico manda un
// suelo mayor en la petición.
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

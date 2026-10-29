package web

// ai_respan.go — Ruta de las llamadas de IA por la pasarela alojada de Respan.
//
// POR QUÉ UNA PASARELA Y NO UN SDK DE TRAZABILIDAD.
//
// Respan no publica SDK para Go: sus paquetes de trazabilidad son de Node y de
// Python, y este ERP es Go. Lo que sí publica es una pasarela
// OpenAI-compatible en https://api.respan.ai/api, que es exactamente el dialecto
// que el cliente de IA de este sistema ya habla (`POST /chat/completions` con
// `Authorization: Bearer`). Pasar por ella deja el registro de cada llamada en
// el servidor de Respan sin añadir una dependencia nueva al binario ni cambiar
// de proveedor: el modelo sigue siendo el que el usuario tenga activo.
//
// QUÉ CAMBIA Y QUÉ NO.
//
// Con RESPAN_API_KEY definida, la llamada del informe de IA (`callChatCompletion`,
// la que atiende /ai/analyze) sale por la pasarela y se autentica con ESA clave.
// Sin la variable, todo sigue igual que antes: se habla con el proveedor de
// siempre y con su clave. Una clave de observabilidad que falte no puede dejar
// al ERP sin IA.
//
// La clave vive solo en el entorno del proceso: no se guarda en la BD, no se
// muestra en ninguna pantalla y no se registra en los errores.

import (
	"os"
	"strings"
)

const (
	// aiRespanGatewayBaseURL es la pasarela pública. El cliente le añade
	// /chat/completions, igual que a cualquier proveedor compatible.
	aiRespanGatewayBaseURL = "https://api.respan.ai/api"
	// aiRespanAPIKeyEnv es el secreto del proceso. Vacío = pasarela apagada.
	aiRespanAPIKeyEnv = "RESPAN_API_KEY"
	// aiRespanBaseURLEnv solo hace falta en Respan Enterprise, que sirve la
	// pasarela desde su propio dominio. Se lee del entorno por la misma razón
	// que AI_VENDOR_<VENDOR>_BASE_URL: es un dato del administrador de la
	// máquina, no del usuario de la pantalla.
	aiRespanBaseURLEnv = "RESPAN_BASE_URL"
)

// aiRespanVendors son los proveedores cuyo tráfico puede salir por la pasarela.
//
// ORC24 queda FUERA a propósito. Es infraestructura propia: se responde desde
// casa y sin salir a la red (ver aiVendorModels), su modelo `orc24` no existe en
// la pasarela y enrutarlo la rompería, además de sacar de nuestra facturación
// unas llamadas que son justamente el producto que se vende.
var aiRespanVendors = map[string]bool{
	"openai":    true,
	"anthropic": true,
}

// aiRespanAPIKey devuelve la clave de la pasarela, o "" si no está configurada.
func aiRespanAPIKey() string {
	return strings.TrimSpace(os.Getenv(aiRespanAPIKeyEnv))
}

// aiRespanBaseURL es la pasarela a la que se apunta. RESPAN_BASE_URL manda
// cuando existe (Enterprise); si no, la pública.
func aiRespanBaseURL() string {
	if alterno := strings.TrimSpace(os.Getenv(aiRespanBaseURLEnv)); alterno != "" {
		return strings.TrimRight(alterno, "/")
	}
	return aiRespanGatewayBaseURL
}

// aiRespanEnRuta dice si la llamada de este proveedor tiene que salir por la
// pasarela: hace falta la clave y un proveedor de terceros.
func aiRespanEnRuta(p aiProvider) bool {
	if aiRespanAPIKey() == "" {
		return false
	}
	return aiRespanVendors[aiNormalizeVendor(p.Vendor)]
}

// aiRespanDestino resuelve endpoint y clave de una llamada: la pasarela cuando
// el proveedor va en ruta, o el proveedor de siempre cuando no.
//
// El modelo viaja TAL CUAL. El catálogo curado de OpenAI y Anthropic usa los
// identificadores del propio proveedor (`gpt-5.5`, `claude-sonnet-5-5`), que son
// los mismos que la pasarela enruta; traducirlos aquí sería una segunda tabla
// de nombres que se queda vieja sin que nadie lo note.
//
// La validación de DNS de aiGuardedClient se aplica igual al host de la
// pasarela: no se abre una puerta de red por integrar observabilidad.
func aiRespanDestino(p aiProvider) (endpoint, apiKey string, viaRespan bool) {
	if aiRespanEnRuta(p) {
		return aiCompletionsEndpoint(aiRespanBaseURL()), aiRespanAPIKey(), true
	}
	return aiCompletionsEndpoint(strings.TrimSpace(p.BaseURL)), strings.TrimSpace(p.APIKey), false
}

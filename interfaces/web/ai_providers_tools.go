package web

// ai_providers_tools.go — Cliente chat-completions con tool calling para los
// agentes. La URL base SIEMPRE viene de aiVendorURL (whitelist compilada);
// la API key viaja solo en el header Authorization.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// aiAgenticMinTokens es el suelo de completion tokens del bucle agéntico:
// un informe JSON estructurado más columnas y totales no cabe en los 400
// tokens que trae por defecto la pantalla de Configuración IA (eso es el
// valor pensado para el análisis simple).
const aiAgenticMinTokens = 2000

// aiChatToolCall es una llamada a herramienta propuesta por el modelo.
type aiChatToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type aiChatRequest struct {
	Model       string              `json:"model"`
	Messages    []map[string]string `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Tools       json.RawMessage     `json:"tools,omitempty"`
}

type aiChatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
			// Razonamiento que algunos modelos/motorizadores devuelven por
			// fuera del content (p.ej. `reasoning_content`). Solo sirve para
			// diagnosticar: el texto del informe SIEMPRE es `content`.
			Reasoning string `json:"reasoning_content"`
			Refusal   string `json:"refusal"`
		} `json:"message"`
		// FinishReason guarda "stop" | "length" | "tool_calls"… según el
		// proveedor. Sin él, un content vacío es indistinguible de un modelo
		// que paró a propósito de uno que se cortó a mitad de razonamiento.
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	// Usage es opcional en un vendor OpenAI-compatible: si no viene, el
	// metering registra tokens 0 en vez de estimar una cifra.
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error   json.RawMessage `json:"error"`
	Message string          `json:"message"`
}

// aiTruncateLog acota un cuerpo HTTP para el log de servidor: la pista de
// diagnóstico sin convertir el log en volcado entero.
func aiTruncateLog(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 600 {
		return s[:600] + "…"
	}
	return s
}

// aiCallChatWithTools envía la conversación + catálogo de herramientas y
// devuelve el contenido y/o las llamadas a herramientas del modelo, junto con
// los tokens reportados por el proveedor (para el metering).
//
// El destino es EL MISMO que el del cliente simple (callChatCompletion):
// aiRespanDestino, la pasarela cuando toca y el proveedor cuando no. Los dos
// clientes iban a destinos distintos —este iba directo a la base URL y esa
// divergencia significaba que el mismo proveedor funcionaba en un lado y no
// en el otro—.
func aiCallChatWithTools(p aiProvider, model string, messages []map[string]string, toolsJSON string) (string, []aiChatToolCall, aiTokenUsage, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = strings.TrimSpace(p.Model)
	}
	if model == "" {
		model = aiModelPorDefecto(p.Vendor)
	}
	// Suelo de la corrida agéntica ANTES de tocar el valor del proveedor: el
	// 400 que trae por defecto la pantalla de Configuración IA sirve para el
	// análisis simple (/ai/analyze), no para un bucle con herramientas que
	// debe entregar un informe JSON completo. Con 400, un modelo que razona
	// antes de escribir se cortaba ANTES del texto y la corrida terminaba con
	// content vacío («Respuesta del agente: ""»): el mismo proveedor, la
	// culpa era de la gestión, no de él.
	maxTokens := p.MaxTokens
	if maxTokens < aiAgenticMinTokens {
		maxTokens = aiAgenticMinTokens
	}
	reqBody := aiChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: aiRoundTemperature(p.Temperature),
		MaxTokens:   maxTokens,
	}
	if strings.TrimSpace(toolsJSON) != "" && toolsJSON != "[]" {
		reqBody.Tools = json.RawMessage(toolsJSON)
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, aiTokenUsage{}, fmt.Errorf("no se pudo preparar la petición: %v", err)
	}
	endpoint, apiKey, _ := aiRespanDestino(p)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, aiTokenUsage{}, fmt.Errorf("no se pudo preparar la petición: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client, err := aiGuardedClient(endpoint, aiTimeout)
	if err != nil {
		return "", nil, aiTokenUsage{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, aiTokenUsage{}, fmt.Errorf("error de red al consultar %q: %v", p.Name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, aiMaxBody))
	if err != nil {
		return "", nil, aiTokenUsage{}, fmt.Errorf("no se pudo leer la respuesta de %q: %v", p.Name, err)
	}
	var parsed aiChatResponse
	_ = json.Unmarshal(raw, &parsed)

	usage := aiTokenUsage{
		Prompt:     parsed.Usage.PromptTokens,
		Completion: parsed.Usage.CompletionTokens,
		Model:      model,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, usage, fmt.Errorf("el proveedor %q devolvió un error (HTTP %d): %s",
			p.Name, resp.StatusCode, aiErrorText(raw, openAIResponse{Error: parsed.Error, Message: parsed.Message}))
	}
	if len(parsed.Choices) == 0 {
		return "", nil, usage, fmt.Errorf("el proveedor %q devolvió una respuesta vacía", p.Name)
	}
	msg := parsed.Choices[0].Message
	calls := make([]aiChatToolCall, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		calls = append(calls, aiChatToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	// Un content vacío y sin tool calls NO es un informe final: es el modelo
	// que no dijo nada (se cortó en el razonamiento, la pasarela devolvió
	// content null, etc.). Devolverlo "con éxito" producía el engaño de
	// «Respuesta del agente: ""» a quien usa la IA, sin decirle ni proveedor
	// ni porqué. Aquí se cuenta la verdad del lado del que la puede usar.
	if strings.TrimSpace(msg.Content) == "" && len(calls) == 0 {
		detalle := "sin texto y sin llamadas a herramientas"
		if f := strings.TrimSpace(parsed.Choices[0].FinishReason); f != "" && f != "stop" {
			detalle += ", finish_reason " + f
		}
		if strings.TrimSpace(msg.Reasoning) != "" {
			detalle += " (el modelo escribió razonamiento fuera del texto final)"
		}
		if f := strings.TrimSpace(msg.Refusal); f != "" {
			detalle += "; el proveedor dijo rechazar la petición: " + aiTrimError(f)
		}
		// El cuerpo crudo al log de servidor: ahí queda la pista exacta de qué
		// forma habló este proveedor sin mostrarlo al usuario de la pantalla.
		log.Printf("aiCallChatWithTools: proveedor %q devolvió cuerpo %s",
			p.Name, aiTruncateLog(string(raw)))
		return "", nil, usage, fmt.Errorf(
			"el proveedor %q respondió con la llamada OK pero %s (modelo %s)",
			p.Name, detalle, model)
	}
	return msg.Content, calls, usage, nil
}

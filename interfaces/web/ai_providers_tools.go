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

// Suelo y tope del presupuesto de respuesta del bucle agéntico.
//
// El modelo QUE PIENSE O QUE NO no depende de nosotros: en 2026 los modelos
// que razonan son la norma y ese razonamiento llega por fuera del texto y
// consume parte del PRESUPUESTO de respuesta (finish_reason length antes de
// escribir el informe). El cliente no puede exigirles pensar menos: lo que
// hace es darles holgura y, si se cortan, REPETIR la petición con un
// presupuesto mayor hasta que el razonamiento quepa. Al pagar es igual: se
// cobran los tokens consumidos de verdad, el tope no es una factura.
const (
	aiAgenticMinTokens = 2000
	aiAgenticMaxTokens = 32768
)

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
			// Razonamiento que los modelos pensantes devuelven por fuera del
			// content (`reasoning_content`). Con finish_reason "length" y
			// content vacío significa que el presupuesto se lo comió el
			// pensar: hay que repetir con más holgura, no rendirse.
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

// aiChatEstado dice qué pasó con la respuesta de un intento, para que el
// bucle decida repetir o diagnosticar sin improvisar fuera.
type aiChatEstado struct {
	Cut       bool // se cortó por presupuesto (finish_reason=length) sin texto
	Reasoning string
	Refusal   string
	Cuerpo    string // crudo de la respuesta, para el log de diagnóstico
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
// los tokens reportados por el proveedor (para el metering), sumados de TODOS
// los intentos porque el proveedor cobra cada llamada.
//
// EL DESTINO: el mismo que el del cliente simple (callChatCompletion) —
// aiRespanDestino, la pasarela cuando toca y el proveedor cuando no. Visto con
// dos destinos que funcionaban de forma distinta con el mismo proveedor.
func aiCallChatWithTools(p aiProvider, model string, messages []map[string]string, toolsJSON string, onProgress ...func(string)) (string, []aiChatToolCall, aiTokenUsage, error) {
	var progress func(string)
	if len(onProgress) > 0 && onProgress[0] != nil {
		progress = onProgress[0]
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = strings.TrimSpace(p.Model)
	}
	if model == "" {
		model = aiModelPorDefecto(p.Vendor)
	}
	// Presupuesto inicial: el que configure el proveedor siempre manda —si le
	// puso más que el suelo, se usa el suyo—; el suelo (+2000) evita heredar
	// el 400 pensado para el análisis simple (/ai/analyze). El bucle SOLO lo
	// amplía si el modelo se corta pensando, nunca lo reduce.
	presupuesto := p.MaxTokens
	if presupuesto < aiAgenticMinTokens {
		presupuesto = aiAgenticMinTokens
	}

	usoTotal := aiTokenUsage{Model: model}
	for intento := 1; ; intento++ {
		content, calls, usage, estado, err := aiPostChatWithTools(p, model, messages, toolsJSON, presupuesto)
		usoTotal.Prompt += usage.Prompt
		usoTotal.Completion += usage.Completion
		if err != nil {
			return "", nil, usoTotal, err
		}
		if len(calls) > 0 || strings.TrimSpace(content) != "" {
			return content, calls, usoTotal, nil
		}
		// Sin texto y sin llamadas: el modelo no dijo nada. Si se cortó
		// pensando y todavía hay margen, repetir con un presupuesto mayor —
		// así trabaja igual un modelo que razona que uno que no—. Si no,
		// contar la verdad con la pista del que la puede arreglar.
		if estado.Cut && presupuesto < aiAgenticMaxTokens {
			nuevo := presupuesto * 4
			if nuevo > aiAgenticMaxTokens {
				nuevo = aiAgenticMaxTokens
			}
			if progress != nil {
				progress(fmt.Sprintf("La IA se cortó pensando; reintentando con más tokens (%d)...", nuevo))
			}
			log.Printf("aiCallChatWithTools: el modelo %s de %q se cortó pensando; repitiendo con %d tokens de respuesta",
				model, p.Name, nuevo)
			presupuesto = nuevo
			continue
		}
		if estado.Refusal != "" {
			return "", nil, usoTotal, fmt.Errorf(
				"el proveedor %q rechazó la petición (modelo %s): %s",
				p.Name, model, aiTrimError(estado.Refusal))
		}
		log.Printf("aiCallChatWithTools: proveedor %q devolvió cuerpo %s",
			p.Name, aiTruncateLog(estado.Cuerpo))
		if estado.Cut {
			return "", nil, usoTotal, fmt.Errorf(
				"el proveedor %q (modelo %s) se cortó por presupuesto (finish_reason length) incluso con %d tokens de respuesta. si el modelo piensa antes de responder, sube Max Tokens de este proveedor en Configuración IA",
				p.Name, model, presupuesto)
		}
		detalle := "sin texto y sin llamadas a herramientas"
		if estado.Reasoning != "" {
			detalle += " (el modelo escribió razonamiento pero no texto final)"
		}
		return "", nil, usoTotal, fmt.Errorf(
			"el proveedor %q (modelo %s) respondió OK pero %s",
			p.Name, model, detalle)
	}
}

// aiPostChatWithTools hace UNA petición al endpoint y traduce su respuesta.
// El bucle de aiCallChatWithTools decide repetir; aquí no hay decisiones.
func aiPostChatWithTools(p aiProvider, model string, messages []map[string]string, toolsJSON string, maxTokens int) (string, []aiChatToolCall, aiTokenUsage, aiChatEstado, error) {
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
		return "", nil, aiTokenUsage{}, aiChatEstado{}, fmt.Errorf("no se pudo preparar la petición: %v", err)
	}
	endpoint, apiKey, _ := aiRespanDestino(p)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, aiTokenUsage{}, aiChatEstado{}, fmt.Errorf("no se pudo preparar la petición: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client, err := aiGuardedClient(endpoint, aiTimeout)
	if err != nil {
		return "", nil, aiTokenUsage{}, aiChatEstado{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, aiTokenUsage{}, aiChatEstado{}, fmt.Errorf("error de red al consultar %q: %v", p.Name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, aiMaxBody))
	if err != nil {
		return "", nil, aiTokenUsage{}, aiChatEstado{}, fmt.Errorf("no se pudo leer la respuesta de %q: %v", p.Name, err)
	}
	var parsed aiChatResponse
	_ = json.Unmarshal(raw, &parsed)

	usage := aiTokenUsage{
		Prompt:     parsed.Usage.PromptTokens,
		Completion: parsed.Usage.CompletionTokens,
		Model:      model,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, usage, aiChatEstado{}, fmt.Errorf("el proveedor %q devolvió un error (HTTP %d): %s",
			p.Name, resp.StatusCode, aiErrorText(raw, openAIResponse{Error: parsed.Error, Message: parsed.Message}))
	}
	if len(parsed.Choices) == 0 {
		return "", nil, usage, aiChatEstado{}, fmt.Errorf("el proveedor %q devolvió una respuesta vacía", p.Name)
	}
	msg := parsed.Choices[0].Message
	calls := make([]aiChatToolCall, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		calls = append(calls, aiChatToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	estado := aiChatEstado{
		Cut:       strings.EqualFold(parsed.Choices[0].FinishReason, "length") && strings.TrimSpace(msg.Content) == "" && len(calls) == 0,
		Reasoning: msg.Reasoning,
		Refusal:   msg.Refusal,
		Cuerpo:    string(raw),
	}
	// Un content vacío y sin tool calls no es un informe: el caller decide
	// repetir o propagar el diagnóstico; el cuerpo crudo viaja en el estado
	// para el log sin esconder la forma en que habló este proveedor.
	return msg.Content, calls, usage, estado, nil
}

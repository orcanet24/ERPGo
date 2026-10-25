package web

// ai_providers_tools.go — Cliente chat-completions con tool calling para los
// agentes. La URL base SIEMPRE viene de aiVendorURL (whitelist compilada);
// la API key viaja solo en el header Authorization.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		} `json:"message"`
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

// aiCallChatWithTools envía la conversación + catálogo de herramientas y
// devuelve el contenido y/o las llamadas a herramientas del modelo, junto con
// los tokens reportados por el proveedor (para el metering).
func aiCallChatWithTools(p aiProvider, model string, messages []map[string]string, toolsJSON string) (string, []aiChatToolCall, aiTokenUsage, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = strings.TrimSpace(p.Model)
	}
	if model == "" {
		model = aiDefaultModel
	}
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
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
	endpoint := aiCompletionsEndpoint(p.BaseURL)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, aiTokenUsage{}, fmt.Errorf("no se pudo preparar la petición: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(p.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
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
	return msg.Content, calls, usage, nil
}

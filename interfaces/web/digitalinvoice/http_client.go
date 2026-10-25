package digitalinvoice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient es el cliente HTTP reutilizable para todas las APIs de facturación digital.
// Centraliza manejo de autenticación, timeouts y reconexiones.
type HTTPClient struct {
	baseURL      string
	bearerToken  string
	httpClient   *http.Client
}

// NuevoHTTPClient crea un cliente para una API de facturación digital.
func NuevoHTTPClient(baseURL, bearerToken string) *HTTPClient {
	return &HTTPClient{
		baseURL:     baseURL,
		bearerToken: bearerToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Do ejecuta una petición HTTP con autenticación Bearer.
func (c *HTTPClient) Do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("error marshalizando body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("error creando request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error en request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error leyendo respuesta: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// DoJSON ejecuta la petición y deserializa la respuesta JSON.
func (c *HTTPClient) DoJSON(ctx context.Context, method, path string, body interface{}, target interface{}) (int, error) {
	respBody, statusCode, err := c.Do(ctx, method, path, body)
	if err != nil {
		return statusCode, err
	}

	if err := json.Unmarshal(respBody, target); err != nil {
		return statusCode, fmt.Errorf("error deserializando respuesta (HTTP %d): %w", statusCode, err)
	}

	return statusCode, nil
}

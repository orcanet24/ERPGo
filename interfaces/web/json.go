package web

import (
	"encoding/json"
	"net/http"
)

// mustJSON marshals without error handling (nil on failure).
func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// jsonDecode decodes a JSON request body into v.
func jsonDecode(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// Package http carries response helpers (envelope writers). Mirrors
// lib/http/response.ts in the Node services.
package http

import (
	"encoding/json"
	"net/http"
)

// SendSuccess writes the standard {success:true, data:...} envelope.
func SendSuccess(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    data,
	})
}

// SendPaginated wraps a list with cursor metadata.
func SendPaginated(w http.ResponseWriter, data any, nextCursor string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    data,
		"meta": map[string]any{
			"nextCursor": nextCursor,
		},
	})
}

// Package api exposes the agent's HTTP API.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type apiError struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, detail string) {
	writeJSON(w, status, apiError{Error: code, Detail: detail})
}

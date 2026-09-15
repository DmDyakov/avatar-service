package handlers

import (
	"encoding/json"
	"net/http"
)

// errorResponse — стандартный ответ с ошибкой.
type errorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// respondJSON отправляет JSON-ответ.
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		_ = err
	}
}

// respondError отправляет JSON-ответ с ошибкой.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, errorResponse{Error: message})
}

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

// RespondJSON отправляет JSON-ответ.
func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		_ = err
	}
}

// RespondError отправляет JSON-ответ с ошибкой.
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, errorResponse{Error: message})
}

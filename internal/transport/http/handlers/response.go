package handlers

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
	MaxSize int64  `json:"max_size,omitempty"`
}

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

func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, errorResponse{Error: message})
}

func RespondErrorWithDetails(w http.ResponseWriter, status int, message, details string) {
	RespondJSON(w, status, errorResponse{Error: message, Details: details})
}

func RespondErrorFileTooLarge(w http.ResponseWriter, maxSize int64) {
	RespondJSON(w, http.StatusRequestEntityTooLarge, errorResponse{
		Error:   "File too large",
		MaxSize: maxSize,
	})
}

// Package handlers содержит HTTP-обработчики запросов.
package handlers

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"avatar-service/internal/domain"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type AvatarService interface {
	Upload(ctx context.Context, userID string, file multipart.File, header *multipart.FileHeader) (*domain.Avatar, error)
	GetByID(ctx context.Context, id string) (*domain.Avatar, io.ReadCloser, error)
	GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, io.ReadCloser, error)
	GetMetadata(ctx context.Context, id string) (*domain.Avatar, error)
	ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error)
	Delete(ctx context.Context, id, userID string) error
}

// AvatarHandler обрабатывает HTTP-запросы, связанные с аватарками.
type AvatarHandler struct {
	service AvatarService
	logger  *zap.Logger
}

// NewAvatarHandler создаёт handler аватарок.
func NewAvatarHandler(service AvatarService, logger *zap.Logger) *AvatarHandler {
	return &AvatarHandler{
		service: service,
		logger:  logger,
	}
}

// Upload обрабатывает POST /api/v1/avatars.
func (h *AvatarHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "X-User-ID header is required")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "invalid multipart form or file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	avatar, err := h.service.Upload(r.Context(), userID, file, header)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, avatar)
}

// Get обрабатывает GET /api/v1/avatars/{id}.
func (h *AvatarHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	avatar, reader, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", avatar.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("failed to stream avatar", zap.Error(err))
	}
}

// GetUserAvatar обрабатывает GET /api/v1/users/{user_id}/avatar.
func (h *AvatarHandler) GetUserAvatar(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	avatar, reader, err := h.service.GetUserAvatar(r.Context(), userID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", avatar.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("failed to stream avatar", zap.Error(err))
	}
}

// GetMetadata обрабатывает GET /api/v1/avatars/{id}/metadata.
func (h *AvatarHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	avatar, err := h.service.GetMetadata(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, avatar)
}

// ListByUserID обрабатывает GET /api/v1/users/{user_id}/avatars.
func (h *AvatarHandler) ListByUserID(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	avatars, err := h.service.ListByUserID(r.Context(), userID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, avatars)
}

// Delete обрабатывает DELETE /api/v1/avatars/{id}.
func (h *AvatarHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "X-User-ID header is required")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := h.service.Delete(r.Context(), id, userID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleError преобразует доменные ошибки в HTTP-ответы.
func (h *AvatarHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		respondError(w, http.StatusNotFound, "avatar not found")
	case errors.Is(err, domain.ErrForbidden):
		respondError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrInvalidFormat):
		respondError(w, http.StatusBadRequest, "invalid file format")
	case errors.Is(err, domain.ErrFileTooLarge):
		respondError(w, http.StatusRequestEntityTooLarge, "file too large")
	case errors.Is(err, domain.ErrInvalidInput):
		respondError(w, http.StatusBadRequest, "invalid input")
	default:
		h.logger.Error("internal error", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "internal server error")
	}
}

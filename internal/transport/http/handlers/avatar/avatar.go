// Package avatar содержит HTTP-обработчики запросов для работы с аватарками.
package avatar

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"

	"avatar-service/internal/domain"
	"avatar-service/internal/transport/http/handlers"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

var userIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

const (
	maxUserIDLen        = 255
	supportedFormatsMsg = "Supported formats: jpeg, png, webp"
)

//go:generate mockgen -source=avatar.go -destination=mocks/mock_avatar_service.go -package=mocks
type AvatarService interface {
	Upload(ctx context.Context, userID string, file multipart.File, header *multipart.FileHeader) (*domain.Avatar, error)
	GetByID(ctx context.Context, id string) (*domain.Avatar, io.ReadCloser, error)
}

type AvatarHandler struct {
	service AvatarService
	logger  *zap.Logger
}

func NewAvatarHandler(service AvatarService, logger *zap.Logger) *AvatarHandler {
	return &AvatarHandler{service: service, logger: logger}
}

// Upload обрабатывает POST /api/v1/avatars.
func (h *AvatarHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if err := validateUserID(userID); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxFileSize)

	if err := r.ParseMultipartForm(domain.MaxFileSize); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			handlers.RespondErrorFileTooLarge(w, domain.MaxFileSize)
			return
		}
		handlers.RespondError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	avatar, err := h.service.Upload(r.Context(), userID, file, header)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, avatar)
}

// Get обрабатывает GET /api/v1/avatars/{id}.
func (h *AvatarHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlers.RespondError(w, http.StatusBadRequest, "id is required")
		return
	}

	avatar, reader, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", avatar.MimeType)
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("failed to stream avatar",
			zap.String("avatar_id", avatar.ID),
			zap.Error(err),
		)
	}
}

// handleError преобразует доменные ошибки в HTTP-ответы.
func (h *AvatarHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		handlers.RespondError(w, http.StatusNotFound, "avatar not found")
	case errors.Is(err, domain.ErrForbidden):
		handlers.RespondErrorWithDetails(w, http.StatusForbidden,
			"Forbidden", "You can only delete your own avatars")
	case errors.Is(err, domain.ErrInvalidFormat):
		handlers.RespondErrorWithDetails(w, http.StatusBadRequest,
			"Invalid file format", supportedFormatsMsg)
	case errors.Is(err, domain.ErrFileTooLarge):
		handlers.RespondErrorFileTooLarge(w, domain.MaxFileSize)
	case errors.Is(err, domain.ErrInvalidInput):
		handlers.RespondError(w, http.StatusBadRequest, "invalid input")
	default:
		h.logger.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func validateUserID(userID string) error {
	if userID == "" {
		return errors.New("X-User-ID header is required")
	}
	if len(userID) > maxUserIDLen {
		return errors.New("X-User-ID is too long (max 255)")
	}
	if !userIDPattern.MatchString(userID) {
		return errors.New("X-User-ID contains invalid characters (allowed: a-z, A-Z, 0-9, _, -)")
	}
	return nil
}

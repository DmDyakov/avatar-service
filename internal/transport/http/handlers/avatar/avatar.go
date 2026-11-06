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
	GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, io.ReadCloser, error)
	GetMetadata(ctx context.Context, id string) (*domain.Avatar, error) // ← добавили
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

// PostUpload обрабатывает POST /web/upload (AS-111).
//
// Гибридный вариант: форма отправляется нативно (если JS отключён),
// сервер делает 303 See Other на галерею. Если JS работает,
// форма перехватывает submit и использует /api/v1/avatars.
func (h *AvatarHandler) PostUpload(w http.ResponseWriter, r *http.Request) {
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

	userID := r.FormValue("user_id")
	if err := validateUserID(userID); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	_, err = h.service.Upload(r.Context(), userID, file, header)
	if err != nil {
		h.handleError(w, err)
		return
	}

	http.Redirect(w, r, "/web/gallery/"+userID, http.StatusSeeOther)
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

	h.streamAvatar(w, avatar, reader)
}

// GetUserAvatar обрабатывает GET /api/v1/users/{user_id}/avatar.
func (h *AvatarHandler) GetUserAvatar(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if err := validateUserID(userID); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	avatar, reader, err := h.service.GetUserAvatar(r.Context(), userID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	h.streamAvatar(w, avatar, reader)
}

// GetMetadata обрабатывает GET /api/v1/avatars/{id}/metadata
func (h *AvatarHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlers.RespondError(w, http.StatusBadRequest, "id is required")
		return
	}

	avatar, err := h.service.GetMetadata(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, toMetadataResponse(avatar))
}

// streamAvatar отдаёт файл аватарки в response.
func (h *AvatarHandler) streamAvatar(w http.ResponseWriter, avatar *domain.Avatar, reader io.ReadCloser) {
	defer reader.Close()

	w.Header().Set("Content-Type", avatar.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, reader); err != nil {
		h.logger.Error("failed to stream avatar",
			zap.String("avatar_id", avatar.ID),
			zap.String("user_id", avatar.UserID),
			zap.Error(err),
		)
	}
}

// handleError преобразует доменные ошибки в HTTP-ответы.
func (h *AvatarHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		handlers.RespondError(w, http.StatusNotFound, "Avatar not found")
	case errors.Is(err, domain.ErrStorageUnavailable):
		h.logger.Error("storage unavailable", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
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

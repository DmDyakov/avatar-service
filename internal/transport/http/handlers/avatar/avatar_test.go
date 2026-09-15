package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"avatar-service/internal/domain"
	"avatar-service/internal/transport/http/handlers/avatar/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func setupTest(t *testing.T) (*mocks.MockAvatarService, http.Handler) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockSvc := mocks.NewMockAvatarService(ctrl)
	handler := NewAvatarHandler(mockSvc, zap.NewNop())

	router := chi.NewRouter()
	router.Post("/api/v1/avatars", handler.Upload)
	router.Get("/api/v1/avatars/{id}", handler.Get)
	router.Get("/api/v1/avatars/{id}/metadata", handler.GetMetadata)
	router.Delete("/api/v1/avatars/{id}", handler.Delete)
	router.Get("/api/v1/users/{user_id}/avatar", handler.GetUserAvatar)
	router.Get("/api/v1/users/{user_id}/avatars", handler.ListByUserID)

	return mockSvc, router
}

func createMultipartBody(t *testing.T, fieldName, fileName string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}

func TestAvatarHandler_Upload(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			Upload(gomock.Any(), "user-123", gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, userID string, file multipart.File, header *multipart.FileHeader) (*domain.Avatar, error) {
				assert.Equal(t, "avatar.jpg", header.Filename)
				return &domain.Avatar{ID: "avatar-1", UserID: userID}, nil
			})

		body, contentType := createMultipartBody(t, "file", "avatar.jpg", []byte("fake image"))

		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-User-ID", "user-123")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var response domain.Avatar
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
		assert.Equal(t, "avatar-1", response.ID)
	})

	t.Run("missing X-User-ID", func(t *testing.T) {
		_, router := setupTest(t)

		body, contentType := createMultipartBody(t, "file", "avatar.jpg", []byte("fake"))

		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", contentType)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestAvatarHandler_GetMetadata(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			GetMetadata(gomock.Any(), "avatar-1").
			Return(&domain.Avatar{ID: "avatar-1", UserID: "user-123"}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/avatar-1/metadata", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response domain.Avatar
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
		assert.Equal(t, "avatar-1", response.ID)
	})

	t.Run("not found", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			GetMetadata(gomock.Any(), "unknown").
			Return(nil, domain.ErrNotFound)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/unknown/metadata", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestAvatarHandler_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			Delete(gomock.Any(), "avatar-1", "user-123").
			Return(nil)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/avatars/avatar-1", nil)
		req.Header.Set("X-User-ID", "user-123")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("forbidden", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			Delete(gomock.Any(), "avatar-1", "other-user").
			Return(domain.ErrForbidden)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/avatars/avatar-1", nil)
		req.Header.Set("X-User-ID", "other-user")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestAvatarHandler_ListByUserID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			ListByUserID(gomock.Any(), "user-123").
			Return([]*domain.Avatar{
				{ID: "avatar-1", UserID: "user-123"},
				{ID: "avatar-2", UserID: "user-123"},
			}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-123/avatars", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var response []*domain.Avatar
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
		assert.Len(t, response, 2)
	})
}

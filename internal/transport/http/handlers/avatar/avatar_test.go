package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
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

	t.Run("invalid X-User-ID - path traversal", func(t *testing.T) {
		_, router := setupTest(t)

		body, contentType := createMultipartBody(t, "file", "avatar.jpg", []byte("fake"))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-User-ID", "../../etc/passwd")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("X-User-ID too long", func(t *testing.T) {
		_, router := setupTest(t)

		body, contentType := createMultipartBody(t, "file", "avatar.jpg", []byte("fake"))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-User-ID", strings.Repeat("a", 256))

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing file field", func(t *testing.T) {
		_, router := setupTest(t)

		body, contentType := createMultipartBody(t, "wrong_field", "avatar.jpg", []byte("fake"))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-User-ID", "user-123")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("body too large - 413 with max_size", func(t *testing.T) {
		_, router := setupTest(t)

		// Тело больше 10 MB — MaxBytesReader вернёт MaxBytesError, handler ответит 413.
		big := bytes.Repeat([]byte("x"), int(domain.MaxFileSize)+1024)
		body, contentType := createMultipartBody(t, "file", "big.jpg", big)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-User-ID", "user-123")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "File too large", resp["error"])
		assert.NotZero(t, resp["max_size"])
	})

	t.Run("invalid format - 400 with details", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, domain.ErrInvalidFormat)

		body, contentType := createMultipartBody(t, "file", "test.txt", []byte("hello"))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-User-ID", "user-123")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp map[string]any
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "Invalid file format", resp["error"])
		assert.Contains(t, resp["details"], "jpeg")
	})
}

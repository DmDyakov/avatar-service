package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	router.Get("/api/v1/avatars/{id}", handler.Get)
	router.Get("/api/v1/users/{user_id}/avatar", handler.GetUserAvatar)

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

func TestAvatarHandler_Get(t *testing.T) {
	t.Run("success - binary with headers", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		avatar := &domain.Avatar{ID: "avatar-1", MimeType: "image/jpeg"}
		mockSvc.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(avatar, io.NopCloser(strings.NewReader("jpeg-bytes")), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/avatar-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
		assert.Equal(t, "jpeg-bytes", rec.Body.String())
	})

	t.Run("not found - 404 with error text from spec", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			GetByID(gomock.Any(), "unknown").
			Return(nil, nil, domain.ErrNotFound)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/unknown", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp map[string]string
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "Avatar not found", resp["error"])
	})

	t.Run("storage unavailable - 500 not 404", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(nil, nil, domain.ErrStorageUnavailable)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/avatar-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("empty id", func(t *testing.T) {
		_, router := setupTest(t)

		// chi не заматчит пустой {id} — но если роут /api/v1/avatars/
		// (со слешем, без id), chi вернёт 404 или 405.
		// Проверяем что эндпоинт не паникует и не 200.
		req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusOK, rec.Code)
	})
}

func TestAvatarHandler_GetUserAvatar(t *testing.T) {
	t.Run("success - binary with headers", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		avatar := &domain.Avatar{ID: "avatar-2", UserID: "user-123", MimeType: "image/png"}
		mockSvc.EXPECT().
			GetUserAvatar(gomock.Any(), "user-123").
			Return(avatar, io.NopCloser(strings.NewReader("png-bytes")), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-123/avatar", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
		assert.Equal(t, "png-bytes", rec.Body.String())
	})

	t.Run("not found", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		mockSvc.EXPECT().
			GetUserAvatar(gomock.Any(), "nobody").
			Return(nil, nil, domain.ErrNotFound)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/nobody/avatar", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp map[string]string
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "Avatar not found", resp["error"])
	})

	t.Run("invalid user_id - 400", func(t *testing.T) {
		mockSvc, router := setupTest(t)

		_ = mockSvc

		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/users/..%2F..%2Fetc%2Fpasswd/avatar", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

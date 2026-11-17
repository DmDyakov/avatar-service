package avatar

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"testing"
	"time"

	"avatar-service/internal/domain"
	"avatar-service/internal/services/avatar/mocks"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

// fakeTxManager — hand-rolled мок TxManager: выполняет fn с nil-tx.
// Моки репозитория и outbox игнорируют tx (принимают gomock.Any()),
// поэтому nil безопасен и не требует MockTxManager.
type fakeTxManager struct {
	err error
}

func (f *fakeTxManager) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, tx pgx.Tx) error,
) error {
	if f.err != nil {
		return f.err
	}
	return fn(ctx, nil)
}

func setupTest(t *testing.T) (
	*mocks.MockAvatarRepository,
	*mocks.MockStorage,
	*mocks.MockOutboxWriter,
	*Service,
) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockAvatarRepository(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockOutbox := mocks.NewMockOutboxWriter(ctrl)

	svc := New(&fakeTxManager{}, mockRepo, mockStorage, mockOutbox, zap.NewNop())

	return mockRepo, mockStorage, mockOutbox, svc
}

// -----------------------------------------------------------------
// Upload
// -----------------------------------------------------------------

func TestService_Upload(t *testing.T) {
	t.Run("success - writes to outbox in same tx", func(t *testing.T) {
		mockRepo, mockStorage, mockOutbox, svc := setupTest(t)

		mockStorage.EXPECT().
			Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "image/jpeg").
			DoAndReturn(func(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
				assert.True(t, strings.HasPrefix(key, "avatars/user-123/"),
					"key должен начинаться с avatars/{userID}/, got %s", key)
				return nil
			})

		mockRepo.EXPECT().
			CreateTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		mockOutbox.EXPECT().
			Write(gomock.Any(), gomock.Any(), "avatar.upload", gomock.Any()).
			DoAndReturn(func(ctx context.Context, tx pgx.Tx, eventType string, payload any) error {
				ev, ok := payload.(domain.AvatarUploadEvent)
				require.True(t, ok, "payload должен быть domain.AvatarUploadEvent")
				assert.NotEmpty(t, ev.AvatarID)
				assert.Equal(t, "user-123", ev.UserID)
				assert.Contains(t, ev.S3Key, "avatars/user-123/")
				return nil
			})

		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes(t))

		avatar, err := svc.Upload(context.Background(), "user-123", file, header)
		require.NoError(t, err)
		assert.Equal(t, "user-123", avatar.UserID)
		assert.Equal(t, "image/jpeg", avatar.MimeType)
		assert.NotEmpty(t, avatar.ID)
		assert.NotNil(t, avatar.Width, "width должен быть заполнен (AS-107.8)")
		assert.NotNil(t, avatar.Height, "height должен быть заполнен (AS-107.8)")
	})

	t.Run("invalid format", func(t *testing.T) {
		_, _, _, svc := setupTest(t)

		file, header := createMultipartFile(t, "test.txt", []byte("hello world"))

		_, err := svc.Upload(context.Background(), "user-123", file, header)
		assert.ErrorIs(t, err, domain.ErrInvalidFormat)
	})

	t.Run("file too large", func(t *testing.T) {
		_, _, _, svc := setupTest(t)

		header := &multipart.FileHeader{
			Filename: "big.jpg",
			Size:     domain.MaxFileSize + 1,
		}

		_, err := svc.Upload(context.Background(), "user-123", nil, header)
		assert.ErrorIs(t, err, domain.ErrFileTooLarge)
	})

	t.Run("empty file", func(t *testing.T) {
		_, _, _, svc := setupTest(t)

		header := &multipart.FileHeader{
			Filename: "empty.jpg",
			Size:     0,
		}

		_, err := svc.Upload(context.Background(), "user-123", nil, header)
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	})

	t.Run("invalid image - DecodeConfig fails", func(t *testing.T) {
		_, _, _, svc := setupTest(t)

		// Magic bytes JPEG валидные (проходит validateUpload), но содержимое
		// битое — image.DecodeConfig падает, сервис возвращает ErrInvalidFormat.
		broken := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("not a real jpeg")...)
		file, header := createMultipartFile(t, "broken.jpg", broken)

		_, err := svc.Upload(context.Background(), "user-123", file, header)
		assert.ErrorIs(t, err, domain.ErrInvalidFormat)
	})

	t.Run("storage upload error", func(t *testing.T) {
		_, mockStorage, _, svc := setupTest(t)

		storageErr := errors.New("s3 down")
		mockStorage.EXPECT().
			Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(storageErr)

		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes(t))

		_, err := svc.Upload(context.Background(), "user-123", file, header)
		assert.ErrorIs(t, err, storageErr)
	})

	t.Run("repo error - rollback s3", func(t *testing.T) {
		mockRepo, mockStorage, _, svc := setupTest(t)

		mockStorage.EXPECT().
			Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		mockRepo.EXPECT().
			CreateTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(domain.ErrInvalidInput)

		// Outbox.Write НЕ ожидается: CreateTx упал → транзакция откатывается.
		// S3-файл откатывается.
		mockStorage.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			Return(nil)

		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes(t))

		_, err := svc.Upload(context.Background(), "user-123", file, header)
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	})

	t.Run("outbox write error - rollback s3", func(t *testing.T) {
		mockRepo, mockStorage, mockOutbox, svc := setupTest(t)

		mockStorage.EXPECT().
			Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		mockRepo.EXPECT().
			CreateTx(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		outboxErr := errors.New("outbox table broken")
		mockOutbox.EXPECT().
			Write(gomock.Any(), gomock.Any(), "avatar.upload", gomock.Any()).
			Return(outboxErr)

		// Транзакция откатится → S3-файл тоже надо откатить.
		mockStorage.EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			Return(nil)

		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes(t))

		_, err := svc.Upload(context.Background(), "user-123", file, header)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "outbox table broken")
	})
}

// -----------------------------------------------------------------
// GetByID
// -----------------------------------------------------------------

func TestService_GetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, mockStorage, _, svc := setupTest(t)

		avatar := &domain.Avatar{
			ID:     "avatar-1",
			UserID: "user-123",
			S3Key:  "avatars/user-123/avatar-1",
		}
		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(avatar, nil)

		reader := io.NopCloser(strings.NewReader("bytes"))
		mockStorage.EXPECT().
			Download(gomock.Any(), "avatars/user-123/avatar-1").
			Return(reader, nil)

		got, r, err := svc.GetByID(context.Background(), "avatar-1")
		require.NoError(t, err)
		assert.Equal(t, avatar, got)
		assert.NotNil(t, r)
		_ = r.Close()
	})

	t.Run("repo not found", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "unknown").
			Return(nil, domain.ErrNotFound)

		_, _, err := svc.GetByID(context.Background(), "unknown")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("storage error mapped to ErrStorageUnavailable", func(t *testing.T) {
		mockRepo, mockStorage, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(&domain.Avatar{ID: "avatar-1", S3Key: "key"}, nil)

		mockStorage.EXPECT().
			Download(gomock.Any(), "key").
			Return(nil, errors.New("s3 down"))

		_, _, err := svc.GetByID(context.Background(), "avatar-1")
		assert.ErrorIs(t, err, domain.ErrStorageUnavailable)
		assert.NotErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("storage returns ErrNotFound -> still ErrStorageUnavailable", func(t *testing.T) {
		mockRepo, mockStorage, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(&domain.Avatar{ID: "avatar-1", S3Key: "key"}, nil)

		mockStorage.EXPECT().
			Download(gomock.Any(), "key").
			Return(nil, domain.ErrNotFound)

		_, _, err := svc.GetByID(context.Background(), "avatar-1")
		assert.ErrorIs(t, err, domain.ErrStorageUnavailable)
		assert.NotErrorIs(t, err, domain.ErrNotFound)
	})
}

// -----------------------------------------------------------------
// GetUserAvatar
// -----------------------------------------------------------------

func TestService_GetUserAvatar(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, mockStorage, _, svc := setupTest(t)

		avatar := &domain.Avatar{
			ID:     "avatar-2",
			UserID: "user-123",
			S3Key:  "avatars/user-123/avatar-2",
		}
		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "user-123").
			Return(avatar, nil)

		reader := io.NopCloser(strings.NewReader("bytes"))
		mockStorage.EXPECT().
			Download(gomock.Any(), "avatars/user-123/avatar-2").
			Return(reader, nil)

		got, r, err := svc.GetUserAvatar(context.Background(), "user-123")
		require.NoError(t, err)
		assert.Equal(t, avatar, got)
		assert.NotNil(t, r)
		_ = r.Close()
	})

	t.Run("repo not found", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "nobody").
			Return(nil, domain.ErrNotFound)

		_, _, err := svc.GetUserAvatar(context.Background(), "nobody")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("storage error mapped to ErrStorageUnavailable", func(t *testing.T) {
		mockRepo, mockStorage, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "user-123").
			Return(&domain.Avatar{ID: "avatar-2", UserID: "user-123", S3Key: "key"}, nil)

		mockStorage.EXPECT().
			Download(gomock.Any(), "key").
			Return(nil, errors.New("s3 down"))

		_, _, err := svc.GetUserAvatar(context.Background(), "user-123")
		assert.ErrorIs(t, err, domain.ErrStorageUnavailable)
		assert.NotErrorIs(t, err, domain.ErrNotFound)
	})
}

// -----------------------------------------------------------------
// ListByUserID
// -----------------------------------------------------------------

func TestService_ListByUserID(t *testing.T) {
	t.Run("success with avatars", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		expected := []*domain.Avatar{
			{ID: "avatar-1", UserID: "user-123", CreatedAt: time.Now()},
			{ID: "avatar-2", UserID: "user-123", CreatedAt: time.Now().Add(-time.Hour)},
		}
		mockRepo.EXPECT().
			ListByUserID(gomock.Any(), "user-123").
			Return(expected, nil)

		got, err := svc.ListByUserID(context.Background(), "user-123")
		require.NoError(t, err)
		assert.Len(t, got, 2)
		assert.Equal(t, expected, got)
	})

	t.Run("nil from repo -> empty slice, not nil", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		// Репозиторий вернул nil (нет записей) — сервис должен нормализовать в [].
		mockRepo.EXPECT().
			ListByUserID(gomock.Any(), "no-avatars-user").
			Return(nil, nil)

		got, err := svc.ListByUserID(context.Background(), "no-avatars-user")
		require.NoError(t, err)
		assert.NotNil(t, got, "сервис должен вернуть пустой слайс, не nil (иначе JSON = null)")
		assert.Empty(t, got)
	})

	t.Run("repo error", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			ListByUserID(gomock.Any(), "user-123").
			Return(nil, domain.ErrInvalidInput)

		_, err := svc.ListByUserID(context.Background(), "user-123")
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	})
}

// -----------------------------------------------------------------
// DeleteUserAvatar
// -----------------------------------------------------------------

func TestService_DeleteUserAvatar(t *testing.T) {
	t.Run("success - soft delete + outbox in same tx", func(t *testing.T) {
		mockRepo, _, mockOutbox, svc := setupTest(t)

		avatar := &domain.Avatar{
			ID:     "avatar-1",
			UserID: "user-123",
			S3Key:  "avatars/user-123/avatar-1",
			ThumbnailS3Keys: map[string]string{
				"100x100": "thumbnails/avatar-1/100x100.jpg",
				"300x300": "thumbnails/avatar-1/300x300.jpg",
			},
		}

		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "user-123").
			Return(avatar, nil)

		mockRepo.EXPECT().
			SoftDeleteTx(gomock.Any(), gomock.Any(), "avatar-1").
			Return(nil)

		mockOutbox.EXPECT().
			Write(gomock.Any(), gomock.Any(), "avatar.delete", gomock.Any()).
			DoAndReturn(func(ctx context.Context, tx pgx.Tx, eventType string, payload any) error {
				ev, ok := payload.(domain.AvatarDeleteEvent)
				require.True(t, ok, "payload должен быть domain.AvatarDeleteEvent")
				assert.Equal(t, "avatar-1", ev.AvatarID)
				assert.Len(t, ev.S3Keys, 3, "оригинал + 2 миниатюры")
				assert.Contains(t, ev.S3Keys, "avatars/user-123/avatar-1")
				assert.Contains(t, ev.S3Keys, "thumbnails/avatar-1/100x100.jpg")
				assert.Contains(t, ev.S3Keys, "thumbnails/avatar-1/300x300.jpg")
				return nil
			})

		err := svc.DeleteUserAvatar(context.Background(), "user-123", "user-123")
		assert.NoError(t, err)
	})

	t.Run("not found - no active avatars for user", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "nobody").
			Return(nil, domain.ErrNotFound)

		err := svc.DeleteUserAvatar(context.Background(), "nobody", "nobody")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("forbidden - actor is not owner", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "victim").
			Return(&domain.Avatar{ID: "avatar-1", UserID: "victim"}, nil)

		err := svc.DeleteUserAvatar(context.Background(), "victim", "attacker")
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("soft delete error propagates, no outbox write", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "user-123").
			Return(&domain.Avatar{ID: "avatar-1", UserID: "user-123", S3Key: "key"}, nil)

		mockRepo.EXPECT().
			SoftDeleteTx(gomock.Any(), gomock.Any(), "avatar-1").
			Return(domain.ErrNotFound)

		// Outbox.Write НЕ ожидается — SoftDeleteTx упал, транзакция откатится.

		err := svc.DeleteUserAvatar(context.Background(), "user-123", "user-123")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

// -----------------------------------------------------------------
// Delete (по ID)
// -----------------------------------------------------------------

func TestService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, mockOutbox, svc := setupTest(t)

		avatar := &domain.Avatar{
			ID:     "avatar-1",
			UserID: "user-123",
			S3Key:  "avatars/user-123/avatar-1",
		}
		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(avatar, nil)

		mockRepo.EXPECT().
			SoftDeleteTx(gomock.Any(), gomock.Any(), "avatar-1").
			Return(nil)

		mockOutbox.EXPECT().
			Write(gomock.Any(), gomock.Any(), "avatar.delete", gomock.Any()).
			DoAndReturn(func(ctx context.Context, tx pgx.Tx, eventType string, payload any) error {
				ev := payload.(domain.AvatarDeleteEvent)
				assert.Equal(t, "avatar-1", ev.AvatarID)
				return nil
			})

		err := svc.Delete(context.Background(), "avatar-1", "user-123")
		assert.NoError(t, err)
	})

	t.Run("forbidden", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(&domain.Avatar{ID: "avatar-1", UserID: "victim"}, nil)

		err := svc.Delete(context.Background(), "avatar-1", "attacker")
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})
}

// jpegBytes читает реальный JPEG из testdata.
//
// Собранный вручную []byte{0xFF, 0xD8, 0xFF, 0xE0} + нули проходит
// magic-bytes в validateUpload, но image.DecodeConfig на нём падает.
// Для тестов, где срабатывает AS-107.8, нужен валидный файл.
func jpegBytes(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile("../../../tests/testdata/avatar.jpg")
	require.NoError(t, err)
	return data
}

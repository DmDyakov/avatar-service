package avatar

import (
	"context"
	"mime/multipart"
	"testing"

	"avatar-service/internal/domain"
	"avatar-service/internal/services/avatar/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func setupTest(t *testing.T) (*mocks.MockAvatarRepository, *mocks.MockStorage, *mocks.MockEventPublisher, *Service) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockAvatarRepository(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockPublisher := mocks.NewMockEventPublisher(ctrl)

	svc := New(mockRepo, mockStorage, mockPublisher, zap.NewNop())

	return mockRepo, mockStorage, mockPublisher, svc
}

func TestService_Upload(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, avatar *domain.Avatar) error {
				avatar.ID = "avatar-1"
				return nil
			})

		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes())

		avatar, err := svc.Upload(context.Background(), "user-123", file, header)
		require.NoError(t, err)
		assert.Equal(t, "user-123", avatar.UserID)
		assert.Equal(t, "image/jpeg", avatar.MimeType)
		assert.Equal(t, "avatar-1", avatar.ID)
	})

	t.Run("invalid format", func(t *testing.T) {
		_, _, _, svc := setupTest(t)

		file, header := createMultipartFile(t, "test.txt", []byte("hello"))

		_, err := svc.Upload(context.Background(), "user-123", file, header)
		assert.ErrorIs(t, err, domain.ErrInvalidFormat)
	})

	t.Run("file too large", func(t *testing.T) {
		_, _, _, svc := setupTest(t)

		header := &multipart.FileHeader{
			Filename: "big.jpg",
			Size:     maxFileSize + 1,
		}

		_, err := svc.Upload(context.Background(), "user-123", nil, header)
		assert.ErrorIs(t, err, domain.ErrFileTooLarge)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(domain.ErrInvalidInput)

		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes())

		_, err := svc.Upload(context.Background(), "user-123", file, header)
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	})
}

func TestService_GetMetadata(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		expected := &domain.Avatar{ID: "avatar-1", UserID: "user-123"}
		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(expected, nil)

		avatar, err := svc.GetMetadata(context.Background(), "avatar-1")
		require.NoError(t, err)
		assert.Equal(t, expected, avatar)
	})

	t.Run("not found", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "unknown").
			Return(nil, domain.ErrNotFound)

		_, err := svc.GetMetadata(context.Background(), "unknown")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		expected := &domain.Avatar{ID: "avatar-1"}
		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(expected, nil)

		avatar, reader, err := svc.GetByID(context.Background(), "avatar-1")
		require.NoError(t, err)
		assert.Equal(t, expected, avatar)
		assert.Nil(t, reader)
	})

	t.Run("not found", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "unknown").
			Return(nil, domain.ErrNotFound)

		_, _, err := svc.GetByID(context.Background(), "unknown")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestService_GetUserAvatar(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		expected := &domain.Avatar{ID: "avatar-1", UserID: "user-123"}
		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "user-123").
			Return(expected, nil)

		avatar, reader, err := svc.GetUserAvatar(context.Background(), "user-123")
		require.NoError(t, err)
		assert.Equal(t, expected, avatar)
		assert.Nil(t, reader)
	})

	t.Run("not found", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetUserAvatar(gomock.Any(), "user-123").
			Return(nil, domain.ErrNotFound)

		_, _, err := svc.GetUserAvatar(context.Background(), "user-123")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestService_ListByUserID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		expected := []*domain.Avatar{
			{ID: "avatar-1", UserID: "user-123"},
			{ID: "avatar-2", UserID: "user-123"},
		}
		mockRepo.EXPECT().
			ListByUserID(gomock.Any(), "user-123").
			Return(expected, nil)

		avatars, err := svc.ListByUserID(context.Background(), "user-123")
		require.NoError(t, err)
		assert.Len(t, avatars, 2)
	})

	t.Run("empty list", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			ListByUserID(gomock.Any(), "user-123").
			Return([]*domain.Avatar{}, nil)

		avatars, err := svc.ListByUserID(context.Background(), "user-123")
		require.NoError(t, err)
		assert.Empty(t, avatars)
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(&domain.Avatar{ID: "avatar-1", UserID: "user-123"}, nil)

		mockRepo.EXPECT().
			SoftDelete(gomock.Any(), "avatar-1").
			Return(nil)

		err := svc.Delete(context.Background(), "avatar-1", "user-123")
		assert.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "unknown").
			Return(nil, domain.ErrNotFound)

		err := svc.Delete(context.Background(), "unknown", "user-123")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("forbidden - not owner", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(&domain.Avatar{ID: "avatar-1", UserID: "other-user"}, nil)

		err := svc.Delete(context.Background(), "avatar-1", "user-123")
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("soft delete error", func(t *testing.T) {
		mockRepo, _, _, svc := setupTest(t)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), "avatar-1").
			Return(&domain.Avatar{ID: "avatar-1", UserID: "user-123"}, nil)

		mockRepo.EXPECT().
			SoftDelete(gomock.Any(), "avatar-1").
			Return(domain.ErrNotFound)

		err := svc.Delete(context.Background(), "avatar-1", "user-123")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

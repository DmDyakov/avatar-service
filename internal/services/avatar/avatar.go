// Package avatar contains business logic for working with avatars.
package avatar

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"time"

	"avatar-service/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"

	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

//go:generate mockgen -destination=mocks/mock_avatar_repository.go -package=mocks avatar-service/internal/services/avatar AvatarRepository
type AvatarRepository interface {
	Create(ctx context.Context, avatar *domain.Avatar) error
	GetByID(ctx context.Context, id string) (*domain.Avatar, error)
	GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, error)
}

//go:generate mockgen -destination=mocks/mock_storage.go -package=mocks avatar-service/internal/services/avatar Storage
type Storage interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

//go:generate mockgen -destination=mocks/mock_event_publisher.go -package=mocks avatar-service/internal/services/avatar EventPublisher
type EventPublisher interface {
	PublishUploadEvent(ctx context.Context, event domain.AvatarUploadEvent) error
}

// Service реализует бизнес-логику работы с аватарками.
type Service struct {
	avatarRepo AvatarRepository
	storage    Storage
	publisher  EventPublisher
	logger     *zap.Logger
}

func New(
	avatarRepo AvatarRepository,
	storage Storage,
	publisher EventPublisher,
	logger *zap.Logger,
) *Service {
	return &Service{
		avatarRepo: avatarRepo,
		storage:    storage,
		publisher:  publisher,
		logger:     logger,
	}
}

// Upload загружает новую аватарку.
func (s *Service) Upload(
	ctx context.Context,
	userID string,
	file multipart.File,
	header *multipart.FileHeader,
) (*domain.Avatar, error) {
	contentType, err := validateUpload(file, header)
	if err != nil {
		return nil, err
	}

	width, height, err := decodeDimensions(file)
	if err != nil {
		return nil, err
	}

	avatarID := uuid.New().String()
	s3Key := fmt.Sprintf("avatars/%s/%s", userID, avatarID)

	if err := s.storage.Upload(ctx, s3Key, file, header.Size, contentType); err != nil {
		return nil, fmt.Errorf("upload to storage: %w", err)
	}

	avatar := &domain.Avatar{
		ID:               avatarID,
		UserID:           userID,
		FileName:         header.Filename,
		MimeType:         contentType,
		SizeBytes:        header.Size,
		S3Key:            s3Key,
		Width:            &width,
		Height:           &height,
		UploadStatus:     domain.UploadStatusUploaded,
		ProcessingStatus: domain.ProcessingStatusPending,
		Attempts:         0,
	}

	if err := s.avatarRepo.Create(ctx, avatar); err != nil {
		if delErr := s.storage.Delete(ctx, s3Key); delErr != nil {
			return nil, errors.Join(
				fmt.Errorf("create avatar: %w", err),
				fmt.Errorf("rollback s3: %w", delErr),
			)
		}
		return nil, fmt.Errorf("create avatar: %w", err)
	}

	event := domain.AvatarUploadEvent{
		AvatarID: avatar.ID,
		UserID:   userID,
		S3Key:    s3Key,
	}
	if err := s.publishWithRetry(ctx, func() error {
		return s.publisher.PublishUploadEvent(ctx, event)
	}); err != nil {
		s.logger.Error("failed to publish upload event",
			zap.String("avatar_id", avatar.ID),
			zap.Error(err),
		)
	}

	return avatar, nil
}

// GetByID возвращает метаданные аватарки и поток её файла.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*domain.Avatar, io.ReadCloser, error) {
	avatar, err := s.avatarRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	reader, err := s.storage.Download(ctx, avatar.S3Key)
	if err != nil {
		return nil, nil, fmt.Errorf("download from storage: %w", err)
	}

	return avatar, reader, nil
}

// GetUserAvatar возвращает текущую аватарку пользователя и поток её файла.
func (s *Service) GetUserAvatar(
	ctx context.Context,
	userID string,
) (*domain.Avatar, io.ReadCloser, error) {
	avatar, err := s.avatarRepo.GetUserAvatar(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	reader, err := s.storage.Download(ctx, avatar.S3Key)
	if err != nil {
		return nil, nil, fmt.Errorf("download from storage: %w", err)
	}

	return avatar, reader, nil
}

// decodeDimensions читает width/height из заголовка изображения.
func decodeDimensions(file multipart.File) (int, int, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("seek before decode: %w", err)
	}

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, fmt.Errorf("decode image config: %w", domain.ErrInvalidFormat)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("seek after decode: %w", err)
	}

	return cfg.Width, cfg.Height, nil
}

// publishWithRetry публикует событие с backoff.
func (s *Service) publishWithRetry(ctx context.Context, fn func() error) error {
	const attempts = 3

	backoff := 100 * time.Millisecond
	var lastErr error

	for i := 0; i < attempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 3
	}

	return lastErr
}

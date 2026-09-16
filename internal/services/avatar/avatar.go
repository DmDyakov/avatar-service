// Package avatar contains business logic for working with avatars.
package avatar

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"

	"avatar-service/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

//go:generate mockgen -destination=mocks/mock_avatar_repository.go -package=mocks avatar-service/internal/services/avatar AvatarRepository
type AvatarRepository interface {
	Create(ctx context.Context, avatar *domain.Avatar) error
	GetByID(ctx context.Context, id string) (*domain.Avatar, error)
	GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, error)
	ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error)
	Update(ctx context.Context, avatar *domain.Avatar) error
	SoftDelete(ctx context.Context, id string) error
}

//go:generate mockgen -destination=mocks/mock_storage.go -package=mocks avatar-service/internal/services/avatar Storage
type Storage interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

//go:generate mockgen -destination=mocks/mock_event_publisher.go -package=mocks avatar-service/internal/services/avatar EventPublisher
type EventPublisher interface {
	PublishAvatarUploaded(ctx context.Context, event domain.AvatarUploadedEvent) error
	PublishAvatarDeleted(ctx context.Context, event domain.AvatarDeletedEvent) error
}

// Service реализует бизнес-логику работы с аватарками.
type Service struct {
	avatarRepo AvatarRepository
	storage    Storage
	publisher  EventPublisher
	logger     *zap.Logger
}

// New создаёт сервис аватарок.
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

	s3Key := fmt.Sprintf("avatars/%s/%s", userID, uuid.New().String())

	avatar := &domain.Avatar{
		UserID:       userID,
		FileName:     header.Filename,
		MimeType:     contentType,
		SizeBytes:    header.Size,
		S3Key:        s3Key,
		UploadStatus: domain.UploadStatusUploaded,
	}

	if err := s.avatarRepo.Create(ctx, avatar); err != nil {
		return nil, fmt.Errorf("create avatar: %w", err)
	}

	return avatar, nil
}

// GetByID возвращает аватарку по ID.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*domain.Avatar, io.ReadCloser, error) {
	avatar, err := s.avatarRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	return avatar, nil, nil
}

// GetUserAvatar возвращает текущую аватарку пользователя.
func (s *Service) GetUserAvatar(
	ctx context.Context,
	userID string,
) (*domain.Avatar, io.ReadCloser, error) {
	avatar, err := s.avatarRepo.GetUserAvatar(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return avatar, nil, nil
}

// GetMetadata возвращает метаданные аватарки.
func (s *Service) GetMetadata(
	ctx context.Context,
	id string,
) (*domain.Avatar, error) {
	return s.avatarRepo.GetByID(ctx, id)
}

// ListByUserID возвращает список аватарок пользователя.
func (s *Service) ListByUserID(
	ctx context.Context,
	userID string,
) ([]*domain.Avatar, error) {
	return s.avatarRepo.ListByUserID(ctx, userID)
}

// Delete удаляет аватарку пользователя.
func (s *Service) Delete(
	ctx context.Context,
	id string,
	userID string,
) error {
	avatar, err := s.avatarRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if avatar.UserID != userID {
		return domain.ErrForbidden
	}

	if err := s.avatarRepo.SoftDelete(ctx, id); err != nil {
		return err
	}

	return nil
}

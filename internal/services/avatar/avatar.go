// Package avatar contains business logic for working with avatars.
package avatar

import (
	"context"
	"io"
	"mime/multipart"

	"avatar-service/internal/domain"

	"go.uber.org/zap"
)

// AvatarRepository — контракт для работы с аватарками.
type AvatarRepository interface {
	Create(ctx context.Context, avatar *domain.Avatar) error
	GetByID(ctx context.Context, id string) (*domain.Avatar, error)
	GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, error)
	ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error)
	Update(ctx context.Context, avatar *domain.Avatar) error
	SoftDelete(ctx context.Context, id string) error
}

// Storage — контракт для работы с файловым хранилищем.
type Storage interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// EventPublisher — контракт для публикации событий.
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
	// TODO: реализовать
	return nil, domain.ErrInvalidInput
}

// GetByID возвращает аватарку и её файл по ID.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*domain.Avatar, io.ReadCloser, error) {
	// TODO: реализовать
	return nil, nil, domain.ErrNotFound
}

// GetUserAvatar возвращает текущую аватарку пользователя.
func (s *Service) GetUserAvatar(
	ctx context.Context,
	userID string,
) (*domain.Avatar, io.ReadCloser, error) {
	// TODO: реализовать
	return nil, nil, domain.ErrNotFound
}

// GetMetadata возвращает метаданные аватарки.
func (s *Service) GetMetadata(
	ctx context.Context,
	id string,
) (*domain.Avatar, error) {
	// TODO: реализовать
	return nil, domain.ErrNotFound
}

// ListByUserID возвращает список аватарок пользователя.
func (s *Service) ListByUserID(
	ctx context.Context,
	userID string,
) ([]*domain.Avatar, error) {
	// TODO: реализовать
	return []*domain.Avatar{}, nil
}

// Delete удаляет аватарку пользователя.
func (s *Service) Delete(
	ctx context.Context,
	id string,
	userID string,
) error {
	// TODO: реализовать
	return domain.ErrNotFound
}

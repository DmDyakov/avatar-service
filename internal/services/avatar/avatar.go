// Package avatar contains business logic for working with avatars.
//
// Сервис пишет события в transactional outbox в той же транзакции, что
// и бизнес-изменения (Create/SoftDelete). Публикацией в RabbitMQ
// занимается отдельный процесс cmd/relay.
package avatar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"

	"avatar-service/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

//go:generate mockgen -destination=mocks/mock_avatar_repository.go -package=mocks avatar-service/internal/services/avatar AvatarRepository
type AvatarRepository interface {
	Create(ctx context.Context, avatar *domain.Avatar) error
	CreateTx(ctx context.Context, tx pgx.Tx, avatar *domain.Avatar) error
	GetByID(ctx context.Context, id string) (*domain.Avatar, error)
	GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, error)
	ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error)
	SoftDelete(ctx context.Context, id string) error
	SoftDeleteTx(ctx context.Context, tx pgx.Tx, id string) error
}

//go:generate mockgen -destination=mocks/mock_storage.go -package=mocks avatar-service/internal/services/avatar Storage
type Storage interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

//go:generate mockgen -destination=mocks/mock_tx_manager.go -package=mocks avatar-service/internal/services/avatar TxManager
type TxManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error
}

//go:generate mockgen -destination=mocks/mock_outbox_writer.go -package=mocks avatar-service/internal/services/avatar OutboxWriter
type OutboxWriter interface {
	Write(ctx context.Context, tx pgx.Tx, eventType string, payload any) error
}

// Service реализует бизнес-логику работы с аватарками.
type Service struct {
	txManager  TxManager
	avatarRepo AvatarRepository
	storage    Storage
	outbox     OutboxWriter
	logger     *zap.Logger
}

func New(
	txManager TxManager,
	avatarRepo AvatarRepository,
	storage Storage,
	outbox OutboxWriter,
	logger *zap.Logger,
) *Service {
	return &Service{
		txManager:  txManager,
		avatarRepo: avatarRepo,
		storage:    storage,
		outbox:     outbox,
		logger:     logger,
	}
}

// Upload загружает новую аватарку.
//
// Порядок: S3 → (Create + outbox в одной транзакции).
// Если транзакция падает — откатываем S3, чтобы не оставить сироту.
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

	event := domain.AvatarUploadEvent{
		AvatarID: avatar.ID,
		UserID:   userID,
		S3Key:    s3Key,
	}

	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.avatarRepo.CreateTx(ctx, tx, avatar); err != nil {
			return fmt.Errorf("create avatar: %w", err)
		}
		if err := s.outbox.Write(ctx, tx, "avatar.upload", event); err != nil {
			return fmt.Errorf("write outbox: %w", err)
		}
		return nil
	})
	if err != nil {
		if delErr := s.storage.Delete(ctx, s3Key); delErr != nil {
			return nil, errors.Join(err, fmt.Errorf("rollback s3: %w", delErr))
		}
		return nil, err
	}

	return avatar, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*domain.Avatar, io.ReadCloser, error) {
	avatar, err := s.avatarRepo.GetByID(ctx, id)
	return s.fetchAvatar(ctx, avatar, err)
}

func (s *Service) GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, io.ReadCloser, error) {
	avatar, err := s.avatarRepo.GetUserAvatar(ctx, userID)
	return s.fetchAvatar(ctx, avatar, err)
}

func (s *Service) GetMetadata(ctx context.Context, id string) (*domain.Avatar, error) {
	return s.avatarRepo.GetByID(ctx, id)
}

func (s *Service) fetchAvatar(
	ctx context.Context,
	avatar *domain.Avatar,
	err error,
) (*domain.Avatar, io.ReadCloser, error) {
	if err != nil {
		return nil, nil, err
	}
	reader, err := s.storage.Download(ctx, avatar.S3Key)
	if err != nil {
		s.logger.Error("failed to download avatar from storage",
			zap.String("avatar_id", avatar.ID),
			zap.String("s3_key", avatar.S3Key),
			zap.Error(err),
		)
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrStorageUnavailable, err)
	}
	return avatar, reader, nil
}

func (s *Service) ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error) {
	avatars, err := s.avatarRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if avatars == nil {
		return []*domain.Avatar{}, nil
	}
	return avatars, nil
}

func (s *Service) Delete(ctx context.Context, id string, userID string) error {
	avatar, err := s.avatarRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.deleteAndPublish(ctx, avatar, userID)
}

func (s *Service) DeleteUserAvatar(ctx context.Context, userID, actorID string) error {
	avatar, err := s.avatarRepo.GetUserAvatar(ctx, userID)
	if err != nil {
		return err
	}
	return s.deleteAndPublish(ctx, avatar, actorID)
}

// deleteAndPublish: проверка владельца, soft delete и outbox —
// в одной транзакции.
func (s *Service) deleteAndPublish(ctx context.Context, avatar *domain.Avatar, actorID string) error {
	if avatar.UserID != actorID {
		return domain.ErrForbidden
	}

	s3Keys := make([]string, 0, 1+len(avatar.ThumbnailS3Keys))
	s3Keys = append(s3Keys, avatar.S3Key)
	for _, key := range avatar.ThumbnailS3Keys {
		s3Keys = append(s3Keys, key)
	}

	event := domain.AvatarDeleteEvent{
		AvatarID: avatar.ID,
		S3Keys:   s3Keys,
	}

	return s.txManager.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.avatarRepo.SoftDeleteTx(ctx, tx, avatar.ID); err != nil {
			return err
		}
		if err := s.outbox.Write(ctx, tx, "avatar.delete", event); err != nil {
			return fmt.Errorf("write outbox: %w", err)
		}
		return nil
	})
}

package avatar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"avatar-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AvatarRepository реализует работу с аватарками в PostgreSQL.
type AvatarRepository struct {
	pool *pgxpool.Pool
}

// NewAvatarRepository создаёт репозиторий аватарок.
func NewAvatarRepository(pool *pgxpool.Pool) *AvatarRepository {
	return &AvatarRepository{pool: pool}
}

// Create сохраняет новую аватарку.
//
// id передаётся явно из сервиса (не генерируется в БД), чтобы id в БД и
// s3_key (avatars/{userID}/{id}) совпадали. Без этого БД генерирует свой
// UUID через DEFAULT, а s3_key остаётся с UUID из сервиса — рассинхрон.
func (r *AvatarRepository) Create(ctx context.Context, avatar *domain.Avatar) error {
	const query = `
		INSERT INTO avatars (
			id, user_id, file_name, mime_type, size_bytes, s3_key,
			width, height, thumbnail_s3_keys, upload_status, processing_status, attempts
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at
	`

	thumbKeys, err := marshalThumbnailKeys(avatar.ThumbnailS3Keys)
	if err != nil {
		return fmt.Errorf("marshal thumbnail keys: %w", err)
	}

	err = r.pool.QueryRow(ctx, query,
		avatar.ID,
		avatar.UserID,
		avatar.FileName,
		avatar.MimeType,
		avatar.SizeBytes,
		avatar.S3Key,
		avatar.Width,
		avatar.Height,
		thumbKeys,
		avatar.UploadStatus,
		avatar.ProcessingStatus,
		avatar.Attempts,
	).Scan(&avatar.CreatedAt, &avatar.UpdatedAt)

	if err != nil {
		return fmt.Errorf("create avatar: %w", err)
	}

	return nil
}

// GetByID получает аватарку по ID.
func (r *AvatarRepository) GetByID(ctx context.Context, id string) (*domain.Avatar, error) {
	const query = `
		SELECT id, user_id, file_name, mime_type, size_bytes, s3_key,
		       width, height, thumbnail_s3_keys, upload_status, processing_status,
		       attempts, created_at, updated_at, deleted_at
		FROM avatars
		WHERE id = $1 AND deleted_at IS NULL
	`

	var avatar domain.Avatar
	var thumbKeys []byte
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&avatar.ID,
		&avatar.UserID,
		&avatar.FileName,
		&avatar.MimeType,
		&avatar.SizeBytes,
		&avatar.S3Key,
		&avatar.Width,
		&avatar.Height,
		&thumbKeys,
		&avatar.UploadStatus,
		&avatar.ProcessingStatus,
		&avatar.Attempts,
		&avatar.CreatedAt,
		&avatar.UpdatedAt,
		&avatar.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get avatar by id: %w", err)
	}

	if err := unmarshalThumbnailKeys(thumbKeys, &avatar); err != nil {
		return nil, fmt.Errorf("unmarshal thumbnail keys: %w", err)
	}

	return &avatar, nil
}

// GetUserAvatar получает последнюю аватарку пользователя.
func (r *AvatarRepository) GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, error) {
	const query = `
		SELECT id, user_id, file_name, mime_type, size_bytes, s3_key,
		       width, height, thumbnail_s3_keys, upload_status, processing_status,
		       attempts, created_at, updated_at, deleted_at
		FROM avatars
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	var avatar domain.Avatar
	var thumbKeys []byte
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&avatar.ID,
		&avatar.UserID,
		&avatar.FileName,
		&avatar.MimeType,
		&avatar.SizeBytes,
		&avatar.S3Key,
		&avatar.Width,
		&avatar.Height,
		&thumbKeys,
		&avatar.UploadStatus,
		&avatar.ProcessingStatus,
		&avatar.Attempts,
		&avatar.CreatedAt,
		&avatar.UpdatedAt,
		&avatar.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get user avatar: %w", err)
	}

	if err := unmarshalThumbnailKeys(thumbKeys, &avatar); err != nil {
		return nil, fmt.Errorf("unmarshal thumbnail keys: %w", err)
	}

	return &avatar, nil
}

// ListByUserID получает все аватарки пользователя.
func (r *AvatarRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error) {
	const query = `
		SELECT id, user_id, file_name, mime_type, size_bytes, s3_key,
		       width, height, thumbnail_s3_keys, upload_status, processing_status,
		       attempts, created_at, updated_at, deleted_at
		FROM avatars
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list avatars: %w", err)
	}
	defer rows.Close()

	var avatars []*domain.Avatar
	for rows.Next() {
		var avatar domain.Avatar
		var thumbKeys []byte
		err := rows.Scan(
			&avatar.ID,
			&avatar.UserID,
			&avatar.FileName,
			&avatar.MimeType,
			&avatar.SizeBytes,
			&avatar.S3Key,
			&avatar.Width,
			&avatar.Height,
			&thumbKeys,
			&avatar.UploadStatus,
			&avatar.ProcessingStatus,
			&avatar.Attempts,
			&avatar.CreatedAt,
			&avatar.UpdatedAt,
			&avatar.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan avatar: %w", err)
		}

		if err := unmarshalThumbnailKeys(thumbKeys, &avatar); err != nil {
			return nil, fmt.Errorf("unmarshal thumbnail keys: %w", err)
		}

		avatars = append(avatars, &avatar)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate avatars: %w", err)
	}

	return avatars, nil
}

// UpdateProcessingStatus обновляет статус фоновой обработки.
func (r *AvatarRepository) UpdateProcessingStatus(ctx context.Context, id string, status domain.ProcessingStatus) error {
	const query = `
		UPDATE avatars
		SET processing_status = $2
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// UpdateThumbnailKeys обновляет ключи миниатюр (JSONB).
func (r *AvatarRepository) UpdateThumbnailKeys(ctx context.Context, id string, keys map[string]string) error {
	const query = `
		UPDATE avatars
		SET thumbnail_s3_keys = $2
		WHERE id = $1 AND deleted_at IS NULL
	`

	thumbKeys, err := marshalThumbnailKeys(keys)
	if err != nil {
		return fmt.Errorf("marshal thumbnail keys: %w", err)
	}

	result, err := r.pool.Exec(ctx, query, id, thumbKeys)
	if err != nil {
		return fmt.Errorf("update thumbnail keys: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// IncrementAttempts увеличивает счётчик попыток обработки.
func (r *AvatarRepository) IncrementAttempts(ctx context.Context, id string) error {
	const query = `
		UPDATE avatars
		SET attempts = attempts + 1
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("increment attempts: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// Update обновляет метаданные аватарки.
func (r *AvatarRepository) Update(ctx context.Context, avatar *domain.Avatar) error {
	const query = `
		UPDATE avatars
		SET file_name = $2,
		    mime_type = $3,
		    size_bytes = $4,
		    s3_key = $5,
		    upload_status = $6,
		    width = $7,
		    height = $8
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query,
		avatar.ID,
		avatar.FileName,
		avatar.MimeType,
		avatar.SizeBytes,
		avatar.S3Key,
		avatar.UploadStatus,
		avatar.Width,
		avatar.Height,
	)

	if err != nil {
		return fmt.Errorf("update avatar: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// SoftDelete мягко удаляет аватарку.
func (r *AvatarRepository) SoftDelete(ctx context.Context, id string) error {
	const query = `
		UPDATE avatars
		SET deleted_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, id, time.Now())
	if err != nil {
		return fmt.Errorf("soft delete avatar: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// marshalThumbnailKeys превращает map в JSONB-байты (nil → NULL).
func marshalThumbnailKeys(keys map[string]string) ([]byte, error) {
	if keys == nil {
		return nil, nil
	}
	return json.Marshal(keys)
}

// unmarshalThumbnailKeys парсит JSONB-байты в map (nil/пусто → nil).
func unmarshalThumbnailKeys(data []byte, avatar *domain.Avatar) error {
	if len(data) == 0 {
		avatar.ThumbnailS3Keys = nil
		return nil
	}
	return json.Unmarshal(data, &avatar.ThumbnailS3Keys)
}

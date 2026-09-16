package avatar

import (
	"context"
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
func (r *AvatarRepository) Create(ctx context.Context, avatar *domain.Avatar) error {
	const query = `
		INSERT INTO avatars (
			user_id, file_name, mime_type, size_bytes, s3_key, upload_status
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		avatar.UserID,
		avatar.FileName,
		avatar.MimeType,
		avatar.SizeBytes,
		avatar.S3Key,
		avatar.UploadStatus,
	).Scan(&avatar.ID, &avatar.CreatedAt, &avatar.UpdatedAt)

	if err != nil {
		return fmt.Errorf("create avatar: %w", err)
	}

	return nil
}

// GetByID получает аватарку по ID.
func (r *AvatarRepository) GetByID(ctx context.Context, id string) (*domain.Avatar, error) {
	const query = `
		SELECT id, user_id, file_name, mime_type, size_bytes, s3_key,
		       upload_status, created_at, updated_at, deleted_at
		FROM avatars
		WHERE id = $1 AND deleted_at IS NULL
	`

	var avatar domain.Avatar
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&avatar.ID,
		&avatar.UserID,
		&avatar.FileName,
		&avatar.MimeType,
		&avatar.SizeBytes,
		&avatar.S3Key,
		&avatar.UploadStatus,
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

	return &avatar, nil
}

// GetUserAvatar получает последнюю аватарку пользователя.
func (r *AvatarRepository) GetUserAvatar(ctx context.Context, userID string) (*domain.Avatar, error) {
	const query = `
		SELECT id, user_id, file_name, mime_type, size_bytes, s3_key,
		       upload_status, created_at, updated_at, deleted_at
		FROM avatars
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	var avatar domain.Avatar
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&avatar.ID,
		&avatar.UserID,
		&avatar.FileName,
		&avatar.MimeType,
		&avatar.SizeBytes,
		&avatar.S3Key,
		&avatar.UploadStatus,
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

	return &avatar, nil
}

// ListByUserID получает все аватарки пользователя.
func (r *AvatarRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.Avatar, error) {
	const query = `
		SELECT id, user_id, file_name, mime_type, size_bytes, s3_key,
		       upload_status, created_at, updated_at, deleted_at
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
		err := rows.Scan(
			&avatar.ID,
			&avatar.UserID,
			&avatar.FileName,
			&avatar.MimeType,
			&avatar.SizeBytes,
			&avatar.S3Key,
			&avatar.UploadStatus,
			&avatar.CreatedAt,
			&avatar.UpdatedAt,
			&avatar.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan avatar: %w", err)
		}
		avatars = append(avatars, &avatar)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate avatars: %w", err)
	}

	return avatars, nil
}

// Update обновляет аватарку.
func (r *AvatarRepository) Update(ctx context.Context, avatar *domain.Avatar) error {
	const query = `
		UPDATE avatars
		SET file_name = $2,
		    mime_type = $3,
		    size_bytes = $4,
		    s3_key = $5,
		    upload_status = $6
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query,
		avatar.ID,
		avatar.FileName,
		avatar.MimeType,
		avatar.SizeBytes,
		avatar.S3Key,
		avatar.UploadStatus,
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

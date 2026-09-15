// Package domain содержит бизнес-сущности
package domain

import "time"

// UploadStatus — статус загрузки файла в S3.
type UploadStatus string

const (
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusUploaded  UploadStatus = "uploaded"
	UploadStatusFailed    UploadStatus = "failed"
)

// Avatar — доменная модель аватарки пользователя (оригинальный файл).
type Avatar struct {
	ID           string       `json:"id"`
	UserID       string       `json:"user_id"`
	FileName     string       `json:"file_name"`
	MimeType     string       `json:"mime_type"`
	SizeBytes    int64        `json:"size_bytes"`
	S3Key        string       `json:"s3_key"`
	UploadStatus UploadStatus `json:"upload_status"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	DeletedAt    *time.Time   `json:"deleted_at,omitempty"`
}

// AvatarUploadedEvent — аватар успешно загружен.
type AvatarUploadedEvent struct {
	AvatarID   string    `json:"avatar_id"`
	UserID     string    `json:"user_id"`
	S3Key      string    `json:"s3_key"`
	OccurredAt time.Time `json:"occurred_at"`
}

// AvatarDeletedEvent — аватар мягко удалён.
type AvatarDeletedEvent struct {
	AvatarID   string    `json:"avatar_id"`
	UserID     string    `json:"user_id"`
	S3Keys     []string  `json:"s3_keys"`
	OccurredAt time.Time `json:"occurred_at"`
}

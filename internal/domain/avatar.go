// Package domain содержит бизнес-сущности
package domain

import "time"

// UploadStatus - статус загрузки файла в S3.
type UploadStatus string

const (
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusUploaded  UploadStatus = "uploaded"
	UploadStatusFailed    UploadStatus = "failed"
)

// ProcessingStatus - статус фоновой обработки (миниатюры, удаление).
type ProcessingStatus string

const (
	ProcessingStatusPending    ProcessingStatus = "pending"
	ProcessingStatusProcessing ProcessingStatus = "processing"
	ProcessingStatusCompleted  ProcessingStatus = "completed"
	ProcessingStatusFailed     ProcessingStatus = "failed"
)

// Avatar - доменная модель аватарки пользователя (оригинальный файл).
type Avatar struct {
	ID               string            `json:"id"`
	UserID           string            `json:"user_id"`
	FileName         string            `json:"file_name"`
	MimeType         string            `json:"mime_type"`
	SizeBytes        int64             `json:"size_bytes"`
	S3Key            string            `json:"s3_key"`
	Width            *int              `json:"width,omitempty"`
	Height           *int              `json:"height,omitempty"`
	ThumbnailS3Keys  map[string]string `json:"thumbnail_s3_keys,omitempty"`
	UploadStatus     UploadStatus      `json:"upload_status"`
	ProcessingStatus ProcessingStatus  `json:"processing_status"`
	Attempts         int               `json:"attempts"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	DeletedAt        *time.Time        `json:"deleted_at,omitempty"`
}

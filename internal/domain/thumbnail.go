package domain

import "time"

// ThumbnailSize — стандартный размер миниатюры.
type ThumbnailSize string

const (
	ThumbnailSize100x100 ThumbnailSize = "100x100"
	ThumbnailSize300x300 ThumbnailSize = "300x300"
)

// Thumbnail — доменная модель миниатюры аватарки.
type Thumbnail struct {
	ID        string        `json:"id"`
	AvatarID  string        `json:"avatar_id"`
	Size      ThumbnailSize `json:"size"`
	Width     int           `json:"width"`
	Height    int           `json:"height"`
	S3Key     string        `json:"s3_key"`
	SizeBytes int64         `json:"size_bytes"`
	CreatedAt time.Time     `json:"created_at"`
}

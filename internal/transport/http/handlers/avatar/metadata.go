package avatar

import (
	"sort"
	"time"

	"avatar-service/internal/domain"
)

// metadataResponse — JSON-ответ GET /api/v1/avatars/{id}/metadata.
type metadataResponse struct {
	ID         string              `json:"id"`
	UserID     string              `json:"user_id"`
	FileName   string              `json:"file_name"`
	MimeType   string              `json:"mime_type"`
	Size       int64               `json:"size"`
	Dimensions metadataDimensions  `json:"dimensions"`
	Thumbnails []metadataThumbnail `json:"thumbnails"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

// metadataDimensions — размеры оригинала.
type metadataDimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// metadataThumbnail — ссылка на готовую миниатюру.
type metadataThumbnail struct {
	Size string `json:"size"`
	URL  string `json:"url"`
}

// toMetadataResponse маппит domain.Avatar в JSON-контракт ТЗ.
func toMetadataResponse(avatar *domain.Avatar) metadataResponse {
	w, h := 0, 0
	if avatar.Width != nil {
		w = *avatar.Width
	}
	if avatar.Height != nil {
		h = *avatar.Height
	}

	sizes := make([]string, 0, len(avatar.ThumbnailS3Keys))
	for size := range avatar.ThumbnailS3Keys {
		sizes = append(sizes, size)
	}
	sort.Strings(sizes)

	thumbnails := make([]metadataThumbnail, 0, len(sizes))
	for _, size := range sizes {
		thumbnails = append(thumbnails, metadataThumbnail{
			Size: size,
			URL:  "/api/v1/avatars/" + avatar.ID + "?size=" + size,
		})
	}

	return metadataResponse{
		ID:         avatar.ID,
		UserID:     avatar.UserID,
		FileName:   avatar.FileName,
		MimeType:   avatar.MimeType,
		Size:       avatar.SizeBytes,
		Dimensions: metadataDimensions{Width: w, Height: h},
		Thumbnails: thumbnails,
		CreatedAt:  avatar.CreatedAt,
		UpdatedAt:  avatar.UpdatedAt,
	}
}

package avatar

import (
	"testing"
	"time"

	"avatar-service/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToMetadataResponse(t *testing.T) {
	t.Run("full avatar with thumbnails", func(t *testing.T) {
		width, height := 1920, 1080
		avatar := &domain.Avatar{
			ID:        "avatar-1",
			UserID:    "user-123",
			FileName:  "avatar.jpg",
			MimeType:  "image/jpeg",
			SizeBytes: 1024000,
			Width:     &width,
			Height:    &height,
			ThumbnailS3Keys: map[string]string{
				"300x300": "thumbnails/avatar-1/300x300.jpg",
				"100x100": "thumbnails/avatar-1/100x100.jpg",
			},
			CreatedAt: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 11, 2, 10, 5, 0, 0, time.UTC),
		}

		resp := toMetadataResponse(avatar)

		assert.Equal(t, "avatar-1", resp.ID)
		assert.Equal(t, "user-123", resp.UserID)
		assert.Equal(t, "avatar.jpg", resp.FileName)
		assert.Equal(t, "image/jpeg", resp.MimeType)
		assert.Equal(t, int64(1024000), resp.Size)
		assert.Equal(t, metadataDimensions{Width: 1920, Height: 1080}, resp.Dimensions)

		require.Len(t, resp.Thumbnails, 2)
		// Сортировка по size: 100x100 идёт первым.
		assert.Equal(t, "100x100", resp.Thumbnails[0].Size)
		assert.Equal(t, "/api/v1/avatars/avatar-1?size=100x100", resp.Thumbnails[0].URL)
		assert.Equal(t, "300x300", resp.Thumbnails[1].Size)
		assert.Equal(t, "/api/v1/avatars/avatar-1?size=300x300", resp.Thumbnails[1].URL)
	})

	t.Run("nil width/height -> 0/0", func(t *testing.T) {
		avatar := &domain.Avatar{
			ID:        "avatar-2",
			SizeBytes: 500,
			// Width, Height — nil (не декодировали при upload)
		}

		resp := toMetadataResponse(avatar)

		assert.Equal(t, metadataDimensions{Width: 0, Height: 0}, resp.Dimensions)
	})

	t.Run("nil thumbnails -> empty array (not null)", func(t *testing.T) {
		avatar := &domain.Avatar{
			ID:              "avatar-3",
			SizeBytes:       100,
			ThumbnailS3Keys: nil,
		}

		resp := toMetadataResponse(avatar)

		assert.NotNil(t, resp.Thumbnails, "thumbnails должен быть [] , не nil")
		assert.Empty(t, resp.Thumbnails)
	})

	t.Run("empty thumbnails map -> empty array", func(t *testing.T) {
		avatar := &domain.Avatar{
			ID:              "avatar-4",
			ThumbnailS3Keys: map[string]string{},
		}

		resp := toMetadataResponse(avatar)

		assert.NotNil(t, resp.Thumbnails)
		assert.Empty(t, resp.Thumbnails)
	})

	t.Run("thumbnails sorted by size", func(t *testing.T) {
		avatar := &domain.Avatar{
			ID: "avatar-5",
			ThumbnailS3Keys: map[string]string{
				"300x300": "k3",
				"100x100": "k1",
				"500x500": "k5",
			},
		}

		resp := toMetadataResponse(avatar)

		require.Len(t, resp.Thumbnails, 3)
		assert.Equal(t, "100x100", resp.Thumbnails[0].Size)
		assert.Equal(t, "300x300", resp.Thumbnails[1].Size)
		assert.Equal(t, "500x500", resp.Thumbnails[2].Size)
	})
}

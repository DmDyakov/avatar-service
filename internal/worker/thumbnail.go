package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"avatar-service/internal/domain"

	"github.com/disintegration/imaging"
	"go.uber.org/zap"
)

// ThumbnailRepository — минимальный интерфейс репозитория для воркера;
// реализуется *avatarrepo.AvatarRepository.
type ThumbnailRepository interface {
	UpdateThumbnailKeys(ctx context.Context, id string, keys map[string]string) error
	UpdateProcessingStatus(ctx context.Context, id string, status domain.ProcessingStatus) error
}

// ThumbnailStorage — минимальный интерфейс хранилища для воркера;
// реализуется *minio.Storage.
type ThumbnailStorage interface {
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
}

// ThumbnailHandler обрабатывает события AvatarUploadEvent: ресайз + S3 + БД.
type ThumbnailHandler struct {
	repo    ThumbnailRepository
	storage ThumbnailStorage
	logger  *zap.Logger
}

// NewThumbnailHandler создаёт обработчик с переданными зависимостями.
func NewThumbnailHandler(repo ThumbnailRepository, storage ThumbnailStorage, logger *zap.Logger) *ThumbnailHandler {
	return &ThumbnailHandler{repo: repo, storage: storage, logger: logger}
}

// Handle парсит AvatarUploadEvent из тела сообщения и запускает обработку.
func (h *ThumbnailHandler) Handle(ctx context.Context, body []byte) error {
	var event domain.AvatarUploadEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}
	return h.Process(ctx, event)
}

// Process — полный цикл: скачать оригинал, сделать миниатюры,
// загрузить в S3 и обновить метаданные в БД.
func (h *ThumbnailHandler) Process(ctx context.Context, event domain.AvatarUploadEvent) (err error) {
	reader, err := h.storage.Download(ctx, event.S3Key)
	if err != nil {
		return fmt.Errorf("download original: %w", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close reader: %w", closeErr)
		}
	}()

	img, err := imaging.Decode(reader, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	keys := make(map[string]string, 2)
	for _, size := range []int{100, 300} {
		thumb := imaging.Thumbnail(img, size, size, imaging.Lanczos)

		var buf bytes.Buffer
		if encErr := imaging.Encode(&buf, thumb, imaging.JPEG, imaging.JPEGQuality(85)); encErr != nil {
			return fmt.Errorf("encode %dx%d: %w", size, size, encErr)
		}

		key := fmt.Sprintf("thumbnails/%s/%dx%d.jpg", event.AvatarID, size, size)
		if upErr := h.storage.Upload(ctx, key, &buf, int64(buf.Len()), "image/jpeg"); upErr != nil {
			return fmt.Errorf("upload %dx%d: %w", size, size, upErr)
		}

		keys[fmt.Sprintf("%dx%d", size, size)] = key
	}

	if err := h.repo.UpdateThumbnailKeys(ctx, event.AvatarID, keys); err != nil {
		return fmt.Errorf("update thumbnail keys: %w", err)
	}
	if err := h.repo.UpdateProcessingStatus(ctx, event.AvatarID, domain.ProcessingStatusCompleted); err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}

	h.logger.Info("thumbnails generated",
		zap.String("avatar_id", event.AvatarID),
		zap.String("100x100", keys["100x100"]),
		zap.String("300x300", keys["300x300"]),
	)

	return nil
}

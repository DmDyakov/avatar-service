package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"avatar-service/internal/config"
	"avatar-service/internal/domain"

	"github.com/disintegration/imaging"
	"go.uber.org/zap"
)

// ThumbnailRepository — минимальный интерфейс репозитория для воркера;
// реализуется *avatarrepo.AvatarRepository.
type ThumbnailRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Avatar, error)
	UpdateThumbnailKeys(ctx context.Context, id string, keys map[string]string) error
	UpdateProcessingStatus(ctx context.Context, id string, status domain.ProcessingStatus) error
	IncrementAttempts(ctx context.Context, id string) error
}

// ThumbnailStorage — минимальный интерфейс хранилища для воркера;
// реализуется *minio.Storage.
type ThumbnailStorage interface {
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
}

// ThumbnailHandler обрабатывает события AvatarUploadEvent с retry.
// Идемпотентен: повторная обработка уже готовой аватарки — no-op.
type ThumbnailHandler struct {
	repo    ThumbnailRepository
	storage ThumbnailStorage
	logger  *zap.Logger
	cfg     config.WorkerConfig
}

// NewThumbnailHandler создаёт обработчик с переданными зависимостями.
func NewThumbnailHandler(
	repo ThumbnailRepository,
	storage ThumbnailStorage,
	logger *zap.Logger,
	cfg config.WorkerConfig,
) *ThumbnailHandler {
	return &ThumbnailHandler{repo: repo, storage: storage, logger: logger, cfg: cfg}
}

// Handle парсит AvatarUploadEvent из тела сообщения и запускает обработку.
func (h *ThumbnailHandler) Handle(ctx context.Context, body []byte) error {
	var event domain.AvatarUploadEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}
	return h.Process(ctx, event)
}

// Process — полный цикл с идемпотентностью, сменой статуса и retry.
//
//  1. Загружает avatar; если processing_status == completed — skip.
//  2. Переводит в processing (маркер «занято»).
//  3. processWithRetry: до RetryMaxAttempts попыток с exponential backoff.
//  4. При успехе — completed уже выставлен внутри process.
//  5. При исчерпании попыток — IncrementAttempts + failed, вернуть ошибку.
func (h *ThumbnailHandler) Process(ctx context.Context, event domain.AvatarUploadEvent) error {
	avatar, err := h.repo.GetByID(ctx, event.AvatarID)
	if err != nil {
		return fmt.Errorf("get avatar: %w", err)
	}

	if avatar.ProcessingStatus == domain.ProcessingStatusCompleted {
		h.logger.Info("avatar already completed, skipping",
			zap.String("avatar_id", event.AvatarID),
		)
		return nil
	}

	if err := h.repo.UpdateProcessingStatus(ctx, event.AvatarID, domain.ProcessingStatusProcessing); err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}

	if procErr := h.processWithRetry(ctx, event); procErr != nil {
		// Фиксируем попытку и статус failed ОДИН раз — после всех retry.
		if incErr := h.repo.IncrementAttempts(ctx, event.AvatarID); incErr != nil {
			h.logger.Error("increment attempts failed",
				zap.String("avatar_id", event.AvatarID),
				zap.Error(incErr),
			)
		}
		if stErr := h.repo.UpdateProcessingStatus(ctx, event.AvatarID, domain.ProcessingStatusFailed); stErr != nil {
			h.logger.Error("update status to failed failed",
				zap.String("avatar_id", event.AvatarID),
				zap.Error(stErr),
			)
		}
		return procErr
	}

	return nil
}

// processWithRetry — обёртка над process() с exponential backoff (ТЗ п. 2.2).
//
// Между попытками: select на ctx.Done и time.After(backoff).
// При отмене ctx — не ждём backoff, возвращаем последнюю ошибку.
// После последней попытки backoff НЕ применяется (нет смысла спать без следующей попытки).
func (h *ThumbnailHandler) processWithRetry(ctx context.Context, event domain.AvatarUploadEvent) error {
	backoff := h.cfg.RetryInitialBackoff
	var lastErr error

	for attempt := 1; attempt <= h.cfg.RetryMaxAttempts; attempt++ {
		if err := h.process(ctx, event); err == nil {
			return nil
		} else {
			lastErr = err
			h.logger.Warn("thumbnail attempt failed",
				zap.String("avatar_id", event.AvatarID),
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", h.cfg.RetryMaxAttempts),
				zap.Error(err),
			)
		}

		// Последняя попытка — не спим перед DLQ.
		if attempt == h.cfg.RetryMaxAttempts {
			break
		}

		h.logger.Info("retrying after backoff",
			zap.String("avatar_id", event.AvatarID),
			zap.Duration("backoff", backoff),
		)

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled during retry: %w", lastErr)
		case <-time.After(backoff):
		}

		backoff = time.Duration(float64(backoff) * h.cfg.RetryMultiplier)
	}

	return fmt.Errorf("all %d attempts failed: %w", h.cfg.RetryMaxAttempts, lastErr)
}

// process — download → resize → upload → update keys → completed.
func (h *ThumbnailHandler) process(ctx context.Context, event domain.AvatarUploadEvent) (err error) {
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
		return fmt.Errorf("update status to completed: %w", err)
	}

	h.logger.Info("thumbnails generated",
		zap.String("avatar_id", event.AvatarID),
		zap.String("100x100", keys["100x100"]),
		zap.String("300x300", keys["300x300"]),
	)

	return nil
}

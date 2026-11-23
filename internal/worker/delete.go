package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"avatar-service/internal/domain"

	"go.uber.org/zap"
)

// DeleteStorage — минимальный интерфейс хранилища для обработчика удаления.
// Реализуется *minio.Storage.
type DeleteStorage interface {
	Delete(ctx context.Context, key string) error
}

// DeleteHandler обрабатывает AvatarDeleteEvent: удаляет файлы из S3.
// Идемпотентен: отсутствующий ключ — не ошибка (файл уже удалён).
type DeleteHandler struct {
	storage DeleteStorage
	logger  *zap.Logger
}

// NewDeleteHandler создаёт обработчик с переданными зависимостями.
func NewDeleteHandler(storage DeleteStorage, logger *zap.Logger) *DeleteHandler {
	return &DeleteHandler{storage: storage, logger: logger}
}

// Handle парсит AvatarDeleteEvent из тела сообщения и запускает удаление.
func (h *DeleteHandler) Handle(ctx context.Context, body []byte) error {
	var event domain.AvatarDeleteEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}
	return h.Process(ctx, event)
}

// Process удаляет все S3-ключи из события (оригинал + миниатюры).
func (h *DeleteHandler) Process(ctx context.Context, event domain.AvatarDeleteEvent) error {
	var errs []error

	for _, key := range event.S3Keys {
		if err := h.storage.Delete(ctx, key); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				h.logger.Debug("s3 key already absent, skipping",
					zap.String("avatar_id", event.AvatarID),
					zap.String("s3_key", key),
				)
				continue
			}
			h.logger.Error("failed to delete s3 key",
				zap.String("avatar_id", event.AvatarID),
				zap.String("s3_key", key),
				zap.Error(err),
			)
			errs = append(errs, fmt.Errorf("delete %s: %w", key, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	h.logger.Info("avatar files deleted",
		zap.String("avatar_id", event.AvatarID),
		zap.Int("keys", len(event.S3Keys)),
	)
	return nil
}

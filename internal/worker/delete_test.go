package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"avatar-service/internal/domain"
	"avatar-service/internal/worker"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockDeleteStorage — мок DeleteStorage.
// failKeys: ключ → ошибка, которую вернуть при удалении.
// deletedKeys: собирает все ключи, по которым вызвали Delete.
type mockDeleteStorage struct {
	failKeys    map[string]error
	deletedKeys []string
	calls       int
}

func (m *mockDeleteStorage) Delete(ctx context.Context, key string) error {
	m.calls++
	m.deletedKeys = append(m.deletedKeys, key)
	if err, ok := m.failKeys[key]; ok {
		return err
	}
	return nil
}

func TestDeleteHandler_Process(t *testing.T) {
	t.Run("success - all keys deleted", func(t *testing.T) {
		storage := &mockDeleteStorage{}
		handler := worker.NewDeleteHandler(storage, zap.NewNop())

		event := domain.AvatarDeleteEvent{
			AvatarID: "avatar-1",
			S3Keys: []string{
				"avatars/user-1/avatar-1",
				"thumbnails/avatar-1/100x100.jpg",
				"thumbnails/avatar-1/300x300.jpg",
			},
		}

		err := handler.Process(context.Background(), event)
		require.NoError(t, err)

		assert.Equal(t, 3, storage.calls, "Delete вызван для каждого ключа")
		assert.Equal(t, event.S3Keys, storage.deletedKeys)
	})

	t.Run("empty S3Keys - no calls, no error", func(t *testing.T) {
		storage := &mockDeleteStorage{}
		handler := worker.NewDeleteHandler(storage, zap.NewNop())

		event := domain.AvatarDeleteEvent{AvatarID: "avatar-2", S3Keys: nil}

		err := handler.Process(context.Background(), event)
		require.NoError(t, err)
		assert.Equal(t, 0, storage.calls, "Delete не должен вызываться при пустом массиве")
	})

	t.Run("ErrNotFound is skipped (idempotency)", func(t *testing.T) {
		storage := &mockDeleteStorage{
			failKeys: map[string]error{
				"thumbnails/avatar-3/100x100.jpg": domain.ErrNotFound,
			},
		}
		handler := worker.NewDeleteHandler(storage, zap.NewNop())

		event := domain.AvatarDeleteEvent{
			AvatarID: "avatar-3",
			S3Keys: []string{
				"avatars/user-1/avatar-3",
				"thumbnails/avatar-3/100x100.jpg", // уже отсутствует
				"thumbnails/avatar-3/300x300.jpg",
			},
		}

		err := handler.Process(context.Background(), event)
		require.NoError(t, err, "ErrNotFound — не ошибка (файл уже удалён)")

		assert.Equal(t, 3, storage.calls, "Delete вызван для каждого ключа, включая отсутствующий")
	})

	t.Run("real error collected via errors.Join", func(t *testing.T) {
		storage := &mockDeleteStorage{
			failKeys: map[string]error{
				"avatars/user-1/avatar-4":         errors.New("s3 timeout"),
				"thumbnails/avatar-4/300x300.jpg": errors.New("access denied"),
			},
		}
		handler := worker.NewDeleteHandler(storage, zap.NewNop())

		event := domain.AvatarDeleteEvent{
			AvatarID: "avatar-4",
			S3Keys: []string{
				"avatars/user-1/avatar-4",         // ошибка
				"thumbnails/avatar-4/100x100.jpg", // ок
				"thumbnails/avatar-4/300x300.jpg", // ошибка
			},
		}

		err := handler.Process(context.Background(), event)
		require.Error(t, err)

		// Все три попытки были (даже после ошибок — не останавливаемся на первой).
		assert.Equal(t, 3, storage.calls)

		// errors.Join: обе ошибки в цепочке.
		assert.Contains(t, err.Error(), "s3 timeout")
		assert.Contains(t, err.Error(), "access denied")
		assert.Contains(t, err.Error(), "avatar-4") // из fmt.Errorf("delete %s: ...", key, ...)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		storage := &mockDeleteStorage{}
		handler := worker.NewDeleteHandler(storage, zap.NewNop())

		event := domain.AvatarDeleteEvent{
			AvatarID: "avatar-5",
			S3Keys:   []string{"avatars/user-1/avatar-5"},
		}
		body, err := json.Marshal(event)
		require.NoError(t, err)

		require.NoError(t, handler.Handle(context.Background(), body))
		assert.Equal(t, 1, storage.calls)
		assert.Equal(t, "avatars/user-1/avatar-5", storage.deletedKeys[0])
	})

	t.Run("invalid JSON", func(t *testing.T) {
		storage := &mockDeleteStorage{}
		handler := worker.NewDeleteHandler(storage, zap.NewNop())

		err := handler.Handle(context.Background(), []byte("not json"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal event")
		assert.Equal(t, 0, storage.calls, "storage не вызывается при ошибке парсинга")
	})

	t.Run("empty body", func(t *testing.T) {
		storage := &mockDeleteStorage{}
		handler := worker.NewDeleteHandler(storage, zap.NewNop())

		err := handler.Handle(context.Background(), []byte{})
		require.Error(t, err)
		assert.Equal(t, 0, storage.calls)
	})
}

// Гарантия, что мок соответствует интерфейсу worker.DeleteStorage.
var _ worker.DeleteStorage = (*mockDeleteStorage)(nil)

// Хелпер для многострочных ошибок в будущем (если понадобится).
func _formatErrs(errs []error) string {
	return fmt.Sprintf("%v", errs)
}

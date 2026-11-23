package health

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(ctx context.Context) error {
	return m.err
}

func TestService_Check(t *testing.T) {
	t.Run("all components ok", func(t *testing.T) {
		s := New()
		s.Register("postgres", &mockPinger{})
		s.Register("storage", &mockPinger{})
		s.Register("rabbitmq", &mockPinger{})

		status, code := s.Check(context.Background())

		assert.Equal(t, 200, code)
		assert.Equal(t, "ok", status.Status)
		assert.Equal(t, "ok", status.Components["postgres"])
		assert.Equal(t, "ok", status.Components["storage"])
		assert.Equal(t, "ok", status.Components["rabbitmq"])
	})

	t.Run("rabbitmq degraded - 503", func(t *testing.T) {
		s := New()
		s.Register("postgres", &mockPinger{})
		s.Register("storage", &mockPinger{})
		s.Register("rabbitmq", &mockPinger{err: errors.New("connection is closed")})

		status, code := s.Check(context.Background())

		assert.Equal(t, 503, code)
		assert.Equal(t, "degraded", status.Status)
		assert.Equal(t, "ok", status.Components["postgres"])
		assert.Equal(t, "ok", status.Components["storage"])
		assert.Contains(t, status.Components["rabbitmq"], "error:")
	})

	t.Run("postgres degraded", func(t *testing.T) {
		s := New()
		s.Register("postgres", &mockPinger{err: errors.New("db down")})
		s.Register("storage", &mockPinger{})
		s.Register("rabbitmq", &mockPinger{})

		status, code := s.Check(context.Background())

		assert.Equal(t, 503, code)
		assert.Equal(t, "degraded", status.Status)
		assert.Contains(t, status.Components["postgres"], "error:")
	})

	t.Run("empty registry - ok", func(t *testing.T) {
		s := New()

		status, code := s.Check(context.Background())

		assert.Equal(t, 200, code)
		assert.Equal(t, "ok", status.Status)
		assert.Empty(t, status.Components)
	})
}

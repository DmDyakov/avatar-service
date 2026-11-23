package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockApp struct {
	runErr      error
	shouldBlock bool
	sleepAfter  time.Duration
}

func (m *mockApp) Run(ctx context.Context) error {
	if m.shouldBlock {
		<-ctx.Done()
		if m.sleepAfter > 0 {
			time.Sleep(m.sleepAfter)
		}
		return ctx.Err()
	}
	return m.runErr
}

func TestRun(t *testing.T) {
	t.Run("app completes before cancel", func(t *testing.T) {
		app := &mockApp{}
		ctx := context.Background()
		err := Run(ctx, app, time.Second)
		assert.NoError(t, err)
	})

	t.Run("app returns error", func(t *testing.T) {
		expectedErr := errors.New("app error")
		app := &mockApp{runErr: expectedErr}
		ctx := context.Background()
		err := Run(ctx, app, time.Second)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("context.Canceled is filtered on first select", func(t *testing.T) {
		app := &mockApp{runErr: context.Canceled}
		ctx := context.Background()
		err := Run(ctx, app, time.Second)
		assert.NoError(t, err)
	})

	t.Run("graceful shutdown on context cancel", func(t *testing.T) {
		app := &mockApp{shouldBlock: true}
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := Run(ctx, app, time.Second)
		assert.NoError(t, err)
	})

	t.Run("context.Canceled is filtered on second select", func(t *testing.T) {
		app := &mockApp{shouldBlock: true}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := Run(ctx, app, time.Second)
		assert.NoError(t, err)
	})

	t.Run("timeout exceeded during shutdown", func(t *testing.T) {
		app := &mockApp{
			shouldBlock: true,
			sleepAfter:  time.Second,
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := Run(ctx, app, 50*time.Millisecond)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// Package lifecycle provides application startup and graceful shutdown orchestration.
package lifecycle

import (
	"context"
	"errors"
	"time"
)

// Application defines the application lifecycle.
type Application interface {
	Run(ctx context.Context) error
}

// Run starts the application and gracefully shuts it down when ctx is canceled.
func Run(ctx context.Context, app Application, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- app.Run(ctx)
	}()

	select {
	case err := <-done:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err

	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case err := <-done:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err

	case <-shutdownCtx.Done():
		return context.DeadlineExceeded
	}
}

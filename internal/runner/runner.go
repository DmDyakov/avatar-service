// Package runner содержит общий bootstrap для запуска сервисов.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"avatar-service/internal/config"
	"avatar-service/internal/logger"
	"avatar-service/pkg/buildinfo"

	"go.uber.org/zap"
)

// App — интерфейс запускаемого сервиса.
type App interface {
	Run(ctx context.Context) error
}

// Run выполняет общий запуск сервиса: загружает конфиг, создаёт логгер,
// собирает приложение через newApp и запускает его до остановки по сигналу.
func Run[T App](name string, newApp func(*config.Config, *zap.Logger) (T, error)) error {
	buildinfo.Print()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log, cleanup, err := logger.New(cfg.IsProd(), cfg.Logger)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "logger cleanup failed: %v\n", err)
		}
	}()

	app, err := newApp(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", name, err)
	}

	if err := app.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s terminated with error: %w", name, err)
	}

	return nil
}

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"avatar-service/internal/app"
	"avatar-service/internal/config"
	"avatar-service/internal/logger"
	"avatar-service/pkg/buildinfo"
	"avatar-service/pkg/lifecycle"
)

func run() error {
	buildinfo.Print()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger, err := logger.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) {
			fmt.Fprintf(os.Stderr, "failed to sync logger: %v\n", err)
		}
	}()

	app, err := app.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	if err := lifecycle.Run(ctx, app, cfg.ShutdownTimeout); err != nil {
		return fmt.Errorf("app terminated with error: %w", err)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

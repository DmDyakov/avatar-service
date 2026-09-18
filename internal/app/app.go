// Package app собирает и запускает сервер.
package app

import (
	"avatar-service/internal/config"
	"avatar-service/internal/publisher"
	"avatar-service/internal/storage/minio"

	"avatar-service/internal/repository/postgres"
	httpserver "avatar-service/internal/transport/http"

	healthservice "avatar-service/internal/services/health"
	healthhandler "avatar-service/internal/transport/http/handlers/health"

	avatarrepo "avatar-service/internal/repository/postgres/avatar"
	avatarservice "avatar-service/internal/services/avatar"
	avatarhandler "avatar-service/internal/transport/http/handlers/avatar"

	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// App управляет жизненным циклом сервера.
type App struct {
	cfg        *config.Config
	logger     *zap.Logger
	pgPool     *pgxpool.Pool
	httpServer *httpserver.Server
	storage    *minio.Storage
}

// New создаёт новый App.
func New(cfg *config.Config, logger *zap.Logger) (*App, error) {
	pgPool, err := postgres.NewPool(cfg.Postgres, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize postgres: %w", err)
	}

	storage, err := minio.NewStorage(cfg.S3)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	pub := publisher.NewPublisher()

	healthService := healthservice.New()
	healthService.Register("postgres", pgPool)
	healthService.Register("storage", storage)
	healthHandler := healthhandler.NewHealthHandler(healthService, logger)

	avatarRepo := avatarrepo.NewAvatarRepository(pgPool)
	avatarService := avatarservice.New(avatarRepo, storage, pub, logger)
	avatarHandler := avatarhandler.NewAvatarHandler(avatarService, logger)

	httpServer := httpserver.New(
		&cfg.HTTPServer,
		healthHandler,
		avatarHandler,
		logger,
	)

	return &App{
		cfg:        cfg,
		logger:     logger,
		pgPool:     pgPool,
		httpServer: httpServer,
		storage:    storage,
	}, nil
}

// Run запускает сервер.
func (a *App) Run(ctx context.Context) error {
	defer a.pgPool.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.httpServer.Run(ctx)
	})

	return g.Wait()
}

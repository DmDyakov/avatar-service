// Package app собирает и запускает сервер.
package app

import (
	"avatar-service/internal/config"
	"avatar-service/internal/publisher"

	"avatar-service/internal/repository/postgres"
	"avatar-service/internal/storage"
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
	cfg    *config.Config
	logger *zap.Logger
	pgPool *pgxpool.Pool
	http   *httpserver.Server
}

// New создаёт новый App.
func New(cfg *config.Config, logger *zap.Logger) (*App, error) {
	pgPool, err := postgres.NewPool(cfg.Postgres, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize postgres: %w", err)
	}

	storage := storage.NewStorage()
	pub := publisher.NewPublisher()

	healthService := healthservice.New(pgPool)
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
		cfg:    cfg,
		logger: logger,
		pgPool: pgPool,
		http:   httpServer,
	}, nil
}

// Run запускает сервер.
func (a *App) Run(ctx context.Context) error {
	defer a.pgPool.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.http.Run(ctx)
	})

	return g.Wait()
}

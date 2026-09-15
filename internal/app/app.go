// Package app собирает и запускает сервер.
package app

import (
	"avatar-service/internal/config"
	"avatar-service/internal/publisher"
	"avatar-service/internal/repository"
	postgres "avatar-service/internal/repository/postgres"
	"avatar-service/internal/storage"

	avatarservice "avatar-service/internal/services/avatar"
	healthservice "avatar-service/internal/services/health"
	httpserver "avatar-service/internal/transport/http"
	"avatar-service/internal/transport/http/handlers"
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
	pg     *pgxpool.Pool
	http   *httpserver.Server
}

// New создаёт новый App.
func New(cfg *config.Config, logger *zap.Logger) (*App, error) {
	pg, err := postgres.NewPool(cfg.Postgres, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize postgres: %w", err)
	}

	storage := storage.NewStorage()
	pub := publisher.NewPublisher()

	healthService := healthservice.New(pg)
	healthHandler := handlers.NewHealthHandler(healthService, logger)

	avatarRepo := repository.NewAvatarRepository()
	avatarService := avatarservice.New(avatarRepo, storage, pub, logger)
	avatarHandler := handlers.NewAvatarHandler(avatarService, logger)

	httpServer := httpserver.New(
		&cfg.HTTPServer,
		healthHandler,
		avatarHandler,
		logger,
	)

	return &App{
		cfg:    cfg,
		logger: logger,
		pg:     pg,
		http:   httpServer,
	}, nil
}

// Run запускает сервер.
func (a *App) Run(ctx context.Context) error {
	defer a.pg.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.http.Run(ctx)
	})

	return g.Wait()
}

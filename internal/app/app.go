// Package app собирает зависимости и запускает сервер.
package app

import (
	"context"
	"fmt"

	"avatar-service/internal/config"
	"avatar-service/internal/storage/minio"

	"avatar-service/internal/repository/postgres"
	"avatar-service/internal/repository/postgres/outbox"
	httpserver "avatar-service/internal/transport/http"

	healthservice "avatar-service/internal/services/health"
	healthhandler "avatar-service/internal/transport/http/handlers/health"

	avatarrepo "avatar-service/internal/repository/postgres/avatar"
	avatarservice "avatar-service/internal/services/avatar"
	avatarhandler "avatar-service/internal/transport/http/handlers/avatar"

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

func New(cfg *config.Config, logger *zap.Logger) (*App, error) {
	pgPool, err := postgres.NewPool(cfg.Postgres, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize postgres: %w", err)
	}

	storage, err := minio.NewStorage(cfg.S3)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	healthService := healthservice.New()
	healthService.Register("postgres", pgPool)
	healthService.Register("storage", storage)

	healthHandler := healthhandler.NewHealthHandler(healthService, logger)

	txMgr := postgres.NewTxManager(pgPool)
	outboxRepo := outbox.NewRepository(pgPool)

	avatarRepo := avatarrepo.NewAvatarRepository(pgPool)
	avatarService := avatarservice.New(txMgr, avatarRepo, storage, outboxRepo, logger)
	avatarHandler := avatarhandler.NewAvatarHandler(avatarService, logger)

	httpServer := httpserver.New(
		&cfg.HTTPServer,
		cfg.RateLimit,
		cfg.CORS,
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

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return a.httpServer.Run(gctx)
	})
	return g.Wait()
}

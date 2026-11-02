package app

import (
	"context"
	"fmt"

	"avatar-service/internal/broker/rabbitmq"
	"avatar-service/internal/config"
	"avatar-service/internal/repository/postgres"
	avatarrepo "avatar-service/internal/repository/postgres/avatar"
	"avatar-service/internal/storage/minio"
	"avatar-service/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// WorkerApp — жизненный цикл воркера (consumer RabbitMQ).
type WorkerApp struct {
	cfg      *config.Config
	logger   *zap.Logger
	pgPool   *pgxpool.Pool
	consumer *rabbitmq.Consumer
	handler  *worker.ThumbnailHandler
}

// NewWorker собирает зависимости воркера: pool, storage, consumer, handler.
func NewWorker(cfg *config.Config, logger *zap.Logger) (*WorkerApp, error) {
	pgPool, err := postgres.NewPool(cfg.Postgres, logger)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}

	storage, err := minio.NewStorage(cfg.S3)
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("storage: %w", err)
	}

	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQ, logger)
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("rabbitmq consumer: %w", err)
	}

	repo := avatarrepo.NewAvatarRepository(pgPool)
	handler := worker.NewThumbnailHandler(repo, storage, logger, cfg.Worker)

	return &WorkerApp{
		cfg:      cfg,
		logger:   logger,
		pgPool:   pgPool,
		consumer: consumer,
		handler:  handler,
	}, nil
}

// Run запускает consumer и блокируется до отмены ctx.
func (a *WorkerApp) Run(ctx context.Context) (err error) {
	defer func() {
		a.pgPool.Close()

		if closeErr := a.consumer.Close(); closeErr != nil {
			a.logger.Error("consumer close failed", zap.Error(closeErr))
			if err == nil {
				err = fmt.Errorf("close consumer: %w", closeErr)
			}
		}
	}()

	return a.consumer.Run(ctx, a.handler.Handle)
}

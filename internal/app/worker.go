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
	"golang.org/x/sync/errgroup"
)

// WorkerApp — жизненный цикл воркера (consumer RabbitMQ).
type WorkerApp struct {
	cfg               *config.Config
	logger            *zap.Logger
	pgPool            *pgxpool.Pool
	thumbnailConsumer *rabbitmq.Consumer
	deleteConsumer    *rabbitmq.Consumer
	thumbnailHandler  *worker.ThumbnailHandler
	deleteHandler     *worker.DeleteHandler
}

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

	thumbnailConsumer, err := rabbitmq.NewConsumer(cfg.RabbitMQ, logger, rabbitmq.ThumbnailQueueName)
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("rabbitmq thumbnail consumer: %w", err)
	}

	deleteConsumer, err := rabbitmq.NewConsumer(cfg.RabbitMQ, logger, rabbitmq.DeleteQueueName)
	if err != nil {
		pgPool.Close()
		_ = thumbnailConsumer.Close()
		return nil, fmt.Errorf("rabbitmq delete consumer: %w", err)
	}

	repo := avatarrepo.NewAvatarRepository(pgPool)
	thumbnailHandler := worker.NewThumbnailHandler(repo, storage, logger, cfg.Worker)
	deleteHandler := worker.NewDeleteHandler(storage, logger)

	return &WorkerApp{
		cfg:               cfg,
		logger:            logger,
		pgPool:            pgPool,
		thumbnailConsumer: thumbnailConsumer,
		deleteConsumer:    deleteConsumer,
		thumbnailHandler:  thumbnailHandler,
		deleteHandler:     deleteHandler,
	}, nil
}

// Run запускает consumer'ы и блокируется до отмены ctx.
func (a *WorkerApp) Run(ctx context.Context) error {
	defer a.pgPool.Close()
	defer func() {
		if err := a.thumbnailConsumer.Close(); err != nil {
			a.logger.Error("thumbnail consumer close failed", zap.Error(err))
		}
		if err := a.deleteConsumer.Close(); err != nil {
			a.logger.Error("delete consumer close failed", zap.Error(err))
		}
	}()

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.thumbnailConsumer.Run(gctx, a.thumbnailHandler.Handle)
	})
	g.Go(func() error {
		return a.deleteConsumer.Run(gctx, a.deleteHandler.Handle)
	})

	return g.Wait()
}

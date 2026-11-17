package app

import (
	"context"
	"fmt"

	"avatar-service/internal/broker/rabbitmq"
	"avatar-service/internal/config"
	"avatar-service/internal/relay"
	"avatar-service/internal/repository/postgres"
	"avatar-service/internal/repository/postgres/outbox"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// RelayApp — жизненный цикл outbox relay (публикация событий в RabbitMQ).
type RelayApp struct {
	cfg       *config.Config
	logger    *zap.Logger
	pgPool    *pgxpool.Pool
	publisher *rabbitmq.Publisher
	relay     *relay.Relay
}

func NewRelay(cfg *config.Config, logger *zap.Logger) (*RelayApp, error) {
	pgPool, err := postgres.NewPool(cfg.Postgres, logger)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}

	publisher, err := rabbitmq.NewPublisher(cfg.RabbitMQ, logger)
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("rabbitmq publisher: %w", err)
	}

	outboxRepo := outbox.NewRepository(pgPool)
	outboxRelay := relay.New(relay.Config{
		BatchSize:        cfg.Outbox.BatchSize,
		MaxAttempts:      cfg.Outbox.MaxAttempts,
		PollInterval:     cfg.Outbox.PollInterval,
		StaleLockTimeout: cfg.Outbox.StaleLockTimeout,
		StaleSweepPeriod: cfg.Outbox.StaleSweepPeriod,
	}, outboxRepo, publisher, logger)

	return &RelayApp{
		cfg:       cfg,
		logger:    logger,
		pgPool:    pgPool,
		publisher: publisher,
		relay:     outboxRelay,
	}, nil
}

func (a *RelayApp) Run(ctx context.Context) error {
	defer a.pgPool.Close()
	defer func() {
		if err := a.publisher.Close(); err != nil {
			a.logger.Error("publisher close failed", zap.Error(err))
		}
	}()

	return a.relay.Run(ctx)
}

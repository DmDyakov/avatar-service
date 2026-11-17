// Package relay доставляет события из outbox_events в брокер.
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"avatar-service/internal/domain"
	"avatar-service/internal/repository/postgres/outbox"

	"go.uber.org/zap"
)

// Publisher — куда публикуем. Совпадает с EventPublisher в сервисе
// до рефакторинга.
type Publisher interface {
	PublishUploadEvent(ctx context.Context, event domain.AvatarUploadEvent) error
	PublishDeleteEvent(ctx context.Context, event domain.AvatarDeleteEvent) error
}

type Config struct {
	WorkerID         string        // для locked_by; по умолчанию hostname
	BatchSize        int           // сколько событий забирать за цикл
	MaxAttempts      int           // после N неудач событие permanent failed
	PollInterval     time.Duration // частота polling
	StaleLockTimeout time.Duration // через сколько in_progress считается зависшим
	StaleSweepPeriod time.Duration // как часто чистить зависшие
}

type Relay struct {
	cfg       Config
	repo      *outbox.Repository
	publisher Publisher
	logger    *zap.Logger
}

func New(cfg Config, repo *outbox.Repository, pub Publisher, logger *zap.Logger) *Relay {
	if cfg.WorkerID == "" {
		host, _ := os.Hostname()
		cfg.WorkerID = host
	}
	return &Relay{cfg: cfg, repo: repo, publisher: pub, logger: logger}
}

// Run блокируется до отмены ctx.
func (r *Relay) Run(ctx context.Context) error {
	r.logger.Info("outbox relay started",
		zap.String("worker_id", r.cfg.WorkerID),
		zap.Duration("poll_interval", r.cfg.PollInterval),
	)

	go r.staleSweep(ctx)

	// Первый drain — сразу, не ждём первого тика.
	if err := r.drain(ctx); err != nil && !errors.Is(err, context.Canceled) {
		r.logger.Error("initial outbox drain failed", zap.Error(err))
	}

	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.drain(ctx); err != nil && !errors.Is(err, context.Canceled) {
				r.logger.Error("outbox drain failed", zap.Error(err))
			}
		}
	}
}

// drain забирает пачки, пока не опустеет.
func (r *Relay) drain(ctx context.Context) error {
	for {
		events, err := r.repo.ClaimBatch(ctx, r.cfg.WorkerID, r.cfg.BatchSize, r.cfg.MaxAttempts)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		for _, e := range events {
			if err := r.publish(ctx, e); err != nil {
				r.logger.Warn("outbox publish failed",
					zap.String("event_id", e.ID),
					zap.String("type", string(e.Type)),
					zap.Int("attempt", e.Attempts),
					zap.Int("max", r.cfg.MaxAttempts),
					zap.Error(err),
				)
				if mErr := r.repo.MarkFailed(ctx, e.ID, err.Error()); mErr != nil {
					r.logger.Error("mark failed", zap.String("event_id", e.ID), zap.Error(mErr))
				}
				continue
			}
			if err := r.repo.MarkPublished(ctx, e.ID); err != nil {
				r.logger.Error("mark published", zap.String("event_id", e.ID), zap.Error(err))
			}
		}

		if len(events) < r.cfg.BatchSize {
			return nil
		}
	}
}

func (r *Relay) publish(ctx context.Context, e outbox.Event) error {
	switch e.Type {
	case outbox.EventTypeUpload:
		var ev domain.AvatarUploadEvent
		if err := json.Unmarshal(e.Payload, &ev); err != nil {
			return fmt.Errorf("unmarshal upload: %w", err)
		}
		return r.publisher.PublishUploadEvent(ctx, ev)
	case outbox.EventTypeDelete:
		var ev domain.AvatarDeleteEvent
		if err := json.Unmarshal(e.Payload, &ev); err != nil {
			return fmt.Errorf("unmarshal delete: %w", err)
		}
		return r.publisher.PublishDeleteEvent(ctx, ev)
	default:
		return fmt.Errorf("unknown event type: %s", e.Type)
	}
}

func (r *Relay) staleSweep(ctx context.Context) {
	t := time.NewTicker(r.cfg.StaleSweepPeriod)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := r.repo.ReleaseStale(ctx, r.cfg.StaleLockTimeout)
			if err != nil {
				r.logger.Error("stale sweep failed", zap.Error(err))
			} else if n > 0 {
				r.logger.Info("released stale outbox events", zap.Int64("count", n))
			}
		}
	}
}

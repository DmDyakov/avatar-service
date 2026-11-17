// Package outbox реализует transactional outbox для надёжной публикации
// событий в брокер: запись в таблицу идёт в той же транзакции, что и
// бизнес-изменения, а отдельный relay-процесс доставляет их в RabbitMQ.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusPublished  Status = "published"
	StatusFailed     Status = "failed"
)

type EventType = string

const (
	EventTypeUpload EventType = "avatar.upload"
	EventTypeDelete EventType = "avatar.delete"
)

// Event — строка из outbox_events.
type Event struct {
	ID            string
	Type          EventType
	Payload       json.RawMessage
	Status        Status
	Attempts      int
	LockedAt      *time.Time
	LockedBy      *string
	FailureReason *string
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Write пишет событие в outbox внутри переданной транзакции.
// Вызывается из сервиса вместе с Create/SoftDelete.
func (r *Repository) Write(ctx context.Context, tx pgx.Tx, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	const q = `INSERT INTO outbox_events (event_type, payload) VALUES ($1, $2)`
	if _, err := tx.Exec(ctx, q, eventType, data); err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// ClaimBatch забирает до limit строк через FOR UPDATE SKIP LOCKED:
// безопасно для нескольких relay-инстансов.
func (r *Repository) ClaimBatch(ctx context.Context, workerID string, limit, maxAttempts int) ([]Event, error) {
	const q = `
		UPDATE outbox_events
		SET status = 'in_progress',
		    locked_at = NOW(),
		    locked_by = $1,
		    attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM outbox_events
			WHERE status IN ('pending', 'failed')
			  AND attempts < $2
			ORDER BY created_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, payload, status, attempts,
		          locked_at, locked_by, failure_reason, created_at, published_at
	`
	rows, err := r.pool.Query(ctx, q, workerID, maxAttempts, limit)
	if err != nil {
		return nil, fmt.Errorf("claim batch: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.ID, &e.Type, &e.Payload, &e.Status, &e.Attempts,
			&e.LockedAt, &e.LockedBy, &e.FailureReason,
			&e.CreatedAt, &e.PublishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// MarkPublished — успешная публикация.
func (r *Repository) MarkPublished(ctx context.Context, id string) error {
	const q = `
		UPDATE outbox_events
		SET status = 'published',
		    published_at = NOW(),
		    locked_at = NULL, locked_by = NULL, failure_reason = NULL
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// MarkFailed — публикация не удалась. attempts уже инкрементирован
// в ClaimBatch; после attempts >= maxAttempts событие перестаёт
// выбираться (permanent failed).
func (r *Repository) MarkFailed(ctx context.Context, id, reason string) error {
	const q = `
		UPDATE outbox_events
		SET status = 'failed',
		    failure_reason = $2,
		    locked_at = NULL, locked_by = NULL
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, q, id, reason)
	return err
}

// ReleaseStale возвращает «зависшие» in_progress в pending. Нужен,
// если relay упал после ClaimBatch и до Mark*.
func (r *Repository) ReleaseStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	const q = `
		UPDATE outbox_events
		SET status = 'pending', locked_at = NULL, locked_by = NULL
		WHERE status = 'in_progress'
		  AND locked_at < NOW() - $1::interval
	`
	tag, err := r.pool.Exec(ctx, q, olderThan)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Requeue — ручной перезапуск permanent failed (admin-путь).
func (r *Repository) Requeue(ctx context.Context, id string) error {
	const q = `
		UPDATE outbox_events
		SET status = 'pending', attempts = 0,
		    failure_reason = NULL, locked_at = NULL, locked_by = NULL
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

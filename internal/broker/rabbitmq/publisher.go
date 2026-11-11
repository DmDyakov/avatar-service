package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"avatar-service/internal/config"
	"avatar-service/internal/domain"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Publisher публикует события в RabbitMQ.
// Автоматическое переподключение и восстановление топологии
// делегировано встроенному механизму amqp091-go.
type Publisher struct {
	logger *zap.Logger

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
}

// NewPublisher создаёт подключение с включённым авто-восстановлением
// и объявляет топологию через общий Declare.
func NewPublisher(cfg config.RabbitMQConfig, logger *zap.Logger) (*Publisher, error) {
	conn, err := amqp.DialConfig(cfg.URL, amqp.Config{
		Heartbeat: cfg.Heartbeat,
		Dial:      amqp.DefaultDial(cfg.ConnectTimeout),
		Recovery:  &amqp.Recovery{},
	})
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil && closeErr != amqp.ErrClosed {
			return nil, errors.Join(
				fmt.Errorf("open channel: %w", err),
				fmt.Errorf("close conn: %w", closeErr),
			)
		}
		return nil, fmt.Errorf("open channel: %w", err)
	}

	p := &Publisher{
		logger:  logger,
		conn:    conn,
		channel: ch,
	}

	if err := p.declareTopology(); err != nil {
		if closeErr := p.Close(); closeErr != nil {
			return nil, errors.Join(
				fmt.Errorf("declare topology: %w", err),
				fmt.Errorf("close publisher: %w", closeErr),
			)
		}
		return nil, fmt.Errorf("declare topology: %w", err)
	}

	logger.Info("rabbitmq publisher initialized with auto-recovery",
		zap.String("exchange", ExchangeName),
		zap.String("queue", ThumbnailQueueName),
		zap.String("dlx", DLXName),
		zap.String("dlq", DLQName),
	)

	return p, nil
}

// declareTopology объявляет топологию через общий Declare.
func (p *Publisher) declareTopology() error {
	return Declare(p.channel)
}

// PublishUploadEvent публикует событие о загрузке аватарки.
func (p *Publisher) PublishUploadEvent(ctx context.Context, event domain.AvatarUploadEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	p.mu.Lock()
	ch := p.channel
	p.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("channel is not initialized")
	}

	err = ch.PublishWithContext(
		ctx,
		ExchangeName,
		UploadRoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
			MessageId:    event.AvatarID,
			Timestamp:    time.Now(),
		},
	)
	if err != nil {
		return fmt.Errorf("publish %s: %w", UploadRoutingKey, err)
	}

	p.logger.Debug("upload event published", zap.String("avatar_id", event.AvatarID))
	return nil
}

// Ping проверяет, что соединение живо; используется в health-check.
func (p *Publisher) Ping(ctx context.Context) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil || conn.IsClosed() {
		return fmt.Errorf("connection is closed")
	}
	return nil
}

// Close закрывает канал и соединение; ошибки объединяются через errors.Join.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	if p.channel != nil {
		if err := p.channel.Close(); err != nil && err != amqp.ErrClosed {
			firstErr = fmt.Errorf("close channel: %w", err)
		}
		p.channel = nil
	}
	if p.conn != nil {
		if err := p.conn.Close(); err != nil && err != amqp.ErrClosed {
			if firstErr == nil {
				firstErr = fmt.Errorf("close connection: %w", err)
			} else {
				firstErr = errors.Join(firstErr, fmt.Errorf("close connection: %w", err))
			}
		}
		p.conn = nil
	}

	p.logger.Info("rabbitmq publisher closed")
	return firstErr
}

// PublishDeleteEvent публикует событие об удалении аватарки.
func (p *Publisher) PublishDeleteEvent(ctx context.Context, event domain.AvatarDeleteEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	p.mu.Lock()
	ch := p.channel
	p.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("channel is not initialized")
	}

	err = ch.PublishWithContext(
		ctx,
		ExchangeName,
		"avatar.delete.created",
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
			MessageId:    event.AvatarID,
			Timestamp:    time.Now(),
		},
	)
	if err != nil {
		return fmt.Errorf("publish avatar.delete.created: %w", err)
	}

	p.logger.Debug("delete event published", zap.String("avatar_id", event.AvatarID))
	return nil
}

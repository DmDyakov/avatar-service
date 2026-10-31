package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"avatar-service/internal/config"
	"avatar-service/internal/domain"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	ExchangeName       = "avatars.exchange"
	ExchangeType       = "topic"
	ThumbnailQueueName = "avatars.thumbnail.queue"
	UploadRoutingKey   = "avatar.uploaded"
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
// и объявляет необходимую топологию.
func NewPublisher(cfg config.RabbitMQConfig, logger *zap.Logger) (*Publisher, error) {
	conn, err := amqp.DialConfig(cfg.URL, amqp.Config{
		Heartbeat: cfg.Heartbeat,
		Dial:      amqp.DefaultDial(cfg.ConnectTimeout),
		Recovery:  &amqp.Recovery{}, // <-- Ключевой момент: включаем авто-восстановление
	})
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	p := &Publisher{
		logger:  logger,
		conn:    conn,
		channel: ch,
	}

	if err := p.declareTopology(); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("declare topology: %w", err)
	}

	logger.Info("rabbitmq publisher initialized with auto-recovery",
		zap.String("exchange", ExchangeName),
		zap.String("queue", ThumbnailQueueName),
	)

	return p, nil
}

// declareTopology объявляет exchange, очередь и биндинг.
// При разрыве связи amqp091-go автоматически выполнит эти же
// объявления на восстановленном канале.
func (p *Publisher) declareTopology() error {
	// 1. Exchange
	if err := p.channel.ExchangeDeclare(
		ExchangeName,
		ExchangeType,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeName, err)
	}

	// 2. Queue
	if _, err := p.channel.QueueDeclare(
		ThumbnailQueueName,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare queue %s: %w", ThumbnailQueueName, err)
	}

	// 3. Binding
	if err := p.channel.QueueBind(
		ThumbnailQueueName,
		UploadRoutingKey,
		ExchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("bind queue %s: %w", ThumbnailQueueName, err)
	}

	return nil
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
		false, // mandatory
		false, // immediate
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

// Ping проверяет, что соединение живо. Используется в health-check.
func (p *Publisher) Ping(ctx context.Context) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil || conn.IsClosed() {
		return fmt.Errorf("connection is closed")
	}
	return nil
}

// Close закрывает канал и соединение.
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
			}
		}
		p.conn = nil
	}

	p.logger.Info("rabbitmq publisher closed")
	return firstErr
}

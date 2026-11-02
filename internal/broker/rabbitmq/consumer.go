package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"avatar-service/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// MessageHandler обрабатывает тело сообщения из очереди.
// Возвращает ошибку — consumer сделает Nack; nil — Ack.
type MessageHandler func(ctx context.Context, body []byte) error

// Consumer читает сообщения из очереди с авто-восстановлением
// через встроенный Recovery в amqp091-go.
type Consumer struct {
	logger   *zap.Logger
	prefetch int

	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewConsumer подключается к RabbitMQ, объявляет топологию через общий
// Declare и открывает канал с включённым Recovery.
func NewConsumer(cfg config.RabbitMQConfig, logger *zap.Logger) (*Consumer, error) {
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

	c := &Consumer{
		logger:   logger,
		prefetch: cfg.PrefetchCount,
		conn:     conn,
		ch:       ch,
	}

	if err := c.declare(); err != nil {
		if closeErr := c.Close(); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close: %w", closeErr))
		}
		return nil, err
	}

	return c, nil
}

// declare объявляет топологию через общий Declare.
func (c *Consumer) declare() error {
	return Declare(c.ch)
}

// Run блокируется до отмены ctx, читая сообщения из thumbnail-очереди
// и передавая их в handler. При ошибке handler — Nack без requeue,
// сообщение уходит в DLX → DLQ.
func (c *Consumer) Run(ctx context.Context, handler MessageHandler) error {
	if err := c.ch.Qos(c.prefetch, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	msgs, err := c.ch.Consume(
		ThumbnailQueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	c.logger.Info("worker consuming", zap.String("queue", ThumbnailQueueName))

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("worker stopped")
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("messages channel closed")
			}

			if hErr := handler(ctx, msg.Body); hErr != nil {
				c.logger.Error("handler failed", zap.Error(hErr))
				// requeue=false → DLX → DLQ
				if nackErr := msg.Nack(false, false); nackErr != nil {
					c.logger.Error("nack failed",
						zap.String("message_id", msg.MessageId),
						zap.Error(nackErr),
					)
				}
				continue
			}

			if ackErr := msg.Ack(false); ackErr != nil {
				c.logger.Error("ack failed",
					zap.String("message_id", msg.MessageId),
					zap.Error(ackErr),
				)
			}
		}
	}
}

// Close закрывает канал и соединение; ошибки объединяются.
func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var firstErr error
	if c.ch != nil {
		if err := c.ch.Close(); err != nil && err != amqp.ErrClosed {
			firstErr = err
		}
		c.ch = nil
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil && err != amqp.ErrClosed {
			if firstErr == nil {
				firstErr = err
			} else {
				firstErr = errors.Join(firstErr, err)
			}
		}
		c.conn = nil
	}
	return firstErr
}

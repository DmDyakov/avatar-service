// Package rabbitmq is a RabbitMQ client with publisher, consumer,
// and topology declaration for avatar events.
package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// ExchangeName — основной exchange для событий аватарок.
	ExchangeName = "avatars.exchange"
	// ExchangeType — тип основного exchange (topic).
	ExchangeType = "topic"
	// ThumbnailQueueName — очередь задач на генерацию миниатюр.
	ThumbnailQueueName = "avatars.thumbnail.queue"
	// UploadRoutingKey — routing key для события upload.
	UploadRoutingKey = "avatar.uploaded"

	DeleteQueueName  = "avatars.delete.queue"
	DeleteRoutingKey = "avatar.delete.created"

	// DLXName — dead-letter exchange для неудачных сообщений.
	DLXName = "avatars.dlx"
	// DLXType — тип DLX (direct).
	DLXType = "direct"
	// DLQName — dead-letter queue.
	DLQName = "avatars.dlq"
	// DLQRoutingKey — routing key для DLQ.
	DLQRoutingKey = "failed"
)

// Declare объявляет всю топологию: exchange, DLX, DLQ, thumbnail queue, bindings.func Declare(ch *amqp.Channel) error {
func Declare(ch *amqp.Channel) error {
	// 1. Основной exchange
	if err := ch.ExchangeDeclare(
		ExchangeName, ExchangeType,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeName, err)
	}

	// 2. DLX (общий для всех очередей)
	if err := ch.ExchangeDeclare(
		DLXName, DLXType,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare dlx %s: %w", DLXName, err)
	}

	// 3. DLQ
	if _, err := ch.QueueDeclare(
		DLQName,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare dlq %s: %w", DLQName, err)
	}

	// 4. Bind DLX → DLQ
	if err := ch.QueueBind(
		DLQName, DLQRoutingKey, DLXName, false, nil,
	); err != nil {
		return fmt.Errorf("bind dlq %s: %w", DLQName, err)
	}

	// Общие DLX-аргументы для очередей.
	args := amqp.Table{
		"x-dead-letter-exchange":    DLXName,
		"x-dead-letter-routing-key": DLQRoutingKey,
	}

	// 5. Thumbnail queue
	if _, err := ch.QueueDeclare(
		ThumbnailQueueName,
		true, false, false, false,
		args,
	); err != nil {
		return fmt.Errorf("declare queue %s: %w", ThumbnailQueueName, err)
	}

	// 6. Bind exchange → thumbnail queue
	if err := ch.QueueBind(
		ThumbnailQueueName, UploadRoutingKey, ExchangeName, false, nil,
	); err != nil {
		return fmt.Errorf("bind queue %s: %w", ThumbnailQueueName, err)
	}

	// 7. Delete queue
	if _, err := ch.QueueDeclare(
		DeleteQueueName,
		true, false, false, false,
		args,
	); err != nil {
		return fmt.Errorf("declare queue %s: %w", DeleteQueueName, err)
	}

	// 8. Bind exchange → delete queue
	if err := ch.QueueBind(
		DeleteQueueName, DeleteRoutingKey, ExchangeName, false, nil,
	); err != nil {
		return fmt.Errorf("bind queue %s: %w", DeleteQueueName, err)
	}

	return nil
}

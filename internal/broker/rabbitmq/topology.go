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

	// DLXName — dead-letter exchange для неудачных сообщений.
	DLXName = "avatars.dlx"
	// DLXType — тип DLX (direct).
	DLXType = "direct"
	// DLQName — dead-letter queue.
	DLQName = "avatars.dlq"
	// DLQRoutingKey — routing key для DLQ.
	DLQRoutingKey = "failed"
)

// Declare объявляет всю топологию: exchange, DLX, DLQ, thumbnail queue, bindings.
//
// Идемпотентно: если топология уже есть с теми же параметрами — RabbitMQ
// подтвердит. Если параметры различаются (например, изменили args у очереди),
// RabbitMQ вернёт PRECONDITION_FAILED — тогда нужно удалить очередь вручную
// (rabbitmqctl delete_queue) или пересоздать volume.
//
// Вызывается и publisher'ом, и consumer'ом — оба процесса стартуют независимо,
// и каждый должен сам поднять топологию, если её нет.
func Declare(ch *amqp.Channel) error {
	// 1. Основной exchange
	if err := ch.ExchangeDeclare(
		ExchangeName, ExchangeType,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeName, err)
	}

	// 2. DLX (direct)
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

	// 5. Thumbnail queue с DLX-аргументами
	args := amqp.Table{
		"x-dead-letter-exchange":    DLXName,
		"x-dead-letter-routing-key": DLQRoutingKey,
	}
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

	return nil
}

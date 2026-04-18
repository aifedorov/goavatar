package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName    = "avatar.exchange"
	DLXExchangeName = "avatar.dlx"

	AvatarUploadQueueName  = "avatar.upload"
	AvatarUploadRoutingKey = "avatar.uploaded"
	AvatarUploadDLQName    = "avatar.upload.dlq"
	AvatarUploadDLQKey     = "upload.dlq"
	AvatarUploadReturnKey  = "upload.retry"

	AvatarDeleteQueueName  = "avatar.delete"
	AvatarDeleteRoutingKey = "avatar.deleted"
	AvatarDeleteDLQName    = "avatar.delete.dlq"
	AvatarDeleteDLQKey     = "delete.dlq"
	AvatarDeleteReturnKey  = "delete.retry"

	HeaderRetryCount = "x-retry-count"
)

type RetryLevel struct {
	QueueName string
	TTL       int32
}

var (
	UploadRetryLevels = []RetryLevel{
		{"avatar.upload.retry.30s", 30_000},
		{"avatar.upload.retry.1m", 60_000},
	}
	DeleteRetryLevels = []RetryLevel{
		{"avatar.delete.retry.30s", 30_000},
		{"avatar.delete.retry.1m", 60_000},
	}
)

func DeclareAvatarTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(ExchangeName, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", ExchangeName, err)
	}

	if err := ch.ExchangeDeclare(DLXExchangeName, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", DLXExchangeName, err)
	}

	if _, err := ch.QueueDeclare(AvatarUploadQueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", AvatarUploadQueueName, err)
	}
	if err := ch.QueueBind(AvatarUploadQueueName, AvatarUploadRoutingKey, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarUploadQueueName, AvatarUploadRoutingKey, err)
	}
	if err := ch.QueueBind(AvatarUploadQueueName, AvatarUploadReturnKey, DLXExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarUploadQueueName, AvatarUploadReturnKey, err)
	}

	if err := declareRetryQueues(ch, UploadRetryLevels, AvatarUploadReturnKey); err != nil {
		return err
	}

	if _, err := ch.QueueDeclare(AvatarUploadDLQName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", AvatarUploadDLQName, err)
	}
	if err := ch.QueueBind(AvatarUploadDLQName, AvatarUploadDLQKey, DLXExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarUploadDLQName, AvatarUploadDLQKey, err)
	}

	if _, err := ch.QueueDeclare(AvatarDeleteQueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", AvatarDeleteQueueName, err)
	}
	if err := ch.QueueBind(AvatarDeleteQueueName, AvatarDeleteRoutingKey, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarDeleteQueueName, AvatarDeleteRoutingKey, err)
	}
	if err := ch.QueueBind(AvatarDeleteQueueName, AvatarDeleteReturnKey, DLXExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarDeleteQueueName, AvatarDeleteReturnKey, err)
	}

	if err := declareRetryQueues(ch, DeleteRetryLevels, AvatarDeleteReturnKey); err != nil {
		return err
	}

	if _, err := ch.QueueDeclare(AvatarDeleteDLQName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", AvatarDeleteDLQName, err)
	}
	if err := ch.QueueBind(AvatarDeleteDLQName, AvatarDeleteDLQKey, DLXExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarDeleteDLQName, AvatarDeleteDLQKey, err)
	}

	return nil
}

func declareRetryQueues(ch *amqp.Channel, levels []RetryLevel, returnKey string) error {
	for _, level := range levels {
		if _, err := ch.QueueDeclare(level.QueueName, true, false, false, false, amqp.Table{
			"x-message-ttl":             level.TTL,
			"x-dead-letter-exchange":    DLXExchangeName,
			"x-dead-letter-routing-key": returnKey,
		}); err != nil {
			return fmt.Errorf("declare queue %q: %w", level.QueueName, err)
		}
	}
	return nil
}

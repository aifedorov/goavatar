package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName           = "avatar.exchange"
	AvatarUploadQueueName  = "avatar.upload"
	AvatarUploadRoutingKey = "avatar.uploaded"
	AvatarDeleteQueueName  = "avatar.delete"
	AvatarDeleteRoutingKey = "avatar.deleted"
)

func DeclareAvatarTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(ExchangeName, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", ExchangeName, err)
	}

	if _, err := ch.QueueDeclare(AvatarUploadQueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", AvatarUploadQueueName, err)
	}

	if err := ch.QueueBind(AvatarUploadQueueName, AvatarUploadRoutingKey, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarUploadQueueName, AvatarUploadRoutingKey, err)
	}

	if _, err := ch.QueueDeclare(AvatarDeleteQueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue %q: %w", AvatarDeleteQueueName, err)
	}

	if err := ch.QueueBind(AvatarDeleteQueueName, AvatarDeleteRoutingKey, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind queue %q to %q: %w", AvatarDeleteQueueName, AvatarDeleteRoutingKey, err)
	}

	return nil
}

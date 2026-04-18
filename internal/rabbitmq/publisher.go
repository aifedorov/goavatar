package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type publisher struct {
	ch *amqp.Channel
}

func NewAvatarEventPublisher(ch *amqp.Channel) domain.AvatarEventPublisher {
	return &publisher{
		ch: ch,
	}
}

func (p *publisher) PublishUploadEvent(ctx context.Context, event domain.AvatarUploadEvent) error {
	return p.publish(ctx, AvatarUploadRoutingKey, event)
}

func (p *publisher) PublishDeleteEvent(ctx context.Context, event domain.AvatarDeleteEvent) error {
	return p.publish(ctx, AvatarDeleteRoutingKey, event)
}

func (p *publisher) publish(ctx context.Context, routingKey string, event any) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	return p.ch.PublishWithContext(
		ctx,
		ExchangeName,
		routingKey,
		false,
		false,
		amqp.Publishing{
			MessageId:    uuid.NewString(),
			Body:         body,
			DeliveryMode: amqp.Persistent,
		},
	)
}

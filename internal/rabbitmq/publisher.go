package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/pkg/telemetry"
)

const tracerName = "github.com/aifedorov/goavatar/internal/rabbitmq"

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

	messageID := uuid.NewString()

	ctx, span := otel.Tracer(tracerName).Start(ctx,
		routingKey+" publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", ExchangeName),
			attribute.String("messaging.rabbitmq.destination.routing_key", routingKey),
			attribute.String("messaging.operation", "publish"),
			attribute.String("messaging.message.id", messageID),
		),
	)
	defer span.End()

	body, err := json.Marshal(event)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		return fmt.Errorf("marshal event: %w", err)
	}

	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, telemetry.AMQPHeadersCarrier(headers))

	err = p.ch.PublishWithContext(
		ctx,
		ExchangeName,
		routingKey,
		false,
		false,
		amqp.Publishing{
			MessageId:    messageID,
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
		},
	)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
	return err
}

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aifedorov/goavatar/internal/config"
	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/rabbitmq"
	"github.com/aifedorov/goavatar/internal/repository/postgres"
	"github.com/aifedorov/goavatar/internal/repository/s3"
	"github.com/aifedorov/goavatar/pkg/imgutil"
	"github.com/aifedorov/goavatar/pkg/telemetry"
)

const tracerName = "github.com/aifedorov/goavatar/internal/worker"

const (
	prefetch          = 2
	uploadConsumerTag = "avatar-upload-worker"
	deleteConsumerTag = "avatar-delete-worker"
	shutdownTimeout   = 30 * time.Second
)

type App struct {
	cfg    *config.Config
	logger *slog.Logger
}

func NewApp(cfg *config.Config, logger *slog.Logger) *App {
	return &App{cfg: cfg, logger: logger}
}

func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pgxCfg, err := pgxpool.ParseConfig(a.cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("parse database config: %w", err)
	}
	pgxCfg.ConnConfig.Tracer = otelpgx.NewTracer()

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	s3Client, err := s3.NewClient(a.cfg.S3Endpoint, a.cfg.S3AccessKey, a.cfg.S3SecretKey, a.cfg.S3UseSSL)
	if err != nil {
		return fmt.Errorf("init s3 client: %w", err)
	}
	fileStorage := s3.NewStorage(s3Client, a.cfg.S3Bucket)
	if err := fileStorage.Ping(ctx); err != nil {
		return fmt.Errorf("ping s3: %w", err)
	}

	avatarRepo := postgres.NewAvatarRepo(pool)
	w := NewWorker(avatarRepo, fileStorage, ResizeFunc(imgutil.Resize))

	conn, err := amqp.Dial(a.cfg.RabbitMQURL)
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	defer ch.Close()

	if err := rabbitmq.DeclareAvatarTopology(ch); err != nil {
		return fmt.Errorf("declare RabbitMQ topology: %w", err)
	}

	if err := ch.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("set channel QoS: %w", err)
	}

	uploadDeliveries, err := ch.Consume(
		rabbitmq.AvatarUploadQueueName,
		uploadConsumerTag,
		false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("start upload consumer: %w", err)
	}

	deleteDeliveries, err := ch.Consume(
		rabbitmq.AvatarDeleteQueueName,
		deleteConsumerTag,
		false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("start delete consumer: %w", err)
	}

	a.logger.InfoContext(ctx, "worker started",
		slog.String("upload_queue", rabbitmq.AvatarUploadQueueName),
		slog.String("delete_queue", rabbitmq.AvatarDeleteQueueName),
		slog.Int("prefetch", prefetch),
	)

	uploadDone := make(chan struct{})
	go func() {
		defer close(uploadDone)
		for delivery := range uploadDeliveries {
			func() {
				msgCtx, span := startConsumerSpan(ctx, rabbitmq.AvatarUploadQueueName, delivery)
				defer span.End()

				a.logger.InfoContext(msgCtx, "received upload delivery",
					slog.String("message_id", delivery.MessageId),
				)

				var event domain.AvatarUploadEvent
				if err := json.Unmarshal(delivery.Body, &event); err != nil {
					a.logger.ErrorContext(msgCtx, "unmarshal upload event", slog.Any("error", err))
					recordSpanError(span, err)
					_ = delivery.Nack(false, false)
					return
				}

				if err := w.HandleUploadEvent(msgCtx, event); err != nil {
					recordSpanError(span, err)
					retry := retryCount(delivery)
					a.logger.ErrorContext(msgCtx, "handle upload event",
						slog.String("avatar_id", event.AvatarID),
						slog.Any("error", err),
						slog.Int64("retry", retry),
					)
					if retry < int64(len(rabbitmq.UploadRetryLevels)) {
						if pubErr := republishOrRequeue(delivery, func() error {
							return publishRetryTo(msgCtx, ch, delivery, retry, rabbitmq.UploadRetryLevels)
						}, nil); pubErr != nil {
							a.logger.ErrorContext(msgCtx, "publish retry", slog.Any("error", pubErr))
							return
						}
					} else {
						if pubErr := republishOrRequeue(delivery, func() error {
							return publishToDLQ(msgCtx, ch, delivery, rabbitmq.AvatarUploadDLQKey)
						}, func() {
							avatarID, parseErr := uuid.Parse(event.AvatarID)
							if parseErr == nil {
								if markErr := w.MarkProcessingFailed(msgCtx, avatarID); markErr != nil {
									a.logger.ErrorContext(msgCtx, "mark processing failed",
										slog.String("avatar_id", event.AvatarID),
										slog.Any("error", markErr),
									)
								}
							}
						}); pubErr != nil {
							a.logger.ErrorContext(msgCtx, "publish to dlq", slog.Any("error", pubErr))
							return
						}
					}
					return
				}

				a.logger.InfoContext(msgCtx, "processed upload event", slog.String("avatar_id", event.AvatarID))
				_ = delivery.Ack(false)
			}()
		}
	}()

	deleteDone := make(chan struct{})
	go func() {
		defer close(deleteDone)
		for delivery := range deleteDeliveries {
			func() {
				msgCtx, span := startConsumerSpan(ctx, rabbitmq.AvatarDeleteQueueName, delivery)
				defer span.End()

				a.logger.InfoContext(msgCtx, "received delete delivery",
					slog.String("message_id", delivery.MessageId),
				)

				var event domain.AvatarDeleteEvent
				if err := json.Unmarshal(delivery.Body, &event); err != nil {
					a.logger.ErrorContext(msgCtx, "unmarshal delete event", slog.Any("error", err))
					recordSpanError(span, err)
					_ = delivery.Nack(false, false)
					return
				}

				if err := w.HandleDeleteEvent(msgCtx, event); err != nil {
					recordSpanError(span, err)
					retry := retryCount(delivery)
					a.logger.ErrorContext(msgCtx, "handle delete event",
						slog.String("avatar_id", event.AvatarID),
						slog.Any("error", err),
						slog.Int64("retry", retry),
					)
					if retry < int64(len(rabbitmq.DeleteRetryLevels)) {
						if pubErr := republishOrRequeue(delivery, func() error {
							return publishRetryTo(msgCtx, ch, delivery, retry, rabbitmq.DeleteRetryLevels)
						}, nil); pubErr != nil {
							a.logger.ErrorContext(msgCtx, "publish delete retry", slog.Any("error", pubErr))
							return
						}
					} else {
						if pubErr := republishOrRequeue(delivery, func() error {
							return publishToDLQ(msgCtx, ch, delivery, rabbitmq.AvatarDeleteDLQKey)
						}, nil); pubErr != nil {
							a.logger.ErrorContext(msgCtx, "publish to delete dlq", slog.Any("error", pubErr))
							return
						}
					}
					return
				}

				a.logger.InfoContext(msgCtx, "processed delete event", slog.String("avatar_id", event.AvatarID))
				_ = delivery.Ack(false)
			}()
		}
	}()

	<-ctx.Done()
	a.logger.InfoContext(ctx, "shutdown signal received, draining")

	_ = ch.Cancel(uploadConsumerTag, false)
	_ = ch.Cancel(deleteConsumerTag, false)

	select {
	case <-uploadDone:
	case <-time.After(shutdownTimeout):
		a.logger.WarnContext(ctx, "upload drain timeout")
	}
	select {
	case <-deleteDone:
	case <-time.After(shutdownTimeout):
		a.logger.WarnContext(ctx, "delete drain timeout")
	}

	return nil
}

func startConsumerSpan(ctx context.Context, queueName string, delivery amqp.Delivery) (context.Context, trace.Span) {
	ctx = otel.GetTextMapPropagator().Extract(ctx,
		telemetry.AMQPHeadersCarrier(delivery.Headers))
	return otel.Tracer(tracerName).Start(ctx,
		queueName+" process",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", queueName),
			attribute.String("messaging.operation", "process"),
			attribute.String("messaging.message.id", delivery.MessageId),
		),
	)
}

func recordSpanError(span trace.Span, err error) {
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}

func retryCount(d amqp.Delivery) int64 {
	if d.Headers == nil {
		return 0
	}
	count, _ := d.Headers[rabbitmq.HeaderRetryCount].(int64)
	return count
}

func publishToDLQ(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, dlqKey string) error {
	headers := make(amqp.Table)
	for k, v := range d.Headers {
		headers[k] = v
	}
	return ch.PublishWithContext(ctx, rabbitmq.DLXExchangeName, dlqKey, false, false, amqp.Publishing{
		MessageId:    d.MessageId,
		Body:         d.Body,
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
	})
}

func publishRetryTo(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, retry int64, levels []rabbitmq.RetryLevel) error {
	level := levels[retry]
	headers := amqp.Table{}
	for k, v := range d.Headers {
		headers[k] = v
	}
	headers[rabbitmq.HeaderRetryCount] = retry + 1
	return ch.PublishWithContext(ctx, "", level.QueueName, false, false, amqp.Publishing{
		MessageId:    d.MessageId,
		Body:         d.Body,
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
	})
}

func republishOrRequeue(delivery amqp.Delivery, publish func() error, afterPublish func()) error {
	if err := publish(); err != nil {
		_ = delivery.Nack(false, true)
		return err
	}

	if afterPublish != nil {
		afterPublish()
	}

	_ = delivery.Ack(false)
	return nil
}

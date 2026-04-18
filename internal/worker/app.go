package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/aifedorov/goavatar/internal/config"
	"github.com/aifedorov/goavatar/internal/domain"
	"github.com/aifedorov/goavatar/internal/rabbitmq"
	"github.com/aifedorov/goavatar/internal/repository/postgres"
	"github.com/aifedorov/goavatar/internal/repository/s3"
	"github.com/aifedorov/goavatar/pkg/imgutil"
)

const (
	prefetch           = 2
	uploadConsumerTag  = "avatar-upload-worker"
	deleteConsumerTag  = "avatar-delete-worker"
	shutdownTimeout    = 30 * time.Second
)

type App struct {
	cfg    *config.Config
	logger *zap.Logger
}

func NewApp(cfg *config.Config, logger *zap.Logger) *App {
	return &App{cfg: cfg, logger: logger}
}

func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, a.cfg.DatabaseURI)
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

	a.logger.Info("worker started",
		zap.String("upload_queue", rabbitmq.AvatarUploadQueueName),
		zap.String("delete_queue", rabbitmq.AvatarDeleteQueueName),
		zap.Int("prefetch", prefetch),
	)

	uploadDone := make(chan struct{})
	go func() {
		defer close(uploadDone)
		for delivery := range uploadDeliveries {
			a.logger.Info("received upload delivery",
				zap.String("message_id", delivery.MessageId),
			)

			var event domain.AvatarUploadEvent
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				a.logger.Error("unmarshal upload event", zap.Error(err))
				_ = delivery.Nack(false, false)
				continue
			}

			if err := w.HandleUploadEvent(ctx, event); err != nil {
				a.logger.Error("handle upload event",
					zap.String("avatar_id", event.AvatarID),
					zap.Error(err),
				)
				requeue := IsRetryable(err)
				if !requeue {
					avatarID, parseErr := uuid.Parse(event.AvatarID)
					if parseErr == nil {
						_ = w.repo.UpdateProcessingStatus(ctx, avatarID, domain.ProcessingStatusFailed, nil)
					}
				}
				_ = delivery.Nack(false, requeue)
				continue
			}

			a.logger.Info("processed upload event", zap.String("avatar_id", event.AvatarID))
			_ = delivery.Ack(false)
		}
	}()

	deleteDone := make(chan struct{})
	go func() {
		defer close(deleteDone)
		for delivery := range deleteDeliveries {
			a.logger.Info("received delete delivery",
				zap.String("message_id", delivery.MessageId),
			)

			var event domain.AvatarDeleteEvent
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				a.logger.Error("unmarshal delete event", zap.Error(err))
				_ = delivery.Nack(false, false)
				continue
			}

			if err := w.HandleDeleteEvent(ctx, event); err != nil {
				a.logger.Error("handle delete event",
					zap.String("avatar_id", event.AvatarID),
					zap.Error(err),
				)
				_ = delivery.Nack(false, false)
				continue
			}

			a.logger.Info("processed delete event", zap.String("avatar_id", event.AvatarID))
			_ = delivery.Ack(false)
		}
	}()

	<-ctx.Done()
	a.logger.Info("shutdown signal received, draining")

	_ = ch.Cancel(uploadConsumerTag, false)
	_ = ch.Cancel(deleteConsumerTag, false)

	select {
	case <-uploadDone:
	case <-time.After(shutdownTimeout):
		a.logger.Warn("upload drain timeout")
	}
	select {
	case <-deleteDone:
	case <-time.After(shutdownTimeout):
		a.logger.Warn("delete drain timeout")
	}

	return nil
}

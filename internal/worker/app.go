package worker

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/aifedorov/goavatar/internal/config"
	"github.com/aifedorov/goavatar/internal/rabbitmq"
	"github.com/aifedorov/goavatar/internal/repository/s3"
)

const (
	prefetch        = 2
	consumerTag     = "avatar-upload-worker"
	shutdownTimeout = 30 * time.Second
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

	deliveries, err := ch.Consume(
		rabbitmq.AvatarUploadQueueName,
		consumerTag,
		false, // autoAck — manual ack for at-least-once
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,
	)
	if err != nil {
		return fmt.Errorf("start consumer: %w", err)
	}

	a.logger.Info("worker started",
		zap.String("queue", rabbitmq.AvatarUploadQueueName),
		zap.Int("prefetch", prefetch),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for delivery := range deliveries {
			a.logger.Info("received delivery",
				zap.String("message_id", delivery.MessageId),
				zap.ByteString("body", delivery.Body),
			)
			if err := delivery.Ack(false); err != nil {
				a.logger.Error("ack delivery", zap.Error(err))
			}
		}
		a.logger.Info("delivery channel closed")
	}()

	<-ctx.Done()
	a.logger.Info("shutdown signal received, draining")

	if err := ch.Cancel(consumerTag, false); err != nil {
		a.logger.Error("cancel consumer", zap.Error(err))
	}

	select {
	case <-done:
		a.logger.Info("worker drained cleanly")
	case <-time.After(shutdownTimeout):
		a.logger.Warn("shutdown timeout exceeded")
	}

	return nil
}

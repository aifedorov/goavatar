package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aifedorov/goavatar/internal/rabbitmq"
	"github.com/exaring/otelpgx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/aifedorov/goavatar/internal/config"
	"github.com/aifedorov/goavatar/internal/handlers"
	"github.com/aifedorov/goavatar/internal/repository/postgres"
	"github.com/aifedorov/goavatar/internal/repository/s3"
	"github.com/aifedorov/goavatar/internal/services"
)

type App struct {
	cfg    *config.Config
	logger *slog.Logger
}

func NewApp(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
	}
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

	avatarRepo := postgres.NewAvatarRepo(pool)

	meter := otel.Meter("github.com/aifedorov/goavatar/internal/app")
	if _, err := meter.Int64ObservableGauge("avatars.storage.bytes",
		metric.WithDescription("Total storage used by avatars"),
		metric.WithUnit("By"),
		metric.WithInt64Callback(func(ctx context.Context, obs metric.Int64Observer) error {
			total, err := avatarRepo.TotalStorageBytes(ctx)
			if err != nil {
				return err
			}
			obs.Observe(total)
			return nil
		}),
	); err != nil {
		return fmt.Errorf("register storage gauge: %w", err)
	}

	s3Client, err := s3.NewClient(a.cfg.S3Endpoint, a.cfg.S3AccessKey, a.cfg.S3SecretKey, a.cfg.S3UseSSL)
	if err != nil {
		return fmt.Errorf("init s3 client: %w", err)
	}
	fileStorage := s3.NewStorage(s3Client, a.cfg.S3Bucket)

	s3KeyFunc := func(id uuid.UUID, _ string, fileName string) string {
		return fmt.Sprintf("originals/%s%s", id, filepath.Ext(fileName))
	}

	conn, err := amqp.Dial(a.cfg.RabbitMQURL)
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("create RabbitMQ channel: %w", err)
	}
	defer ch.Close()

	if err := rabbitmq.DeclareAvatarTopology(ch); err != nil {
		return fmt.Errorf("declare RabbitMQ topology: %w", err)
	}

	publisher := rabbitmq.NewAvatarEventPublisher(ch)

	avatarService := services.NewAvatarService(avatarRepo, fileStorage, s3KeyFunc, publisher, a.cfg.MaxUploadBytes)
	avatarHandler := handlers.NewAvatarHandler(avatarService, avatarService, avatarService, avatarService, a.logger, a.cfg.MaxUploadBytes)

	healthHandler := handlers.NewHealthHandler(a.logger, 2*time.Second,
		handlers.HealthCheck{Name: "postgres", Check: pool.Ping},
		handlers.HealthCheck{Name: "s3", Check: fileStorage.Ping},
		handlers.HealthCheck{Name: "rabbitmq", Check: func(_ context.Context) error {
			if conn.IsClosed() {
				return fmt.Errorf("connection closed")
			}
			testCh, err := conn.Channel()
			if err != nil {
				return fmt.Errorf("open channel: %w", err)
			}
			return testCh.Close()
		}},
	)

	r := chi.NewRouter()
	r.Use(routeTagMiddleware)
	r.Get("/health", healthHandler.Handle)
	r.Post("/api/v1/avatars", avatarHandler.Upload)
	r.Get("/api/v1/avatars/{avatar_id}", avatarHandler.GetImage)
	r.Get("/api/v1/avatars/{avatar_id}/metadata", avatarHandler.GetMetadata)
	r.Delete("/api/v1/avatars/{avatar_id}", avatarHandler.DeleteAvatar)
	r.Get("/api/v1/users/{user_id}/avatar", avatarHandler.GetUserAvatar)
	r.Delete("/api/v1/users/{user_id}/avatar", avatarHandler.DeleteUserAvatar)
	r.Get("/api/v1/users/{user_id}/avatars", avatarHandler.ListUserAvatars)

	r.Handle("/*", http.FileServer(http.Dir(a.cfg.StaticDir)))

	srv := &http.Server{
		Addr: a.cfg.HTTPAddress,
		Handler: otelhttp.NewHandler(r, "http.server",
			otelhttp.WithSpanNameFormatter(func(_ string, req *http.Request) string {
				return req.Method + " " + req.URL.Path
			}),
		),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			a.logger.ErrorContext(shutdownCtx, "shutdown server", slog.Any("error", err))
		}
	}()

	a.logger.InfoContext(ctx, "starting server", slog.String("address", a.cfg.HTTPAddress))

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start server: %w", err)
	}

	return nil
}

func routeTagMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(w, req)
		rctx := chi.RouteContext(req.Context())
		if rctx == nil {
			return
		}
		pattern := rctx.RoutePattern()
		if pattern == "" {
			return
		}
		span := trace.SpanFromContext(req.Context())
		span.SetName(req.Method + " " + pattern)
		span.SetAttributes(attribute.String("http.route", pattern))
	})
}

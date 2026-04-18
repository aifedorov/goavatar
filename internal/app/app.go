package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/aifedorov/goavatar/internal/config"
	"github.com/aifedorov/goavatar/internal/handlers"
	"github.com/aifedorov/goavatar/internal/repository/postgres"
	"github.com/aifedorov/goavatar/internal/services"
)

type App struct {
	cfg    *config.Config
	logger *zap.Logger
}

func NewApp(cfg *config.Config, logger *zap.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
	}
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

	avatarRepo := postgres.NewAvatarRepo(pool)

	// TODO: replace nil with FileStorage implementation (MinIO/S3)
	avatarService := services.NewAvatarService(avatarRepo, nil, nil)
	avatarHandler := handlers.NewAvatarHandler(avatarService, avatarService, avatarService, avatarService, a.logger)

	r := chi.NewRouter()
	r.Post("/api/v1/avatars", avatarHandler.Upload)
	r.Get("/api/v1/avatars/{avatar_id}", avatarHandler.GetImage)
	r.Get("/api/v1/avatars/{avatar_id}/metadata", avatarHandler.GetMetadata)
	r.Get("/api/v1/users/{user_id}/avatar", avatarHandler.GetUserAvatar)

	srv := &http.Server{
		Addr:    a.cfg.HTTPAddress,
		Handler: r,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("shutdown server", zap.Error(err))
		}
	}()

	a.logger.Info("starting server", zap.String("address", a.cfg.HTTPAddress))

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start server: %w", err)
	}

	return nil
}

package application

import (
	"context"

	"github.com/aifedorov/goavatar/internal/config"
	"go.uber.org/zap"
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
	_ = context.Background()
	return nil
}

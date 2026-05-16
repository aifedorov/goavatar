package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aifedorov/goavatar/internal/config"
	"github.com/aifedorov/goavatar/internal/worker"
	"github.com/aifedorov/goavatar/pkg/logger"
	"github.com/aifedorov/goavatar/pkg/telemetry"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = log.Sync()
	}()

	shutdownTracing, err := telemetry.SetupTracing(context.Background())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize tracing: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(ctx)
	}()

	shutdownMetrics, err := telemetry.SetupMetrics(context.Background())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize metrics: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownMetrics(ctx)
	}()

	app := worker.NewApp(cfg, log)
	if err := app.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to run worker: %v\n", err)
		os.Exit(1)
	}
}

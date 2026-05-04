package main

import (
	"fmt"
	"os"

	"github.com/aifedorov/goavatar/internal/config"
	"github.com/aifedorov/goavatar/internal/worker"
	"github.com/aifedorov/goavatar/pkg/logger"
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

	app := worker.NewApp(cfg, log)
	if err := app.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to run worker: %v\n", err)
		os.Exit(1)
	}
}

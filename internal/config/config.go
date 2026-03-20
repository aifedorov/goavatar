package config

import (
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

const dotEnvFile = ".env"

type Config struct {
	// HTTP server settings.
	HTTPAddress string `env:"HTTP_ADDRESS" envDefault:":8080"`
	// Logging level (debug, info, warn, error).
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

func LoadConfig() (*Config, error) {
	// Load .env.client storage if exists (optional)
	_ = godotenv.Load(dotEnvFile)

	var cfg Config
	err := env.Parse(&cfg)
	if err != nil {
		return &Config{}, err
	}

	return &cfg, nil
}

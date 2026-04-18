package config

import (
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

const dotEnvFile = ".env"

type Config struct {
	HTTPAddress string `env:"HTTP_ADDRESS" envDefault:":8080"`
	DatabaseURI string `env:"DATABASE_URI,required"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	S3Endpoint  string `env:"S3_ENDPOINT,required"`
	S3Bucket    string `env:"S3_BUCKET,required"`
	S3AccessKey string `env:"S3_ACCESS_KEY,required"`
	S3SecretKey string `env:"S3_SECRET_KEY,required"`
	S3UseSSL    bool   `env:"S3_USE_SSL" envDefault:"false"`
	RabbitMQURL    string `env:"RABBITMQ_URL,required"`
	MaxUploadBytes int64  `env:"MAX_UPLOAD_BYTES" envDefault:"10485760"`
	StaticDir      string `env:"STATIC_DIR" envDefault:"web/static"`
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load(dotEnvFile)

	var cfg Config
	err := env.Parse(&cfg)
	if err != nil {
		return &Config{}, err
	}

	return &cfg, nil
}

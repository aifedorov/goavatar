package logger

import "go.uber.org/zap"

func New(logLevel string) (*zap.Logger, error) {
	lvl, err := zap.ParseAtomicLevel(logLevel)
	if err != nil {
		return nil, err
	}

	cfg := zap.NewDevelopmentConfig()
	cfg.Level = lvl

	return cfg.Build()
}

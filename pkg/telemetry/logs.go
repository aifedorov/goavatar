package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
)

func SetupLogs(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp grpc log exporter: %w", err)
	}

	res, err := newResource(ctx)
	if err != nil {
		return nil, err
	}

	processor := log.NewBatchProcessor(exporter)
	lp := log.NewLoggerProvider(
		log.WithProcessor(processor),
		log.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	return lp.Shutdown, nil
}

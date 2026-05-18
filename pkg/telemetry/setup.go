package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const shutdownTimeout = 5 * time.Second

type ShutdownFunc func() error

func Setup(ctx context.Context) (ShutdownFunc, error) {
	var shutdowns []func(context.Context) error

	logsShutdown, err := SetupLogs(ctx)
	if err != nil {
		return nil, fmt.Errorf("setup logs: %w", err)
	}
	shutdowns = append(shutdowns, logsShutdown)

	tracingShutdown, err := SetupTracing(ctx)
	if err != nil {
		_ = shutdownAll(ctx, shutdowns)
		return nil, fmt.Errorf("setup tracing: %w", err)
	}
	shutdowns = append(shutdowns, tracingShutdown)

	metricsShutdown, err := SetupMetrics(ctx)
	if err != nil {
		_ = shutdownAll(ctx, shutdowns)
		return nil, fmt.Errorf("setup metrics: %w", err)
	}
	shutdowns = append(shutdowns, metricsShutdown)

	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return shutdownAll(ctx, shutdowns)
	}, nil
}

func shutdownAll(ctx context.Context, shutdowns []func(context.Context) error) error {
	var errs []error
	for i := len(shutdowns) - 1; i >= 0; i-- {
		if err := shutdowns[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

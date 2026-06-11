package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/sdk/resource"
)

func newResource(ctx context.Context) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}
	return res, nil
}

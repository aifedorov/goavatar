package worker

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"net"
	"time"
)

type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
	}
}

func Retry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error
	for attempt := range cfg.MaxAttempts {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !IsRetryable(lastErr) {
			return lastErr
		}
		if attempt == cfg.MaxAttempts-1 {
			break
		}
		delay := backoffDelay(attempt, cfg.BaseDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func backoffDelay(attempt int, base time.Duration) time.Duration {
	exp := math.Pow(2, float64(attempt))
	maxDelay := time.Duration(exp) * base
	return time.Duration(rand.Int64N(int64(maxDelay) + 1))
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

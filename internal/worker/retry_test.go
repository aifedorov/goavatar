package worker

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tempNetError struct{ msg string }

func (e *tempNetError) Error() string   { return e.msg }
func (e *tempNetError) Timeout() bool   { return true }
func (e *tempNetError) Temporary() bool { return true }

var _ net.Error = (*tempNetError)(nil)

func TestRetry_SucceedsOnThirdAttempt(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond}
	calls := 0

	err := Retry(context.Background(), cfg, func() error {
		calls++
		if calls < 3 {
			return &tempNetError{msg: "transient"}
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetry_ExhaustsAttempts(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}
	calls := 0

	err := Retry(context.Background(), cfg, func() error {
		calls++
		return &tempNetError{msg: "always fails"}
	})

	require.Error(t, err)
	assert.Equal(t, 3, calls)
	assert.Contains(t, err.Error(), "always fails")
}

func TestRetry_PermanentErrorStopsImmediately(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond}
	calls := 0

	err := Retry(context.Background(), cfg, func() error {
		calls++
		return fmt.Errorf("permanent: decode error")
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetry_HonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := RetryConfig{MaxAttempts: 10, BaseDelay: 100 * time.Millisecond}
	calls := 0

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, cfg, func() error {
		calls++
		return &tempNetError{msg: "transient"}
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, calls, 10)
}

func TestRetry_ImmediateSuccess(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}
	calls := 0

	err := Retry(context.Background(), cfg, func() error {
		calls++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestIsRetryable(t *testing.T) {
	assert.True(t, IsRetryable(&tempNetError{msg: "timeout"}))
	assert.True(t, IsRetryable(fmt.Errorf("wrapped: %w", &tempNetError{msg: "timeout"})))
	assert.False(t, IsRetryable(fmt.Errorf("decode error")))
	assert.False(t, IsRetryable(nil))
}

func TestBackoffDelay_BoundedByMaxDelay(t *testing.T) {
	base := 100 * time.Millisecond
	for attempt := range 5 {
		for range 100 {
			d := backoffDelay(attempt, base)
			maxDelay := time.Duration(1<<attempt) * base
			assert.GreaterOrEqual(t, int64(d), int64(0))
			assert.LessOrEqual(t, int64(d), int64(maxDelay))
		}
	}
}

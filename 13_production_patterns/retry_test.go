package productionpatterns

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	var calls atomic.Int32

	err := Retry(context.Background(), DefaultRetryConfig(), func(ctx context.Context) error {
		calls.Add(1)
		return nil //👉 Success immediately
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRetry_SuccessOnThirdAttempt(t *testing.T) {
	var calls atomic.Int32

	cfg := RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Millisecond, //Small delay for test
		MaxDelay:    100 * time.Millisecond,
		Jitter:      false,
	}

	err := Retry(context.Background(), cfg, func(ctx context.Context) error {
		n := calls.Add(1)
		if n < 3 {
			return fmt.Errorf("temporary error (attempt %d)", n)
		}
		return nil //👉 Success on 3rd attempt
	})

	require.NoError(t, err)
	assert.Equal(t, int32(3), calls.Load())
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Jitter:      false,
	}

	err := Retry(context.Background(), cfg, func(ctx context.Context) error {
		return fmt.Errorf("permanent error")
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 3 attempts exhausted")
	assert.Contains(t, err.Error(), "permanent error")
}

func TestRetry_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := RetryConfig{
		MaxAttempts: 100,
		BaseDelay:   1 * time.Second, //👉 Long delay - ctx will be canceled earlier
		MaxDelay:    5 * time.Second,
		Jitter:      false,
	}

	err := Retry(ctx, cfg, func(ctx context.Context) error {
		return fmt.Errorf("error")
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 4,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    1 * time.Second,
		Jitter:      false, //No jitter for predictability
	}

	var timestamps []time.Time

	Retry(context.Background(), cfg, func(ctx context.Context) error {
		timestamps = append(timestamps, time.Now())
		return fmt.Errorf("error")
	})

	//👉 We check that delays are growing exponentially
	// delay[0] ~ 50ms, delay[1] ~ 100ms, delay[2] ~ 200ms
	require.Len(t, timestamps, 4)

	delay1 := timestamps[1].Sub(timestamps[0])
	delay2 := timestamps[2].Sub(timestamps[1])
	delay3 := timestamps[3].Sub(timestamps[2])

	//Each subsequent delay is approximately 2 times longer (with a tolerance of ±30ms)
	assert.Greater(t, delay2, delay1-30*time.Millisecond)
	assert.Greater(t, delay3, delay2-30*time.Millisecond)
}

// =============================================================================
// RetryWithResult
// =============================================================================

func TestRetryWithResult_Success(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		Jitter:      false,
	}

	var calls atomic.Int32

	result, err := RetryWithResult(context.Background(), cfg, func(ctx context.Context) (string, error) {
		if calls.Add(1) < 2 {
			return "", fmt.Errorf("not ready yet")
		}
		return "data received", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "data received", result)
}

func TestRetryWithResult_AllFail(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond}

	result, err := RetryWithResult(context.Background(), cfg, func(ctx context.Context) (int, error) {
		return 0, fmt.Errorf("service unavailable")
	})

	require.Error(t, err)
	assert.Equal(t, 0, result) //👉 zero value on error
}

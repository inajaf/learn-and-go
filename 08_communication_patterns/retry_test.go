package communication_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	comm "learning_path/08_communication_patterns"
)

// =============================================================================
//Retry tests with Exponential Backoff
// =============================================================================

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	var calls atomic.Int32

	err := comm.Retry(comm.DefaultRetryConfig(), func() error {
		calls.Add(1)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRetry_SuccessOnSecondAttempt(t *testing.T) {
	cfg := comm.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond, //Fast for tests
		MaxDelay:    100 * time.Millisecond,
		Jitter:      false,
	}
	var calls atomic.Int32

	err := comm.Retry(cfg, func() error {
		n := calls.Add(1)
		if n < 2 {
			return fmt.Errorf("temporary error")
		}
		return nil //Success on 2nd try
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	cfg := comm.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
		Jitter:      false,
	}
	var calls atomic.Int32

	err := comm.Retry(cfg, func() error {
		calls.Add(1)
		return fmt.Errorf("service unavailable")
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, comm.ErrMaxRetriesExceeded)
	assert.Contains(t, err.Error(), "service unavailable")
	assert.Equal(t, int32(3), calls.Load())
}

func TestRetry_ExponentialBackoff_Timing(t *testing.T) {
	//No jitter - predictable delays
	cfg := comm.RetryConfig{
		MaxAttempts: 4,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    1 * time.Second,
		Jitter:      false,
	}

	// attempt 0: 10ms * 2^0 = 10ms
	assert.Equal(t, 10*time.Millisecond, comm.CalculateDelayForTest(cfg, 0))
	// attempt 1: 10ms * 2^1 = 20ms
	assert.Equal(t, 20*time.Millisecond, comm.CalculateDelayForTest(cfg, 1))
	// attempt 2: 10ms * 2^2 = 40ms
	assert.Equal(t, 40*time.Millisecond, comm.CalculateDelayForTest(cfg, 2))
}

func TestRetry_MaxDelayCap(t *testing.T) {
	cfg := comm.RetryConfig{
		MaxAttempts: 10,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    500 * time.Millisecond,
		Jitter:      false,
	}

	// attempt 5: 100ms * 2^5 = 3200ms → capped at 500ms
	delay := comm.CalculateDelayForTest(cfg, 5)
	assert.Equal(t, 500*time.Millisecond, delay)
}

func TestRetry_Jitter_AddsVariation(t *testing.T) {
	cfg := comm.RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    10 * time.Second,
		Jitter:      true,
	}

	//With jitter, each call should produce different delays
	delays := make(map[time.Duration]bool)
	for i := 0; i < 20; i++ {
		d := comm.CalculateDelayForTest(cfg, 1) // attempt 1: base ~200ms
		delays[d] = true
	}

	//There should be several different values ​​(jitter introduces variation)
	assert.Greater(t, len(delays), 1, "jitter should give different delays")
}

func TestRetry_ActualTimingWithBackoff(t *testing.T) {
	cfg := comm.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		Jitter:      false,
	}

	start := time.Now()
	_ = comm.Retry(cfg, func() error {
		return fmt.Errorf("fail")
	})
	elapsed := time.Since(start)

	//3 attempts, delays: 10ms + 20ms = 30ms (no delay after last attempt)
	//We allow for error
	assert.GreaterOrEqual(t, elapsed, 25*time.Millisecond)
	assert.Less(t, elapsed, 200*time.Millisecond)
}

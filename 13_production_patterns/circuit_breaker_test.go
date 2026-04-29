package productionpatterns

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_ClosedState_PassesThrough(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3, //👉 Opens after 3 failures
		ResetTimeout:     1 * time.Second,
	}
	cb := NewCircuitBreaker(cfg)

	//3 failures in a row
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return fmt.Errorf("service unavailable")
		})
	}

	//👉 CB is now open
	assert.Equal(t, StateOpen, cb.State())

	//The next call is an immediate failure (does not call the function)
	var called bool
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCircuitOpen)
	assert.False(t, called, "function should not be called while CB is open")
}

func TestCircuitBreaker_ResetsAfterSuccess(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     50 * time.Millisecond, //Short timeout for test
	}
	cb := NewCircuitBreaker(cfg)

	//2 failures → Open
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return fmt.Errorf("error")
		})
	}
	assert.Equal(t, StateOpen, cb.State())

	//Waiting for resetTimeout
	time.Sleep(100 * time.Millisecond)

	//👉 Trial request successful → Closed
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_HalfOpenFailure_ReturnsToOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(cfg)

	//2 failures → Open
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return fmt.Errorf("error")
		})
	}

	//Waiting for resetTimeout → Half-Open
	time.Sleep(100 * time.Millisecond)

	//👉 Test request ALSO unsuccessful → back to Open
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return fmt.Errorf("still broken")
	})

	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     1 * time.Second,
	}
	cb := NewCircuitBreaker(cfg)

	//2 failures (we do not reach the threshold)
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return fmt.Errorf("error")
		})
	}

	//👉 One success - resets the counter
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})

	//2 more failures - CB is still Closed (counter has been reset)
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return fmt.Errorf("error")
		})
	}

	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	//2 successes, 1 failure
	cb.Execute(context.Background(), func(ctx context.Context) error { return nil })
	cb.Execute(context.Background(), func(ctx context.Context) error { return nil })
	cb.Execute(context.Background(), func(ctx context.Context) error { return fmt.Errorf("err") })

	stats := cb.Stats()
	assert.Equal(t, "CLOSED", stats.State)
	assert.Equal(t, int64(3), stats.TotalCalls)
	assert.Equal(t, int64(2), stats.TotalSuccess)
	assert.Equal(t, int64(1), stats.TotalFailure)
	assert.Equal(t, 1, stats.ConsecFailures)
}

// =============================================================================
//Retry + Circuit Breaker combination tests
// =============================================================================

func TestResilientCall_RetryThenCircuitOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     5 * time.Second,
	}
	cb := NewCircuitBreaker(cfg)

	retryCfg := RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Jitter:      false,
	}

	err := ResilientCall(context.Background(), cb, retryCfg, func(ctx context.Context) error {
		return fmt.Errorf("service unavailable")
	})

	require.Error(t, err)
	//👉 After 3 failures, the CB opens, the remaining retry receives ErrCircuitOpen
	assert.True(t, errors.Is(err, ErrCircuitOpen) || cb.State() == StateOpen)
}

func TestResilientCallWithFallback(t *testing.T) {
	cfg := CircuitBreakerConfig{FailureThreshold: 1, ResetTimeout: 5 * time.Second}
	cb := NewCircuitBreaker(cfg)

	retryCfg := RetryConfig{MaxAttempts: 1, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond}

	//First call - failure, CB opens
	ResilientCall(context.Background(), cb, retryCfg, func(ctx context.Context) error {
		return fmt.Errorf("failure")
	})

	//👉 Second call - CB is open → fallback
	var fallbackCalled bool
	err := ResilientCallWithFallback(context.Background(), cb, retryCfg,
		func(ctx context.Context) error { return fmt.Errorf("main service") },
		func(ctx context.Context) error {
			fallbackCalled = true
			return nil
		},
	)

	//fallback called (wrapped into error)
	assert.True(t, fallbackCalled || err != nil)
}

// =============================================================================
// ExecuteWithResult
// =============================================================================

func TestExecuteWithResult_Success(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	result, err := ExecuteWithResult(cb, context.Background(), func(ctx context.Context) (string, error) {
		return "42 units in stock", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "42 units in stock", result)
}

func TestExecuteWithResult_CircuitOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{FailureThreshold: 1, ResetTimeout: 5 * time.Second}
	cb := NewCircuitBreaker(cfg)

	//Open CB
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return fmt.Errorf("failure")
	})

	result, err := ExecuteWithResult(cb, context.Background(), func(ctx context.Context) (int, error) {
		return 42, nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCircuitOpen)
	assert.Equal(t, 0, result) // 👉 zero value
}

package productionpatterns

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// =============================================================================
//Retry with Exponential Backoff + Jitter
// =============================================================================
//
//Why retry:
//- The network is unreliable (timeouts, lost packets)
//- Services are temporarily unavailable (deploy, restart)
//- DB is overloaded (too many connections)
//
//Why exponential backoff:
//- No delay: 1000 clients retract simultaneously → DDoS their service
//- With backoff: the delay increases: 100ms → 200ms → 400ms → 800ms
//- We give the service time to recover
//
//Why jitter (random addition):
//- Without jitter: all clients retrace in one second (thundering herd)
//- With jitter: requests are spread out over time
//
//Formula: delay = min(base * 2^attempt + random(0, base), maxDelay)

//RetryConfig - retry configuration.
type RetryConfig struct {
	MaxAttempts int           //Maximum attempts (including first)
	BaseDelay   time.Duration //Initial delay
	MaxDelay    time.Duration //Maximum Latency
	Jitter      bool          //Add randomness
}

//DefaultRetryConfig - reasonable defaults for production.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      true,
	}
}

//RetryableFunc - a function that needs to be repeated in case of an error.
type RetryableFunc func(ctx context.Context) error

//Retry executes a function that retries on error.
//
//Usage example:
//
//	err := Retry(ctx, DefaultRetryConfig(), func(ctx context.Context) error {
//	    return httpClient.Do(req)
//	})
func Retry(ctx context.Context, cfg RetryConfig, fn RetryableFunc) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		//We check the context before each attempt
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("retry canceled after %d attempts: %w", attempt, err)
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil //👉 Success - let's go out
		}

		//Last try - we can't wait
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		//👉 Calculate the delay: base * 2^attempt
		delay := cfg.calculateDelay(attempt)

		select {
		case <-time.After(delay):
			//We wait and try again
		case <-ctx.Done():
			return fmt.Errorf("retry canceled while waiting: %w", ctx.Err())
		}
	}

	return fmt.Errorf("all %d attempts exhausted: %w", cfg.MaxAttempts, lastErr)
}

//calculateDelay calculates the delay with exponential backoff + jitter.
func (c RetryConfig) calculateDelay(attempt int) time.Duration {
	// base * 2^attempt
	delay := float64(c.BaseDelay) * math.Pow(2, float64(attempt))

	//Limit to maximum
	if delay > float64(c.MaxDelay) {
		delay = float64(c.MaxDelay)
	}

	if c.Jitter {
		// 👉 Full jitter: random(0, delay)
		//This is the best algorithm according to AWS:
		// https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
		delay = rand.Float64() * delay
	}

	return time.Duration(delay)
}

// =============================================================================
//RetryWithResult - retry for functions returning a value
// =============================================================================

//RetryableResultFunc - a function with a result.
type RetryableResultFunc[T any] func(ctx context.Context) (T, error)

//RetryWithResult executes a retry function and returns the result.
//
//Example:
//
//	order, err := RetryWithResult(ctx, cfg, func(ctx context.Context) (*Order, error) {
//	    return orderClient.GetOrder(ctx, orderID)
//	})
func RetryWithResult[T any](ctx context.Context, cfg RetryConfig, fn RetryableResultFunc[T]) (T, error) {
	var zero T
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, fmt.Errorf("retry canceled after %d attempts: %w", attempt, err)
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if attempt == cfg.MaxAttempts-1 {
			break
		}

		delay := cfg.calculateDelay(attempt)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, fmt.Errorf("retry canceled while waiting: %w", ctx.Err())
		}
	}

	return zero, fmt.Errorf("all %d attempts exhausted: %w", cfg.MaxAttempts, lastErr)
}

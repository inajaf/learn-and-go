package communication

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// =============================================================================
//Retry with Exponential Backoff + Jitter
// =============================================================================
//
//Problem: The external service is temporarily unavailable.
//Without retry: the user receives an error.
//With retry: repeat after 100ms → 200ms → 400ms → ... → the user receives a response.
//
// Exponential Backoff:
//   delay = baseDelay * 2^attempt
//Example: 100ms → 200ms → 400ms → 800ms → 1600ms
//
//Jitter (random deviation):
//   delay = baseDelay * 2^attempt * random(0.5..1.5)
//For what? Without jitter, 1000 clients will retry after a failure AT THE SAME TIME
//(thundering herd). Jitter "spreads" requests over time.
//
//In production:
//- Kafka consumer: built-in retry with backoff
//   - gRPC: grpc-retry interceptor
//   - HTTP: go-retryablehttp

//ErrMaxRetriesExceeded - all attempts have been exhausted.
var ErrMaxRetriesExceeded = errors.New("max retries exceeded")

//RetryConfig - retry settings.
type RetryConfig struct {
	MaxAttempts int           //Maximum attempts (including first)
	BaseDelay   time.Duration //Initial delay
	MaxDelay    time.Duration //Maximum delay (cap)
	Jitter      bool          //Add random deviation
}

//DefaultRetryConfig returns reasonable default values.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      true,
	}
}

//Retry executes fn with retries on error.
//
//	err := Retry(DefaultRetryConfig(), func() error {
//	    return inventoryClient.CheckStock(ctx, productID, qty)
//	})
func Retry(cfg RetryConfig, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		//Last try - we can't wait
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		delay := calculateDelay(cfg, attempt)
		time.Sleep(delay)
	}

	return fmt.Errorf("%w: after %d attempts: %w", ErrMaxRetriesExceeded, cfg.MaxAttempts, lastErr)
}

//calculateDelay calculates the delay for a given attempt.
//
//	attempt 0: baseDelay * 2^0 = baseDelay
//	attempt 1: baseDelay * 2^1 = baseDelay * 2
//	attempt 2: baseDelay * 2^2 = baseDelay * 4
func calculateDelay(cfg RetryConfig, attempt int) time.Duration {
	// Exponential: baseDelay * 2^attempt
	delay := float64(cfg.BaseDelay) * math.Pow(2, float64(attempt))

	//Cap: no more than maxDelay
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	//Jitter: random deviation ±50%
	//👉 "Full jitter" algorithm from the AWS article:
	//    https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
	if cfg.Jitter {
		jitterFactor := 0.5 + rand.Float64() // [0.5, 1.5)
		delay *= jitterFactor
	}

	return time.Duration(delay)
}

//CalculateDelayForTest exports calculateDelay for tests.
func CalculateDelayForTest(cfg RetryConfig, attempt int) time.Duration {
	return calculateDelay(cfg, attempt)
}

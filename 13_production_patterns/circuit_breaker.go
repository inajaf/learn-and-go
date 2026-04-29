package productionpatterns

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
//Circuit Breaker - fuse for external calls
// =============================================================================
//
//Analogy: an electrical circuit breaker in a panel.
//- When everything is good, the current passes (Closed)
//- Too many failures - the machine knocks out (Open) - requests do not go through
//- After a while - try one request (Half-Open)
//- If successful, turn it back on (Closed)
//- If it fails, it knocks out again (Open)
//
//For what:
//- External service crashed → without CB: all requests hang on timeout (30s each)
//- With CB: after 5 failures → instant failure (fail fast) → do not waste resources
//- Service restored → CB tries → everything works
//
//State diagram:
//
//success N failures in a row
//   ┌───────────────┐     ┌──────────────┐
//   │               │     │              │
//   │   CLOSED      │────▶│    OPEN      │
//│ (passes) │ │ (blocks) │
//   │               │◀────│              │
//   └───────────────┘     └──────┬───────┘
//          ▲                     │
//│ success resetTimeout
//          │                     │
//   ┌──────┴────────┐           │
//   │               │◀──────────┘
//   │  HALF-OPEN    │
//│ (1 trial) │────▶ failure → back to OPEN
//   │               │
//   └───────────────┘

//CircuitState — state of the circuit breaker.
type CircuitState int

const (
	StateClosed   CircuitState = iota //Normal operation, requests go through
	StateOpen                         //The service is broken, requests are blocked
	StateHalfOpen                     //Let's try one request
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

//ErrCircuitOpen — circuit breaker is open, requests are not accepted.
var ErrCircuitOpen = errors.New("circuit breaker open: service temporarily unavailable")

//CircuitBreakerConfig - configuration.
type CircuitBreakerConfig struct {
	FailureThreshold int           //How many failures before opening (default: 5)
	ResetTimeout     time.Duration //After how long we try (default: 30s)
}

//DefaultCircuitBreakerConfig - reasonable defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		ResetTimeout:     30 * time.Second,
	}
}

//CircuitBreaker is an implementation of the Circuit Breaker pattern.
type CircuitBreaker struct {
	config       CircuitBreakerConfig
	mu           sync.RWMutex
	state        CircuitState
	failures     int       //Consecutive failure counter
	lastFailure  time.Time //Time of last failure
	totalCalls   int64     //Total number of calls (for metrics)
	totalSuccess int64
	totalFailure int64
}

//NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

//Execute executes the function via a circuit breaker.
//
//Usage example:
//
//	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
//
//	err := cb.Execute(ctx, func(ctx context.Context) error {
//	    return inventoryClient.CheckStock(ctx, itemID)
//	})
//
//	if errors.Is(err, ErrCircuitOpen) {
//// Service unavailable - return fallback or error to the client
//	}
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	//1. Check whether the request can be executed
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	//2. Let's do it
	cb.mu.Lock()
	cb.totalCalls++
	cb.mu.Unlock()

	err := fn(ctx)

	//3. Update the state
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

//allowRequest checks whether the request should be allowed.
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true //👉 Everything is fine - skip it

	case StateOpen:
		//👉 Checking whether resetTimeout has passed
		if time.Since(cb.lastFailure) > cb.config.ResetTimeout {
			//Go to Half-Open (write lock required)
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = StateHalfOpen
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false //👉 It’s too early - we’re blocking

	case StateHalfOpen:
		return true //👉 Skip the test request

	default:
		return false
	}
}

//recordSuccess handles a successful call.
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalSuccess++
	cb.failures = 0 //👉 Resetting the failure counter

	if cb.state == StateHalfOpen {
		//👉 Test request is successful - return to Closed
		cb.state = StateClosed
	}
}

//recordFailure handles a failed call.
func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalFailure++
	cb.failures++
	cb.lastFailure = time.Now()

	switch cb.state {
	case StateClosed:
		//👉 We have reached the threshold - we open
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = StateOpen
		}

	case StateHalfOpen:
		//👉 Trial request failed - back to Open
		cb.state = StateOpen
	}
}

//State returns the current state (for metrics/monitoring).
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

//Stats returns statistics (for dashboards).
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return CircuitBreakerStats{
		State:          cb.state.String(),
		TotalCalls:     cb.totalCalls,
		TotalSuccess:   cb.totalSuccess,
		TotalFailure:   cb.totalFailure,
		ConsecFailures: cb.failures,
	}
}

type CircuitBreakerStats struct {
	State          string
	TotalCalls     int64
	TotalSuccess   int64
	TotalFailure   int64
	ConsecFailures int
}

// =============================================================================
//CircuitBreakerWithResult - for functions returning a value
// =============================================================================

//ExecuteWithResult executes a function through CB and returns the result.
//
//Example:
//
//	stock, err := ExecuteWithResult(cb, ctx, func(ctx context.Context) (int, error) {
//	    return inventoryClient.GetStock(ctx, "item-1")
//	})
func ExecuteWithResult[T any](cb *CircuitBreaker, ctx context.Context, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T

	if !cb.allowRequest() {
		return zero, ErrCircuitOpen
	}

	cb.mu.Lock()
	cb.totalCalls++
	cb.mu.Unlock()

	result, err := fn(ctx)
	if err != nil {
		cb.recordFailure()
		return zero, err
	}

	cb.recordSuccess()
	return result, nil
}

// =============================================================================
//Combination: Circuit Breaker + Retry
// =============================================================================
//
//👉 In production they often combine:
//    Retry(ctx, config, func(ctx) error {
//        return cb.Execute(ctx, func(ctx) error {
//            return externalService.Call(ctx)
//        })
//    })
//
//The order is important:
//Retry outside → CB inside
//NOT the other way around! Otherwise, CB will not see real errors.

//ResilientCall combines retry + circuit breaker.
func ResilientCall(ctx context.Context, cb *CircuitBreaker, retryCfg RetryConfig, fn func(ctx context.Context) error) error {
	return Retry(ctx, retryCfg, func(ctx context.Context) error {
		return cb.Execute(ctx, fn)
	})
}

//ResilientCallWithFallback adds fallback when CB is open.
//
//	err := ResilientCallWithFallback(ctx, cb, retryCfg,
//func(ctx context.Context) error { return service.Call(ctx) }, // main
//	    func(ctx context.Context) error { return cache.GetCached(ctx) }, // fallback
//	)
func ResilientCallWithFallback(ctx context.Context, cb *CircuitBreaker, retryCfg RetryConfig, fn, fallback func(ctx context.Context) error) error {
	err := ResilientCall(ctx, cb, retryCfg, fn)
	if err != nil && errors.Is(err, ErrCircuitOpen) {
		//👉 CB is open - try fallback (cache, default value)
		return fmt.Errorf("the main call is unavailable, use fallback: %w", fallback(ctx))
	}
	return err
}

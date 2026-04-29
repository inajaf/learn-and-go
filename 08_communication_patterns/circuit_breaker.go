package communication

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
//Circuit Breaker for synchronous calls between services
// =============================================================================
//
//State diagram:
//
//everything is fine N failures in a row
//   ┌─────────────────┐      ┌──────────────────┐
//   │     CLOSED      │─────▶│      OPEN         │
//│ (skips) │ │ (fail fast) │
//   │                 │◀─────│                    │
//   └─────────────────┘      └────────┬───────────┘
//          ▲                          │
//│ trial │ resetTimeout
//│ request │ expired
//│ successful │
//   ┌──────┴──────────┐              │
//   │   HALF-OPEN     │◀─────────────┘
//│ (1 trial) │
//│ │───▶ failure → back to OPEN
//   └──────────────────┘

//CBState — circuit breaker state.
type CBState int

const (
	CBClosed   CBState = iota //Normal operation
	CBOpen                     //Requests are blocked
	CBHalfOpen                 //Test request
)

func (s CBState) String() string {
	switch s {
	case CBClosed:
		return "CLOSED"
	case CBOpen:
		return "OPEN"
	case CBHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

//ErrCircuitOpen - CB is open.
var ErrCircuitOpen = errors.New("circuit breaker open")

//CircuitBreaker - protection against cascading failures.
//
//Example:
//
//	cb := NewCircuitBreaker(3, 5*time.Second)
//
//// Wrap the call to InventoryService
//	err := cb.Call(func() error {
//	    return inventoryChecker.CheckStock(itemID, qty)
//	})
//
//	if errors.Is(err, ErrCircuitOpen) {
//// Fallback: return "product possibly available" or cache
//	}
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CBState
	failureThreshold int
	resetTimeout     time.Duration
	failures         int
	lastFailure      time.Time
}

//NewCircuitBreaker creates a circuit breaker.
func NewCircuitBreaker(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CBClosed,
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
	}
}

//Call performs the function through a circuit breaker.
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()

	switch cb.state {
	case CBOpen:
		//Checking whether resetTimeout has passed
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = CBHalfOpen
			cb.mu.Unlock()
			//Let's try one request
			return cb.execute(fn)
		}
		cb.mu.Unlock()
		return ErrCircuitOpen

	case CBHalfOpen:
		cb.mu.Unlock()
		return cb.execute(fn)

	default: // Closed
		cb.mu.Unlock()
		return cb.execute(fn)
	}
}

func (cb *CircuitBreaker) execute(fn func() error) error {
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailure = time.Now()

		if cb.state == CBHalfOpen || cb.failures >= cb.failureThreshold {
			cb.state = CBOpen
		}
		return err
	}

	//Success
	cb.failures = 0
	cb.state = CBClosed
	return nil
}

//State returns the current state.
func (cb *CircuitBreaker) State() CBState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// =============================================================================
//ResilientInventoryChecker - InventoryChecker with Circuit Breaker
// =============================================================================
//
//👉 We wrap the real InventoryChecker in a Circuit Breaker.
//If InventoryService is down → fail fast instead of timeouts.

//ResilientInventoryChecker adds CB to InventoryChecker.
type ResilientInventoryChecker struct {
	inner InventoryChecker
	cb    *CircuitBreaker
}

//NewResilientInventoryChecker creates a wrapper with CB.
func NewResilientInventoryChecker(inner InventoryChecker, failureThreshold int, resetTimeout time.Duration) *ResilientInventoryChecker {
	return &ResilientInventoryChecker{
		inner: inner,
		cb:    NewCircuitBreaker(failureThreshold, resetTimeout),
	}
}

func (r *ResilientInventoryChecker) CheckStock(itemID string, quantity int) (bool, error) {
	var available bool
	err := r.cb.Call(func() error {
		var innerErr error
		available, innerErr = r.inner.CheckStock(itemID, quantity)
		return innerErr
	})
	if errors.Is(err, ErrCircuitOpen) {
		//👉 CB is open - return fallback
		return false, fmt.Errorf("inventory service is not available (circuit breaker is open)")
	}
	return available, err
}

func (r *ResilientInventoryChecker) ReserveStock(itemID string, quantity int) error {
	return r.cb.Call(func() error {
		return r.inner.ReserveStock(itemID, quantity)
	})
}

func (r *ResilientInventoryChecker) ReleaseStock(itemID string, quantity int) error {
	return r.cb.Call(func() error {
		return r.inner.ReleaseStock(itemID, quantity)
	})
}

//CBSstate method for tests.
func (r *ResilientInventoryChecker) CircuitState() CBState {
	return r.cb.State()
}

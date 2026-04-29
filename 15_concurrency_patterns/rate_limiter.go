package concurrencypatterns

import (
	"context"
	"sync"
	"time"
)

// =============================================================================
//Rate Limiter - limiting the request speed
// =============================================================================
//
//For what:
//- External API: "no more than 100 requests per second"
//- Database overload protection
//- Fair use: one client should not take all the resources
//
//Algorithm: Token Bucket
//- Shopping cart with N tokens
//- Each request takes 1 token
//- Tokens are replenished at a fixed rate
//- No tokens → request is waiting or rejected
//
//Cart: [●][●][●][●][ ][ ] (4 of 6 tokens)
//                    ↑
//request takes away
//
//Replenishment: every 100ms 1 token is added

//TokenBucketLimiter - rate limiter based on token bucket.
type TokenBucketLimiter struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration //How often is 1 token added?
	mu         sync.Mutex
	stopCh     chan struct{}
}

//NewTokenBucketLimiter creates a rate limiter.
//
//	limiter := NewTokenBucketLimiter(10, 100*time.Millisecond)
//// 10 tokens max, 1 token every 100ms = 10 req/sec
func NewTokenBucketLimiter(maxTokens int, refillRate time.Duration) *TokenBucketLimiter {
	l := &TokenBucketLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		stopCh:     make(chan struct{}),
	}

	//👉 Background goroutine for token replenishment
	go l.refill()

	return l
}

func (l *TokenBucketLimiter) refill() {
	ticker := time.NewTicker(l.refillRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			if l.tokens < l.maxTokens {
				l.tokens++
			}
			l.mu.Unlock()
		case <-l.stopCh:
			return
		}
	}
}

//Allow checks whether the request can be executed (non-blocking).
//
//	if !limiter.Allow() {
//	    http.Error(w, "Too Many Requests", 429)
//	    return
//	}
func (l *TokenBucketLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.tokens > 0 {
		l.tokens--
		return true
	}
	return false
}

//Wait blocks until the token is received or the context is canceled.
//
//	if err := limiter.Wait(ctx); err != nil {
//	    return err // context cancelled
//	}
//// Continue - token received
func (l *TokenBucketLimiter) Wait(ctx context.Context) error {
	for {
		if l.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.refillRate / 2):
			//Try again after half an interval
		}
	}
}

//Available returns the current number of tokens (for metrics).
func (l *TokenBucketLimiter) Available() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tokens
}

//Stop stops background replenishment.
func (l *TokenBucketLimiter) Stop() {
	close(l.stopCh)
}

// =============================================================================
//Semaphore - limiting concurrent operations
// =============================================================================
//
//Simpler than token bucket, used when you need to limit
//number of SIMULTANEOUS goroutines (not speed, but parallelism).
//
//Example: no more than 5 simultaneous queries to the database.

//Semaphore limits the number of simultaneous operations.
type Semaphore struct {
	ch chan struct{}
}

//NewSemaphore creates a semaphore with the specified limit.
//
//sem := NewSemaphore(5) // max 5 simultaneous operations
func NewSemaphore(limit int) *Semaphore {
	return &Semaphore{ch: make(chan struct{}, limit)}
}

//Acquire takes over the slot. Blocks if all slots are occupied.
func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

//Release frees up a slot.
func (s *Semaphore) Release() {
	<-s.ch
}

//TryAcquire attempts to acquire a slot without blocking.
func (s *Semaphore) TryAcquire() bool {
	select {
	case s.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

package messaging

import (
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Dead Letter Queue (DLQ) — queue of undeliverable messages
// =============================================================================
//
// Problem: a handler fails on a specific event → an infinite retry loop.
// This is called a "poison message".
//
// Without a DLQ:
//   event → handler (error) → retry → handler (error) → retry → ... forever
//
// With a DLQ:
//   event → handler (error) → retry 1 → retry 2 → retry 3 → DLQ
//                                                               ↓
//                                                   Alert → Developer investigates
//
// In production:
//   - Kafka: a separate topic (orders.created.dlq)
//   - RabbitMQ: built-in DLQ support
//   - Our example: a separate channel + log

// DLQEntry — an entry in the DLQ.
type DLQEntry struct {
	Event    EnrichedEvent // The original event
	Error    string        // The last error
	Attempts int           // How many attempts we made
	FailedAt time.Time     // When we gave up
}

// DeadLetterQueue — queue for "poisoned" messages.
type DeadLetterQueue struct {
	mu      sync.RWMutex
	entries []DLQEntry
}

func NewDeadLetterQueue() *DeadLetterQueue {
	return &DeadLetterQueue{}
}

// Add adds an event to the DLQ.
func (dlq *DeadLetterQueue) Add(entry DLQEntry) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	dlq.entries = append(dlq.entries, entry)
}

// Entries returns all entries (for monitoring/manual processing).
func (dlq *DeadLetterQueue) Entries() []DLQEntry {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()
	result := make([]DLQEntry, len(dlq.entries))
	copy(result, dlq.entries)
	return result
}

// Len returns the number of entries.
func (dlq *DeadLetterQueue) Len() int {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()
	return len(dlq.entries)
}

// =============================================================================
// RetryHandler — a wrapper with retry + DLQ
// =============================================================================
//
// Combines:
//   1. Retry with a fixed number of attempts
//   2. DLQ for events that couldn't be processed
//
// Usage:
//
//	rh := NewRetryHandler("payment", 3, dlq, func(e EnrichedEvent) error {
//	    return processPayment(e)
//	})
//
//	rh.Handle(event) // Tries 3 times, on failure → DLQ

// RetryHandler processes an event with retry and DLQ.
type RetryHandler struct {
	name       string
	maxRetries int
	dlq        *DeadLetterQueue
	handler    EnrichedHandler
	retryDelay time.Duration // Delay between attempts
}

// NewRetryHandler creates a handler with retry.
func NewRetryHandler(name string, maxRetries int, dlq *DeadLetterQueue, handler EnrichedHandler) *RetryHandler {
	return &RetryHandler{
		name:       name,
		maxRetries: maxRetries,
		dlq:        dlq,
		handler:    handler,
		retryDelay: 10 * time.Millisecond, // Small delay for tests
	}
}

// WithRetryDelay sets the delay between attempts.
func (rh *RetryHandler) WithRetryDelay(d time.Duration) *RetryHandler {
	rh.retryDelay = d
	return rh
}

// Handle processes the event with retry. When attempts are exhausted → DLQ.
func (rh *RetryHandler) Handle(event EnrichedEvent) error {
	var lastErr error

	for attempt := 1; attempt <= rh.maxRetries; attempt++ {
		lastErr = rh.handler(event)
		if lastErr == nil {
			return nil // 👉 Success
		}

		// Not the last attempt — wait
		if attempt < rh.maxRetries {
			time.Sleep(rh.retryDelay)
		}
	}

	// 👉 All attempts exhausted — send to DLQ
	rh.dlq.Add(DLQEntry{
		Event:    event,
		Error:    fmt.Sprintf("handler %s: %v", rh.name, lastErr),
		Attempts: rh.maxRetries,
		FailedAt: time.Now(),
	})

	return fmt.Errorf("handler %s: all %d attempts exhausted for event %s: %w",
		rh.name, rh.maxRetries, event.ID, lastErr)
}

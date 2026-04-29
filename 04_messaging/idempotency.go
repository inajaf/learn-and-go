package messaging

import (
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Idempotent Consumer — exactly-once event processing
// =============================================================================
//
// The problem: Kafka/RabbitMQ guarantee "at-least-once" delivery.
// That means — the same event can arrive MULTIPLE times:
//   - Consumer processed it but didn't manage to ack → redelivery
//   - Network partition → broker re-sends
//   - Consumer restarted → offset wasn't committed
//
// Without idempotency:
//   OrderCreated arrived twice → 2 charges on the card!
//
// With idempotency:
//   OrderCreated arrived twice → processed once
//
// The pattern:
//   1. Every event has a unique ID
//   2. Before processing we check: "have we already processed this ID?"
//   3. If yes → skip. If no → process and remember the ID.
//
// In production the store is Redis or a DB table:
//   INSERT INTO processed_events(event_id) VALUES($1) ON CONFLICT DO NOTHING

// EnrichedEvent — an event with metadata for production use.
// 👉 Unlike the base Event, contains an ID and CorrelationID.
type EnrichedEvent struct {
	ID            string    // Unique event ID (for idempotency)
	CorrelationID string    // Event-chain ID (for tracing)
	Type          EventType // Event type
	Payload       any       // Data
	OccurredAt    time.Time // When it happened
}

// EnrichedHandler — handler for enriched events.
type EnrichedHandler func(event EnrichedEvent) error

// --- IdempotentStore — a store of processed events --------------------------

// IdempotentStore checks and remembers processed event IDs.
type IdempotentStore interface {
	IsProcessed(eventID string) bool
	MarkProcessed(eventID string)
}

// InMemoryIdempotentStore — in-memory implementation (for tests).
// 👉 In production: Redis SET with TTL or a PostgreSQL table.
type InMemoryIdempotentStore struct {
	mu        sync.RWMutex
	processed map[string]time.Time
}

func NewInMemoryIdempotentStore() *InMemoryIdempotentStore {
	return &InMemoryIdempotentStore{processed: make(map[string]time.Time)}
}

func (s *InMemoryIdempotentStore) IsProcessed(eventID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.processed[eventID]
	return exists
}

func (s *InMemoryIdempotentStore) MarkProcessed(eventID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed[eventID] = time.Now()
}

// ProcessedCount returns the number of processed events (for tests).
func (s *InMemoryIdempotentStore) ProcessedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.processed)
}

// --- IdempotentHandler — wrapper that makes handling idempotent -------------

// IdempotentHandler wraps a handler and guarantees one-time processing.
//
//	handler := NewIdempotentHandler(store, func(e EnrichedEvent) error {
//	    return processPayment(e) // Called exactly once per event ID
//	})
//
//	handler.Handle(event) // Processes
//	handler.Handle(event) // Skips (already processed)
type IdempotentHandler struct {
	store   IdempotentStore
	handler EnrichedHandler
	name    string // Name for logging
}

func NewIdempotentHandler(name string, store IdempotentStore, handler EnrichedHandler) *IdempotentHandler {
	return &IdempotentHandler{store: store, handler: handler, name: name}
}

// Handle processes the event if it has not been processed yet.
func (h *IdempotentHandler) Handle(event EnrichedEvent) error {
	// 👉 Step 1: Check whether we've already processed it
	if h.store.IsProcessed(event.ID) {
		// Already processed — skip (this is normal, not an error)
		return nil
	}

	// 👉 Step 2: Process
	if err := h.handler(event); err != nil {
		return fmt.Errorf("handler %s: failed to process event %s: %w", h.name, event.ID, err)
	}

	// 👉 Step 3: Mark as processed
	// IMPORTANT: mark AFTER successful processing.
	// If the handler errors — the event will be re-processed on the next delivery.
	h.store.MarkProcessed(event.ID)

	return nil
}

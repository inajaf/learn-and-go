package messaging_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	messaging "learning_path/04_messaging"
)

// =============================================================================
// Idempotent handler tests
// =============================================================================

func TestIdempotentHandler_ProcessesOnce(t *testing.T) {
	store := messaging.NewInMemoryIdempotentStore()
	var callCount atomic.Int32

	handler := messaging.NewIdempotentHandler("payment", store, func(e messaging.EnrichedEvent) error {
		callCount.Add(1)
		return nil
	})

	event := messaging.EnrichedEvent{
		ID:         "evt-123",
		Type:       messaging.EventOrderCreated,
		OccurredAt: time.Now(),
	}

	// 👉 First call — processes
	require.NoError(t, handler.Handle(event))
	assert.Equal(t, int32(1), callCount.Load())

	// 👉 Repeat call — skipped (idempotent)
	require.NoError(t, handler.Handle(event))
	assert.Equal(t, int32(1), callCount.Load()) // Still 1!

	// 👉 Third call — also skipped
	require.NoError(t, handler.Handle(event))
	assert.Equal(t, int32(1), callCount.Load())
}

func TestIdempotentHandler_DifferentEventsProcessed(t *testing.T) {
	store := messaging.NewInMemoryIdempotentStore()
	var callCount atomic.Int32

	handler := messaging.NewIdempotentHandler("notify", store, func(e messaging.EnrichedEvent) error {
		callCount.Add(1)
		return nil
	})

	// 👉 Different event IDs — all processed
	for i := 0; i < 5; i++ {
		event := messaging.EnrichedEvent{
			ID:         fmt.Sprintf("evt-%d", i),
			Type:       messaging.EventOrderCreated,
			OccurredAt: time.Now(),
		}
		require.NoError(t, handler.Handle(event))
	}

	assert.Equal(t, int32(5), callCount.Load())
	assert.Equal(t, 5, store.ProcessedCount())
}

func TestIdempotentHandler_ErrorDoesNotMarkProcessed(t *testing.T) {
	store := messaging.NewInMemoryIdempotentStore()
	var callCount atomic.Int32

	handler := messaging.NewIdempotentHandler("payment", store, func(e messaging.EnrichedEvent) error {
		callCount.Add(1)
		if callCount.Load() == 1 {
			return fmt.Errorf("transient error") // First time — error
		}
		return nil // Second time — success
	})

	event := messaging.EnrichedEvent{
		ID:         "evt-fail",
		Type:       messaging.EventOrderCreated,
		OccurredAt: time.Now(),
	}

	// 👉 First call — error, do NOT mark as processed
	err := handler.Handle(event)
	require.Error(t, err)
	assert.False(t, store.IsProcessed("evt-fail"))

	// 👉 Repeat call — success, mark it
	err = handler.Handle(event)
	require.NoError(t, err)
	assert.True(t, store.IsProcessed("evt-fail"))
	assert.Equal(t, int32(2), callCount.Load())
}

// =============================================================================
// DLQ (Dead Letter Queue) tests
// =============================================================================

func TestRetryHandler_SuccessOnFirstAttempt(t *testing.T) {
	dlq := messaging.NewDeadLetterQueue()
	var called atomic.Int32

	rh := messaging.NewRetryHandler("test", 3, dlq, func(e messaging.EnrichedEvent) error {
		called.Add(1)
		return nil
	})

	event := messaging.EnrichedEvent{ID: "evt-1", Type: messaging.EventOrderCreated, OccurredAt: time.Now()}
	err := rh.Handle(event)

	require.NoError(t, err)
	assert.Equal(t, int32(1), called.Load())
	assert.Equal(t, 0, dlq.Len()) // 👉 DLQ is empty
}

func TestRetryHandler_SuccessOnRetry(t *testing.T) {
	dlq := messaging.NewDeadLetterQueue()
	var called atomic.Int32

	rh := messaging.NewRetryHandler("test", 3, dlq, func(e messaging.EnrichedEvent) error {
		n := called.Add(1)
		if n < 3 {
			return fmt.Errorf("transient error (attempt %d)", n)
		}
		return nil // 👉 Success on the 3rd attempt
	})

	event := messaging.EnrichedEvent{ID: "evt-2", Type: messaging.EventOrderCreated, OccurredAt: time.Now()}
	err := rh.Handle(event)

	require.NoError(t, err)
	assert.Equal(t, int32(3), called.Load())
	assert.Equal(t, 0, dlq.Len()) // DLQ is still empty
}

func TestRetryHandler_AllFailures_GoesToDLQ(t *testing.T) {
	dlq := messaging.NewDeadLetterQueue()

	rh := messaging.NewRetryHandler("payment", 3, dlq, func(e messaging.EnrichedEvent) error {
		return fmt.Errorf("permanent error") // 👉 Always fails
	})

	event := messaging.EnrichedEvent{
		ID:         "evt-poison",
		Type:       messaging.EventOrderCreated,
		OccurredAt: time.Now(),
	}
	err := rh.Handle(event)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 3 attempts exhausted")

	// 👉 The event made it into the DLQ
	assert.Equal(t, 1, dlq.Len())

	entries := dlq.Entries()
	assert.Equal(t, "evt-poison", entries[0].Event.ID)
	assert.Equal(t, 3, entries[0].Attempts)
	assert.Contains(t, entries[0].Error, "permanent error")
}

func TestRetryHandler_MultiplePoisonMessages(t *testing.T) {
	dlq := messaging.NewDeadLetterQueue()

	rh := messaging.NewRetryHandler("notify", 2, dlq, func(e messaging.EnrichedEvent) error {
		return fmt.Errorf("notification service unavailable")
	})

	// 👉 3 "poison" events
	for i := 0; i < 3; i++ {
		event := messaging.EnrichedEvent{
			ID:         fmt.Sprintf("poison-%d", i),
			Type:       messaging.EventOrderCreated,
			OccurredAt: time.Now(),
		}
		rh.Handle(event)
	}

	assert.Equal(t, 3, dlq.Len())
}

// =============================================================================
// Combination test: Idempotent + Retry + DLQ
// =============================================================================

func TestIdempotentRetryDLQ_Integration(t *testing.T) {
	store := messaging.NewInMemoryIdempotentStore()
	dlq := messaging.NewDeadLetterQueue()
	var callCount atomic.Int32

	// Inner handler
	innerHandler := func(e messaging.EnrichedEvent) error {
		callCount.Add(1)
		return nil
	}

	// Wrapping: idempotency → retry → handler
	retryH := messaging.NewRetryHandler("svc", 3, dlq, innerHandler)
	idempotentH := messaging.NewIdempotentHandler("svc", store, func(e messaging.EnrichedEvent) error {
		return retryH.Handle(e)
	})

	event := messaging.EnrichedEvent{ID: "evt-combo", Type: messaging.EventOrderCreated, OccurredAt: time.Now()}

	// First call — processes
	require.NoError(t, idempotentH.Handle(event))
	assert.Equal(t, int32(1), callCount.Load())

	// Repeat call — idempotently skipped
	require.NoError(t, idempotentH.Handle(event))
	assert.Equal(t, int32(1), callCount.Load())

	assert.Equal(t, 0, dlq.Len())
}

package messaging_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	messaging "learning_path/04_messaging"
)

// TestPublishAndReceive — basic test: publish an event, receive it.
func TestPublishAndReceive(t *testing.T) {
	bus := messaging.NewInMemoryEventBus(10)
	defer bus.Close()

	// Channel for receiving the event in the test
	received := make(chan messaging.Event, 1)

	// Subscribe to the event
	bus.Subscribe(messaging.EventOrderCreated, func(event messaging.Event) {
		received <- event
	})

	// Publish the event
	err := bus.Publish(messaging.Event{
		Type: messaging.EventOrderCreated,
		Payload: messaging.OrderCreatedPayload{
			OrderID:    "order-1",
			CustomerID: "cust-1",
			Amount:     100,
		},
	})
	require.NoError(t, err)

	// Wait for the event (at most 1 second)
	select {
	case event := <-received:
		assert.Equal(t, messaging.EventOrderCreated, event.Type)
		payload, ok := event.Payload.(messaging.OrderCreatedPayload)
		require.True(t, ok, "payload should be OrderCreatedPayload")
		assert.Equal(t, "order-1", payload.OrderID)
	case <-time.After(time.Second):
		t.Fatal("timeout: event was not received")
	}
}

// TestMultipleSubscribers — multiple subscribers receive the same event.
func TestMultipleSubscribers(t *testing.T) {
	bus := messaging.NewInMemoryEventBus(10)
	defer bus.Close()

	// 👉 sync.WaitGroup — wait until all goroutines finish
	var wg sync.WaitGroup
	wg.Add(3) // expect 3 invocations

	counter := 0
	var mu sync.Mutex

	// Register 3 different subscribers
	for i := 0; i < 3; i++ {
		bus.Subscribe(messaging.EventOrderConfirmed, func(event messaging.Event) {
			mu.Lock()
			counter++
			mu.Unlock()
			wg.Done()
		})
	}

	err := bus.Publish(messaging.Event{Type: messaging.EventOrderConfirmed})
	require.NoError(t, err)

	// Wait until all three handlers complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, 3, counter)
	case <-time.After(time.Second):
		t.Fatal("timeout: not all subscribers were called")
	}
}

// TestDifferentEventTypes — each subscriber receives only its own type.
func TestDifferentEventTypes(t *testing.T) {
	bus := messaging.NewInMemoryEventBus(10)
	defer bus.Close()

	createdCount := 0
	cancelledCount := 0
	var mu sync.Mutex

	bus.Subscribe(messaging.EventOrderCreated, func(e messaging.Event) {
		mu.Lock()
		createdCount++
		mu.Unlock()
	})
	bus.Subscribe(messaging.EventOrderCancelled, func(e messaging.Event) {
		mu.Lock()
		cancelledCount++
		mu.Unlock()
	})

	// Publish two different events
	require.NoError(t, bus.Publish(messaging.Event{Type: messaging.EventOrderCreated}))
	require.NoError(t, bus.Publish(messaging.Event{Type: messaging.EventOrderCancelled}))

	// A brief pause so events get processed
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// 👉 Each subscriber received only its own event type
	assert.Equal(t, 1, createdCount, "created handler should fire once")
	assert.Equal(t, 1, cancelledCount, "cancelled handler should fire once")
}

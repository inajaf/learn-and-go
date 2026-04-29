// Package messaging demonstrates event-driven architecture via pub/sub.
//
// A real project uses Kafka, RabbitMQ, NATS.
// Here we implement an in-memory event bus on top of Go channels —
// same idea, just without an external broker.
//
// Key concept: services don't know about each other.
// They communicate through events.
package messaging

import (
	"fmt"
	"sync"
)

// ─────────────────────────────────────────────────────────────────
// EVENTS
//
// An event = a fact that has already happened.
// Named in past tense: OrderCreated, OrderCancelled.
// ─────────────────────────────────────────────────────────────────

// EventType — the type of an event.
type EventType string

const (
	EventOrderCreated   EventType = "order.created"
	EventOrderConfirmed EventType = "order.confirmed"
	EventOrderCancelled EventType = "order.cancelled"
)

// Event — the base event struct.
// 👉 Every event should contain at minimum: a type and the entity ID.
//
//	Payload — arbitrary event data.
type Event struct {
	Type    EventType
	Payload any // 👉 any = interface{} — holds the concrete event
}

// OrderCreatedPayload — payload for the "order created" event.
type OrderCreatedPayload struct {
	OrderID    string
	CustomerID string
	Amount     float64
}

// OrderCancelledPayload — payload for the "order cancelled" event.
type OrderCancelledPayload struct {
	OrderID string
	Reason  string
}

// ─────────────────────────────────────────────────────────────────
// Publisher and Consumer INTERFACES
//
// 👉 Services work through these interfaces — not against a concrete EventBus.
//    In tests we swap in a MockPublisher. In production — a Kafka/NATS implementation.
// ─────────────────────────────────────────────────────────────────

// Publisher — interface for publishing events.
type Publisher interface {
	Publish(event Event) error
}

// Handler — an event-handler function.
type Handler func(event Event)

// Consumer — interface for subscribing to events.
type Consumer interface {
	Subscribe(eventType EventType, handler Handler)
}

// EventBus — a wrapper that implements both interfaces.
type EventBus interface {
	Publisher
	Consumer
}

// ─────────────────────────────────────────────────────────────────
// InMemoryEventBus — an EventBus implementation on top of Go channels.
//
// Architecture:
//   Publish() → channel → goroutine (dispatcher) → handlers[]
//
// The dispatcher runs in a goroutine — events are processed asynchronously.
// ─────────────────────────────────────────────────────────────────

// InMemoryEventBus — the in-memory EventBus implementation.
type InMemoryEventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
	ch       chan Event
	done     chan struct{}
}

// NewInMemoryEventBus creates and starts an event bus.
func NewInMemoryEventBus(bufferSize int) *InMemoryEventBus {
	bus := &InMemoryEventBus{
		handlers: make(map[EventType][]Handler),
		ch:       make(chan Event, bufferSize), // buffered channel
		done:     make(chan struct{}),
	}
	go bus.dispatch() // run the dispatcher in the background
	return bus
}

// Publish pushes an event onto the queue.
// 👉 Non-blocking write: if the buffer is full we return an error.
func (b *InMemoryEventBus) Publish(event Event) error {
	select {
	case b.ch <- event:
		return nil
	default:
		return fmt.Errorf("event bus buffer is full, event %q dropped", event.Type)
	}
}

// Subscribe registers a handler for an event type.
// 👉 You can register several handlers for one type.
func (b *InMemoryEventBus) Subscribe(eventType EventType, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Close stops the dispatcher.
// Always call at the end — otherwise the goroutine leaks.
func (b *InMemoryEventBus) Close() {
	close(b.done)
}

// dispatch — the internal goroutine that reads events and invokes handlers.
func (b *InMemoryEventBus) dispatch() {
	for {
		select {
		case <-b.done:
			return // received the stop signal
		case event := <-b.ch:
			b.mu.RLock()
			handlers := b.handlers[event.Type]
			b.mu.RUnlock()

			// 👉 Invoke every handler for this event type
			for _, h := range handlers {
				h(event) // synchronously — for simplicity
			}
		}
	}
}

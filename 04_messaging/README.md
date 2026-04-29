# Module 4 — Messaging / Event-Driven Architecture

## 📌 What you'll learn in this module

- Why messaging exists and what event-driven architecture is
- The Pub/Sub pattern — Publisher and Subscriber don't know about each other
- The difference between an Event, a Command, and a Query
- Delivery guarantees — at-most-once / at-least-once / exactly-once
- An in-memory implementation using Go channels
- How this maps to Kafka and RabbitMQ (see module 8 for the comparison)

---

## ❓ Why do we need messaging?

Imagine two services: **OrderService** and **NotificationService**.

### ❌ Synchronous call (bad):

```go
// OrderService calls NotificationService directly
func (s *OrderService) CreateOrder(...) {
    order := s.repo.Save(order)

    // Hard dependency!
    s.notificationClient.SendEmail(order.CustomerID, order.ID)
    s.analyticsClient.TrackOrder(order)    // another dependency!
    s.inventoryClient.ReserveItems(order)  // and another!
}
```

**Problems:**
1. **High coupling** — OrderService knows about 3 other services
2. **Low reliability** — NotificationService is down → the whole order fails
3. **Slow** — we wait for every service sequentially
4. **Hard to extend** — adding a new subscriber means editing OrderService

### ✅ Asynchronous messaging (right):

```go
func (s *OrderService) CreateOrder(...) {
    order := s.repo.Save(order)

    // Publish the FACT — that the order was created
    // OrderService doesn't know who's listening
    s.publisher.Publish(Event{
        Type:    "order.created",
        Payload: OrderCreatedPayload{OrderID: order.ID, ...},
    })
    // Done! Return immediately.
}

// NotificationService subscribed on its own:
bus.Subscribe("order.created", func(e Event) {
    notification.SendEmail(...)  // async, independent
})

// AnalyticsService subscribed too:
bus.Subscribe("order.created", func(e Event) {
    analytics.TrackOrder(...)  // OrderService has no idea!
})
```

---

## 🔄 The Pub/Sub pattern

```
                    ┌──────────────┐
                    │  OrderService│
                    │  (Publisher) │
                    └──────┬───────┘
                           │ Publish("order.created")
                           ▼
                    ┌──────────────┐
                    │   EventBus   │ ← message broker
                    │ (Kafka/NATS) │
                    └──────┬───────┘
               ┌───────────┼───────────┐
               ▼           ▼           ▼
    ┌──────────────┐ ┌───────────┐ ┌──────────────┐
    │Notification  │ │Analytics  │ │  Inventory   │
    │  Service     │ │  Service  │ │  Service     │
    │(Subscriber)  │ │(Subscriber│ │(Subscriber)  │
    └──────────────┘ └───────────┘ └──────────────┘
```

Publisher doesn't know how many subscribers there are.
Subscribers don't know about the Publisher.
This is **loose coupling** — the foundation of scalable architecture.

---

## 📦 Event vs Command vs Query

In event-driven architecture it's important to understand the difference:

```
EVENT — a fact about something that happened in the past:
  "order.created", "payment.completed", "user.registered"
  Named in PAST TENSE.
  The Publisher doesn't know who will handle it.
  No expectation of a reply.

COMMAND — a request to do something:
  "CreateOrder", "SendEmail", "ReserveInventory"
  Named in IMPERATIVE MOOD.
  Addressed to a specific recipient.
  May carry an expectation of a reply.

QUERY — a request to fetch data:
  "GetOrderByID", "ListOrders"
  Phrased as a QUESTION.
  Does not change system state.
```

---

## 📬 Delivery guarantees

This is critical for production systems:

```
AT-MOST-ONCE (at most one time):
  The message is delivered once or lost.
  Use cases: metrics, logs (loss is OK)

AT-LEAST-ONCE (at least one time):
  The message is delivered AT LEAST once. May be duplicated.
  Use cases: most business events
  Requires: idempotent handlers!

EXACTLY-ONCE (exactly one time):
  The message is delivered exactly once.
  Use cases: financial transactions
  Implementation: Kafka transactions + idempotent producer
  Expensive and complex!
```

### Idempotent handler:

```go
// Idempotency = can be invoked multiple times with the same result
func (h *OrderHandler) HandleOrderCreated(event Event) {
    payload := event.Payload.(OrderCreatedPayload)

    // Check we haven't processed this event already
    exists, _ := h.db.EventProcessed(event.ID)
    if exists {
        return  // already processed — skip the duplicate
    }

    // Process it
    h.sendEmail(payload.CustomerID, payload.OrderID)

    // Mark it as processed
    h.db.MarkEventProcessed(event.ID)
}
```

---

## 🏗️ InMemoryEventBus architecture

```
Publish(event)
       │
       ▼
  chan Event ──── goroutine (dispatcher) ──── handlers[event.Type]
   (buffer)                                          │
                                              ┌──────┤
                                              ▼      ▼
                                         handler1  handler2
```

- Publish is non-blocking: if the buffer is full — error
- Dispatcher runs in a goroutine — events are processed asynchronously
- Multiple handlers for one event type — fan-out

---

## 📁 Module files

| File                     | What it does                                                 |
|--------------------------|--------------------------------------------------------------|
| `bus.go`                 | `Publisher`/`Consumer` interfaces + in-memory implementation |
| `bus_test.go`            | Test of pub/sub, multiple subscribers, graceful shutdown     |
| `idempotency.go` ⭐      | `IdempotentHandler` — processes each event exactly once      |
| `dlq.go` ⭐              | Dead Letter Queue + `RetryHandler` with retries              |
| `idempotency_test.go` ⭐ | Tests for idempotency, DLQ, and combinations                 |

---

## ⭐ Idempotent Consumer — exactly-once processing

```
Without idempotency:
  OrderCreated → handler → charge $100
  OrderCreated (replay!) → handler → charge $100 ← DUPLICATE!

With idempotency:
  OrderCreated → check ID → handler → charge $100 → mark ID
  OrderCreated (replay!) → check ID → already processed → SKIP ✓
```

```go
store := NewInMemoryIdempotentStore()  // In production: Redis or PostgreSQL
handler := NewIdempotentHandler("payment", store, func(e EnrichedEvent) error {
    return processPayment(e)
})

handler.Handle(event) // Processes
handler.Handle(event) // Skips (idempotent)
```

---

## ⭐ Dead Letter Queue (DLQ) — handling "poison" messages

```
Without DLQ:
  event → handler (error) → retry → handler (error) → ... forever!

With DLQ:
  event → handler (error) → retry 1 → retry 2 → retry 3 → DLQ
                                                              ↓
                                                  Alert → Developer investigates
```

```go
dlq := NewDeadLetterQueue()
rh := NewRetryHandler("payment", 3, dlq, func(e EnrichedEvent) error {
    return callPaymentService(e)
})

rh.Handle(event) // Tries 3 times, on failure → DLQ
dlq.Entries()    // See what's in the DLQ
```

### Combo: Idempotency + Retry + DLQ

```go
// Idempotency outside → Retry inside → DLQ when attempts are exhausted
retryH := NewRetryHandler("svc", 3, dlq, businessHandler)
idempotentH := NewIdempotentHandler("svc", store, func(e EnrichedEvent) error {
    return retryH.Handle(e)
})
```

---

## 💡 In a real project: Kafka vs RabbitMQ vs NATS

The `Publisher` and `Consumer` interfaces **don't change** — services don't know what got swapped!

| Broker       | Best for                    | Weaknesses         |
|--------------|-----------------------------|--------------------|
| **Kafka**    | High load, retained history | Complex setup      |
| **RabbitMQ** | Routing, priority queues    | Lower throughput   |
| **NATS**     | Simplicity, speed, embedded | Limited guarantees |

A detailed comparison lives in **Module 8**.

---

## 🏆 Messaging Best Practices

> **1. Name events in the past tense.**
> `order.created` — not `create.order`. It's a fact, not a command.

> **2. Make handlers idempotent.**
> Messages can be duplicated. Handle them twice — don't break anything.

> **3. Don't let an event handler get too smart.**
> It should do one thing. Otherwise it's a service, not a handler.

> **4. Include a correlation_id in the event.**
> For tracing a request across several services.

> **5. The Publisher should not wait for a response from the subscriber.**
> If you need a response — you need gRPC, not messaging.

---

## ▶️ How to run

```bash
go test ./04_messaging/... -v
```

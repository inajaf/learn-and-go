# Module 8 — Microservice Communication Patterns

## 📌 What you'll learn in this module

- **The main question**: when to use gRPC, when Kafka, when RabbitMQ?
- Synchronous vs asynchronous communication
- 3 real scenarios with solutions
- Architectural patterns: CQRS, Saga, Outbox
- A complete decision map

---

## 🗺️ The big picture: kinds of communication

```
                        SERVICE-TO-SERVICE COMMUNICATION
                                    │
              ┌─────────────────────┴─────────────────────┐
              │                                           │
         SYNCHRONOUS                              ASYNCHRONOUS
      (wait for a reply)                     (publish and move on)
              │                                          │
    ┌─────────┴──────────┐                    ┌──────────┴──────────┐
    │                    │                    │                     │
  REST/HTTP            gRPC              Message Queue          Event Bus
 (JSON, public)   (Protobuf,           (RabbitMQ,             (Kafka,
                  internal)            tasks)                 events)
```

---

## 🔑 The main question: sync or async?

This is the first question to ask yourself.

### Synchronous communication (gRPC / REST):

```
Service A              Service B
   │                      │
   │──── request ────────▶│
   │                      │ (handles)
   │◀─── response ────────│
   │ (only continues      │
   │  after a reply)      │
```

**Use it when:**
- ✅ You need an answer right now (reading data)
- ✅ The client can't proceed without a response
- ✅ Validation: you need to know if a check passed immediately
- ✅ Data queries (GET operations)

**Drawbacks:**
- ❌ If Service B is down — Service A goes down too
- ❌ Latency stacks: A→B→C→D = sum(all latencies)
- ❌ Cascade failures — one failure pulls others down

### Asynchronous communication (Kafka / RabbitMQ):

```
Service A     EventBus/Queue     Service B
   │               │                │
   │──── event ───▶│                │
   │ (continues    │                │
   │  immediately!)│──── event ────▶│
                                    │ (handles
                                    │  independently)
```

**Use it when:**
- ✅ Service A doesn't wait for a reply
- ✅ Several services react to one event
- ✅ Temporary unavailability is OK
- ✅ You need to process large volumes of data
- ✅ Decoupling matters more than response speed

**Drawbacks:**
- ❌ Harder to debug (no single trace)
- ❌ Eventual consistency — data will agree, but not immediately
- ❌ You need the infrastructure (Kafka/RabbitMQ cluster)

---

## 📊 REST vs gRPC vs Kafka vs RabbitMQ

### Detailed comparison

| Criterion | REST/HTTP | gRPC | Kafka | RabbitMQ |
|-----------|-----------|------|-------|----------|
| **Type** | Sync | Sync | Async | Async |
| **Protocol** | HTTP/1.1 | HTTP/2 | Binary (TCP) | AMQP |
| **Format** | JSON | Protobuf | Bytes (JSON/Avro) | JSON/bytes |
| **Speed** | ★★★ | ★★★★★ | ★★★★ | ★★★ |
| **Delivery guarantees** | At-most-once | At-most-once | At-least-once | At-least-once |
| **History retention** | ❌ | ❌ | ✅ (days/weeks) | ❌ (after consume) |
| **Routing** | URL | method | topic/partition | exchange/binding |
| **Fan-out** | No | No | ✅ consumer groups | ✅ bindings |
| **Complexity** | Low | Medium | High | Medium |
| **Browser support** | ✅ | ❌ | ❌ | ❌ |
| **Streaming** | ❌/SSE | ✅ native | ✅ native | ❌ |
| **When** | Public API | Internal API | Event streaming | Task queues |

---

## 🎯 Decision flowchart

```
Need an answer immediately?
├── YES ──▶ Internal service-to-service call?
│           ├── YES ──▶ gRPC (fast, typed)
│           └── NO ──▶ REST/HTTP (browser, partners)
│
└── NO ──▶ Need event history/replay?
           ├── YES ──▶ Kafka (event log, retention)
           └── NO ──▶ Need complex routing/priorities?
                       ├── YES ──▶ RabbitMQ (exchanges, routing)
                       └── NO ──▶ NATS (simple, fast)
```

---

## 🏗️ Scenario 1: E-commerce shop

```
┌──────────────────────────────────────────────────────────┐
│                     Client (browser)                     │
└──────────────────┬───────────────────────────────────────┘
                   │ REST/HTTP (client → API)
                   ▼
┌──────────────────────────────────────────────────────────┐
│                      API Gateway                         │
└───┬──────────────┬────────────────────────────┬──────────┘
    │ gRPC         │ gRPC                       │ gRPC
    ▼              ▼                            ▼
┌────────┐   ┌──────────┐                ┌────────────┐
│ Order  │   │ Inventory│                │  Payment   │
│Service │   │ Service  │                │  Service   │
└────┬───┘   └──────────┘                └────────────┘
     │
     │ Publish "order.created" event
     ▼
┌──────────────────────────────────────────────┐
│                   Kafka                      │
└────┬─────────────────────────────────────────┘
     │                │                │
     ▼                ▼                ▼
┌────────────┐  ┌─────────────┐  ┌─────────────┐
│Notification│  │  Analytics  │  │   Warehouse │
│ Service    │  │   Service   │  │   Service   │
└────────────┘  └─────────────┘  └─────────────┘
```

**Why gRPC between services?**
- OrderService → InventoryService: need the answer "is the item in stock?"
- OrderService → PaymentService: need the answer "did the payment go through?"
- Synchronous calls, immediate responses required

**Why Kafka for events?**
- After order creation we need to notify 3+ services
- The services don't depend on each other
- We need event history for analytics

---

## 🏗️ Scenario 2: Banking / Fintech

```
┌───────────────┐    gRPC      ┌─────────────────┐
│ Mobile App    │ ────────────▶│   API Gateway   │
└───────────────┘              └────────┬────────┘
                                        │ gRPC
                               ┌────────┴────────┐
                               │   Account Svc   │
                               └────────┬────────┘
                                        │
                          ┌─────────────┤
                          │             │
                          ▼             ▼
               ┌──────────────┐  ┌──────────────┐
               │  Debit Svc   │  │  Credit Svc  │
               │  (withdraw)  │  │  (deposit)   │
               └──────┬───────┘  └──────┬───────┘
                      │                 │
                      └────────┬────────┘
                               │ Kafka (transactions)
                               ▼
                    ┌──────────────────┐
                    │  Audit Service   │
                    │  (history)       │
                    └──────────────────┘
```

**Why gRPC for transactions?**
- Debit + credit must be atomic
- You need an immediate success/failure response
- Strict typing is critical for financial data

**Why Kafka for audit?**
- Every transaction must be recorded
- History must live for a long time (compliance)
- Audit must not block the transaction

---

## 🏗️ Scenario 3: Food delivery (Uber Eats)

```
┌─────────────┐                    ┌───────────────┐
│   Customer  │──── REST ─────────▶│  Order API    │
└─────────────┘                    └───────┬───────┘
                                           │ gRPC
                                    ┌──────┴──────┐
                                    │  Order Svc  │
                                    └──────┬──────┘
                                           │
                                           │ "order.placed" → Kafka
                                           │
                          ┌────────────────┤────────────┐
                          │                │            │
                          ▼                ▼            ▼
               ┌──────────────┐  ┌──────────────┐  ┌──────────┐
               │ Restaurant   │  │   Courier    │  │  Push    │
               │   Service    │  │   Service    │  │  Notify  │
               │ (accept?)    │  │ (find?)      │  │  Svc     │
               └──────┬───────┘  └──────┬───────┘  └──────────┘
                      │                 │
                      │ "order.accepted"│ "courier.assigned"
                      └────────┬────────┘
                               │ → Kafka
                               ▼
                    ┌──────────────────┐
                    │   Customer App   │
                    │ (live updates    │
                    │  via WebSocket)  │
                    └──────────────────┘
```

---

## 📐 Architectural patterns

### Saga Pattern — distributed transactions

When you need to "transactionally" update several services:

```
Choreography Saga (via events):

OrderSvc ──"order.created"──▶ InventorySvc
                                    │ "inventory.reserved"
                                    ▼
                               PaymentSvc
                                    │ "payment.processed"
                                    ▼
                               OrderSvc (updates the status)

On failure — compensating events:
"payment.failed" ──▶ InventorySvc "inventory.released"
                 ──▶ OrderSvc "order.cancelled"
```

### Outbox Pattern — reliable event publishing

The problem: save to DB + publish event → DB dies between the two!

```go
// The problem:
s.repo.Save(order)         // step 1
s.publisher.Publish(event) // step 2 — what if we crash between 1 and 2?

// The solution — Outbox Pattern:
// 1. In a SINGLE DB transaction: save the order + save the event in the outbox table
// 2. A separate worker reads the outbox and publishes events
// Guarantee: either both steps, or neither

tx.Begin()
tx.Insert(order)                    // into the orders table
tx.Insert(OutboxEvent{...})         // into the outbox table
tx.Commit()

// A separate worker:
for event := range outbox.Poll() {
    publisher.Publish(event)
    outbox.MarkProcessed(event.ID)
}
```

### CQRS — Command Query Responsibility Segregation

```
Writes (Commands):              Reads (Queries):
   OrderService                  ReadOrderService
       │                               │
       ▼                               ▼
   PostgreSQL                    Elasticsearch
  (normalized                    (denormalized,
   write schema)                  fast to search)
```

Commands and queries go through different models and different stores.
Synchronization — through events (Kafka).

---

## 🎯 Final decision map

```
QUESTIONS:

1. Do you need an immediate reply?
   YES → gRPC (internal) / REST (external)
   NO  → continue

2. Do you need a delivery guarantee?
   NO  → fire-and-forget (NATS)
   YES → continue

3. Do you need history/replay of events?
   YES → Kafka
   NO  → continue

4. Do you need complex routing/priorities?
   YES → RabbitMQ
   NO  → NATS or Kafka

5. One recipient or many?
   One  → Point-to-point queue (RabbitMQ)
   Many → Pub/Sub (Kafka topics)
```

---

## ⭐ Circuit Breaker — protection against cascading failures

```
Without a Circuit Breaker:
  OrderService → InventoryService (down, 30s timeout)
  → 100 requests hang for 30s each
  → OrderService runs out of goroutines
  → OrderService is down too!

With a Circuit Breaker:
  OrderService → CB → InventoryService (down)
  → 3 failures → CB opens
  → subsequent requests → instant failure (0ms instead of 30s)
  → OrderService alive, returns a fallback
```

```go
resilient := NewResilientInventoryChecker(inventory, 3, 30*time.Second)

available, err := resilient.CheckStock("item-1", 5)
if errors.Is(err, ErrCircuitOpen) {
    // Fallback: "item is probably available" / cached data
}
```

> **🏭 In production:** a Circuit Breaker wraps EVERY call to an external service.
> Libraries: `sony/gobreaker`, `afex/hystrix-go`.
> More detail: Module 13 (Production Patterns).

---

## 🔁 Retry with Exponential Backoff + Jitter

A Circuit Breaker protects against **long** failures. Retry protects against **short** ones —
a network blip, a timeout on an overloaded replica, a transient error.

### Naïve retry — an anti-pattern

```go
// ❌ BAD: fixed delay + every client retries at the same time
for i := 0; i < 3; i++ {
    err := call()
    if err == nil { return nil }
    time.Sleep(1 * time.Second)  // all 1000 clients hit at the same moment
}
```

If the service crashed and 1000 clients retry with a 1s delay — they all hit
simultaneously every second. The service can't recover. This is called
**thundering herd**.

### Exponential Backoff — delay grows exponentially

```
attempt 0: 100ms
attempt 1: 200ms
attempt 2: 400ms
attempt 3: 800ms
attempt 4: 1600ms   ← capped at maxDelay
```

Formula: `delay = baseDelay * 2^attempt`, capped at `maxDelay`.

### Jitter — randomness against synchronization

Even with exponential backoff, every client retries **the same way**. Add jitter —
a random multiplier to stretch load over time:

```go
delay := baseDelay * math.Pow(2, attempt)
if jitter {
    delay *= 0.5 + rand.Float64()  // multiplier in [0.5, 1.5)
}
```

This is the **"full jitter"** algorithm from AWS — recommended for everyone.

### Usage

```go
cfg := RetryConfig{
    MaxAttempts: 5,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    5 * time.Second,
    Jitter:      true,
}

err := Retry(cfg, func() error {
    return inventory.CheckStock("item-1", 5)
})
if errors.Is(err, ErrMaxRetriesExceeded) {
    // all attempts failed — fallback / open the circuit
}
```

### When retry DOESN'T help

Retry is only justified for **transient** errors:

- ✅ Network timeout, connection refused
- ✅ 503 Service Unavailable, 429 Too Many Requests
- ✅ Database deadlock, connection pool exhausted
- ❌ 400 Bad Request — the request is invalid, retry won't fix it
- ❌ 404 Not Found — the data isn't there, retry won't create it
- ❌ 401/403 — authz errors don't fix themselves through retry

**Rule:** retry for infrastructure errors, no retry for domain errors.

### Retry + Circuit Breaker — better together

```
Retry          → protects against SHORT failures (network blip)
Circuit Breaker → protects against LONG failures (service down)

    client → CB → Retry(fn) → inventory
             │      │
             │      └─ 5 attempts with backoff
             └─ if every attempt fails N times → open
```

> **🏭 In production:** combine both defenses. Retry inside CB: if retry exhausts
> its budget — register a failure in the CB. If CB is open — don't even retry.

See `retry.go` and `retry_test.go`.

---

## 📁 Module files

| File | What it does |
|------|--------------|
| `README.md` | This guide — theory and comparisons |
| `patterns.go` | Sync/Async patterns, Saga, Outbox |
| `patterns_test.go` | Pattern demonstrations with tests |
| `circuit_breaker.go` ⭐ | Circuit Breaker + ResilientInventoryChecker |
| `circuit_breaker_test.go` ⭐ | CB tests + Saga integration |
| `retry.go` ⭐ | Retry with exponential backoff + jitter |
| `retry_test.go` ⭐ | Retry tests, backoff timing, jitter |

---

## ❌ Common mistakes

1. **gRPC for everything** → use async for tasks that don't need an immediate reply
2. **Kafka for simple queues** → RabbitMQ or NATS are simpler and cheaper
3. **No Circuit Breaker** → one failed service brings the whole system down
4. **Saga without compensation** → on failure, data stays inconsistent
5. **Outbox without cleanup** → the outbox table grows forever

---

## 🔗 Related modules

- **← Module 03** (gRPC) — synchronous communication
- **← Module 04** (Messaging) — Pub/Sub, idempotency, DLQ
- **← Module 13** (Production) — retry + circuit breaker patterns
- **→ Module 09** (Complete Demo) — everything together in a working system

---

## ▶️ How to run

```bash
go test ./08_communication_patterns/... -v
```

# 🏆 Module 7 - Capstone: everything together

## 🎯 What's here?

A minimal Order Service that uses everything you've learned in previous modules.
This is a **reference** implementation - look at it as a template for your microservice.

| What | From |
|-----|--------|
| Interfaces + Repository Pattern | [Module 01](../01_interfaces/) |
| DTO and mapping | [Module 02](../02_dto/) |
| gRPC - transport layer | [Module 03](../03_grpc/) |
| Messaging - Events | [Module 04](../04_messaging/) |
| Unit test with mocks | [Unit 05](../05_unit_testing/) |
| Integration suite | [Module 06](../06_integration_testing/) |

---

## 🏗️ Architecture

Classic **pure architecture** with separation by layers:

```
┌───────────────────────────────────────────────────────────┐
│ TRANSPORT LAYER                                           │
│                                                           │
│   gRPC Client                                             │
│       │                                                   │
│       ▼                                                   │
│   GRPCHandler  ───► proto.CreateOrderRequest              │
│                            │                              │
└────────────────────────────┼──────────────────────────────┘
                             │ CreateOrderRequest (DTO)
                             ▼
┌───────────────────────────────────────────────────────────┐
│ BUSINESS LOGIC                                            │
│                                                           │
│   OrderService.CreateOrder(ctx, req)                      │
│       │                                                   │
│       ├── requestToDomain(req) ───► *Order (domain)       │
│       │                                                   │
│       ├── repo.Save(ctx, order)                           │
│       │       │                                           │
│       │       ▼                                           │
│       │ OrderRepository (interface)                       │
│       │                                                   │
│       ├── publisher.Publish("order.created", ...)         │
│       │       │                                           │
│       │       ▼                                           │
│       │ EventPublisher (interface)                        │
│       │                                                   │
│       └── domainToResponse(order)  ───► OrderResponse     │
│                                                           │
└───────────────────────────────────────────────────────────┘
                             │
                             ▼
┌───────────────────────────────────────────────────────────┐
│ INFRASTRUCTURE                                            │
│                                                           │
│   InMemoryOrderRepository      LogPublisher               │
│   PostgresOrderRepository      KafkaPublisher             │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

---

## 🔄 CreateOrder request trace

Step by step what happens in `CreateOrder(ctx, req)`:

```
1. DTO Validation
   requestToDomain(req)
     ├── CustomerID != "" ?
     ├── len(Items) > 0 ?
     └── every item.Quantity > 0 ?
↓ if ok
*Order (domain model with ID, Status=Pending)

2. Calculation of the amount (business logic IN THE DOMAIN, not in the service!)
   order.TotalAmount = order.TotalPrice()
     └── count by items

3. Preservation (through abstraction)
   repo.Save(ctx, order)
     └── InMemoryRepository OR Postgres - the service does not know

4. Publishing an event (non-fatal - only a warning)
   _ = publisher.Publish("order.created", {...})

5. Convert back to DTO
domainToResponse(order) ───► OrderResponse (for client)
```

**Key Point:** `OrderService` never imports infrastructure packages.
It ONLY knows about the `OrderRepository` and `EventPublisher` interfaces.

---

## 💡 Dependency Inversion in action

Look at the direction of dependencies - they always point DOWN (to abstractions):

```
        ┌──────────────┐
        │ GRPCHandler  │
        └──────┬───────┘
               │ imports
               ▼
        ┌──────────────┐
        │ OrderService │ ◄──── high-level logic
        └──────┬───────┘
               │ depends on interfaces
               ▼
     ┌─────────────────────┐
     │ OrderRepository     │ ◄──── ABSTRACTIONS
     │ EventPublisher      │ (in the same package as the service)
     └─────────┬───────────┘
               │ implement
               ▼
  ┌────────────────────────────┐
  │ InMemoryRepo, PostgresRepo │ ◄──── low-level details
  │ LogPublisher, KafkaPub     │
  └────────────────────────────┘
```

**Rule:** High-level modules do not depend on low-level ones.
Both depend on abstractions. This makes the service **testable** and **replaceable**.

---

## 🧪 How to test

Thanks to interfaces, each layer is tested in isolation:

| Layer | What we wet | What we check |
|------|------------|---------------|
| `OrderService` | `OrderRepository`, `EventPublisher` | Business logic, validation, call order |
| `OrderRepository` (real implementation) | Nothing (integration test) | SQL, mapping, concurrency |
| `GRPCHandler` | `OrderService` | Broadcast proto ↔ DTO, error codes |

See `service_test.go` for an example of a unit test with mocks.

---

## 🏭 In production you would also add

This capstone is a **minimal** tutorial example. In a real service, add:

1. **context.Context propagation** - deadline / cancel pass through all layers ([Module 13](../13_production_patterns/))
2. **Optimistic locking** — `Version int` in `Order`, `UPDATE ... WHERE version = ?`
3. **CancelOrder with compensation** - saga pattern: roll back the debit/reserve of an item
4. **Outbox pattern** - instead of `publisher.Publish` writing an event to the `outbox` table in one transaction with `Save`, a separate worker sends it to Kafka. Guaranteed at-least-once.
5. **Structured logging** - `slog.Logger` to all services, `slog.With("order_id", id)` ([Module 13](../13_production_patterns/))
6. **Metrics** — counters of created orders, latency histogram
7. **Circuit breaker** for external calls ([Module 08](../08_communication_patterns/))

---

## ❌ Common mistakes

1. **Business logic in the handler** → the handler only translates the DTO ↔ service
2. **The service imports the database package** → interface only, implementation via DI
3. **Domain model with JSON tags** → tags only on DTO, clean domain
4. **One big `Order` for everything** → different DTOs for Create / Update / Response
5. **`Publish` in the same transaction as `Save`** → if the broker is lying, the order will not be saved. Use Outbox.

---

## ▶️ Launch

```bash
go test ./07_capstone/... -v
```

---

## 🔗 Related modules

- **← Module 06** (Integration Testing) - testing via suite
- **→ Module 08** (Communication Patterns) — circuit breaker, retry for external calls
- **→ Module 09** (Complete Demo) - a more complete example with the same patterns
- **→ Module 13** (Production Patterns) – context, graceful shutdown, slog

# Learn and Go - Visual Guide

Use this guide before opening a module README. It gives you the mental model first, then points you to the code that makes the model real.

## 1. Course Map

```mermaid
flowchart TD
    subgraph Foundation
        P["11 Pointers"]
        I["01 Interfaces"]
        E["14 Error Handling"]
        D["02 DTO"]
    end

    subgraph Edges["API and Messaging Edges"]
        H["12 HTTP API"]
        G["03 gRPC"]
        M["04 Messaging"]
    end

    subgraph Operations["Production and Architecture"]
        PP["13 Production Patterns"]
        C["15 Concurrency"]
        CP["08 Communication Patterns"]
    end

    subgraph Tests
        U["05 Unit Testing"]
        IT["06 Integration Testing"]
    end

    subgraph Final["Putting It Together"]
        CAP["07 Capstone"]
        DEMO["09 Complete Demo"]
        DB["10 Database"]
    end

    P -. "optional refresher" .-> I
    I --> D
    I --> E
    D --> H
    E --> H
    H --> G
    H --> M
    PP --> H
    PP --> G
    PP --> M
    C --> PP
    G --> CP
    M --> CP
    I --> U
    U --> IT
    D --> CAP
    E --> CAP
    H --> CAP
    G --> DEMO
    M --> DEMO
    CP --> DEMO
    DB --> DEMO
```

This map shows relationships between topics. It is not a strict order. For example, Module 10 is a database deep dive, but the complete demo also teaches how a final system is wired together.

## 2. One Request Through The System

```mermaid
sequenceDiagram
    autonumber
    participant Client
    participant HTTP as HTTP Handler
    participant Service as OrderService
    participant Repo as Repository
    participant Inv as Inventory
    participant Bus as EventBus
    participant Worker as Subscribers

    Client->>HTTP: POST /orders
    HTTP->>HTTP: decode + validate JSON
    HTTP->>Service: CreateOrder(ctx, request)
    Service->>Inv: HasStock(ctx, product, qty)
    Inv-->>Service: ok
    Service->>Repo: Save(ctx, order)
    Repo-->>Service: saved
    Service->>Bus: publish order.created
    Bus-->>Worker: notify + analytics
    Service-->>HTTP: OrderResponse
    HTTP-->>Client: 201 Created
```

This single flow connects most modules: DTO mapping, interfaces, context, repository, sync communication, async events, and tests.

## 3. Layer Boundaries

```mermaid
flowchart LR
    Transport["Transport DTO\nJSON / gRPC / protobuf"] --> Mapper["Mapper\nvalidate + convert"]
    Mapper --> Domain["Domain Model\nbusiness rules"]
    Domain --> Ports["Interfaces / Ports\nRepository, Publisher, StockChecker"]
    Ports --> Adapters["Adapters\nPostgres, Kafka, gRPC client, in-memory test fake"]
```

Rule of thumb: dependencies point inward. The domain should not import HTTP, database, Kafka, or gRPC packages.

## 4. Communication Decision Tree

```mermaid
flowchart TD
    A{"Need an answer now?"}
    A -- "yes" --> B{"Internal service call?"}
    B -- "yes" --> GRPC["gRPC\nfast, typed, internal"]
    B -- "no" --> REST["REST/HTTP\npublic, browser-friendly"]
    A -- "no" --> C{"Need replay/history?"}
    C -- "yes" --> KAFKA["Kafka\nevent log, analytics, fan-out"]
    C -- "no" --> D{"Need routing or priorities?"}
    D -- "yes" --> RABBIT["RabbitMQ\ntask queues, routing"]
    D -- "no" --> NATS["NATS/simple pub-sub\nlow ceremony"]
```

Use this before choosing a protocol. Most architecture mistakes come from forcing one tool into every communication problem.

## 5. Testing Pyramid For This Repo

```mermaid
flowchart TD
    Unit["Unit tests\nfast, mocks/spies/fakes\n05_unit_testing"] --> Integration["Integration tests\nreal components together\n06_integration_testing"]
    Integration --> Contract["Transport tests\ngRPC bufconn, HTTP httptest\n03_grpc + 12_http_api"]
    Contract --> System["System demo tests\ncomplete flow\n09_complete_demo"]
```

Prefer many unit tests, enough integration tests to prove wiring, and a small number of system tests for the happy path plus critical failures.

## 6. Code Sample: Interface-First Service

```go
type OrderRepository interface {
    Save(ctx context.Context, order *Order) error
    FindByID(ctx context.Context, id string) (*Order, error)
}

type EventPublisher interface {
    Publish(eventType string, payload any) error
}

type OrderService struct {
    repo      OrderRepository
    publisher EventPublisher
}

func NewOrderService(repo OrderRepository, publisher EventPublisher) *OrderService {
    return &OrderService{repo: repo, publisher: publisher}
}
```

The service accepts behavior, not a concrete database or broker. That is why the same business logic can run with in-memory fakes in tests and real adapters in production.

## 7. Code Sample: Handler Shape

```go
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    var req CreateOrderRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid JSON")
        return
    }

    resp, err := h.service.CreateOrder(ctx, req)
    if err != nil {
        writeError(w, mapError(err), err.Error())
        return
    }

    writeJSON(w, http.StatusCreated, resp)
}
```

Most HTTP handlers in production should follow this boring shape: decode, validate, call service, map errors, encode response.

## 8. Code Sample: Reliable Event Publishing Shape

```go
func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest) error {
    return s.tx.Run(ctx, func(ctx context.Context) error {
        order := requestToDomain(req)

        if err := s.orders.Save(ctx, order); err != nil {
            return err
        }

        return s.outbox.Save(ctx, OutboxEvent{
            Type: "order.created",
            Payload: orderCreatedPayload(order),
        })
    })
}
```

Save the business record and the event record in one transaction. A worker publishes the outbox later. This avoids the classic "DB saved, event lost" failure.

## 9. Study Checklist

- Read the visual diagram first.
- Open the module README and skim the "what you learn" section.
- Read the production-shaped code.
- Run that module's tests.
- Break one test on purpose, then fix it.
- Move to the next module only when you can explain the main interface and the main failure mode.

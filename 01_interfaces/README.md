# Module 1 - Interfaces and Patterns

## 📌 What will you learn in this module

- What is an interface and why is it needed (in depth, with examples)
- Duck Typing in Go vs explicit implementation in Java/C#
- The empty `any` interface and when NOT to use it
- Nil interface is the most common Go trap
- Repository Pattern - hide the repository behind the contract
- Decorator Pattern - add behavior without changing the code
- SOLID principles in Go
- Best practices of a senior developer

---

## ❓ Why do we need interfaces at all?

Imagine that you are writing an ordering service. You need to save orders somewhere.
Where exactly - in memory, in PostgreSQL, in MongoDB - you don’t want to decide right now.

### ❌ No interface (bad):

```go
type OrderService struct {
db *PostgresDB // hard link to Postgres
}

func NewOrderService() *OrderService {
db := postgres.Connect("host=localhost...") // need real Postgres
    return &OrderService{db: db}
}
```

**Problems:**
1. It’s impossible to write a test - you always need real Postgres
2. Do you want to change to MongoDB? You rewrite the entire service
3. You can’t start a service without a database (even locally)

### ✅ With interface (correct):

```go
// Contract: "someone who knows how to work with orders"
type OrderRepository interface {
    Save(order *Order) error
    FindByID(id string) (*Order, error)
    FindAll() ([]*Order, error)
    Delete(id string) error
}

type OrderService struct {
repo OrderRepository // just a contract - it doesn’t matter what’s inside
}

// In tests we serve InMemory, in production - PostgresRepo
func NewOrderService(repo OrderRepository) *OrderService {
    return &OrderService{repo: repo}
}
```

This is the **Dependency Inversion Principle (DIP)** - the D in SOLID.

---

## 🦆 Duck Typing is a Go feature

In Go, interfaces are implemented **implicitly**. This is called *structural typing* or *Duck Typing*:

> "If it looks like a duck and quacks like a duck, it's a duck"

### Comparison with other languages:

```java
// Java - EXPLICIT implementation (you need to write implements):
class InMemoryRepo implements OrderRepository { // ← required!
    ...
}
```

```go
// Go - IMPLICIT implementation (just implement the methods):
type InMemoryRepo struct { ... }

func (r *InMemoryRepo) Save(order *Order) error    { ... }
func (r *InMemoryRepo) FindByID(id string) (*Order, error) { ... }
func (r *InMemoryRepo) FindAll() ([]*Order, error)  { ... }
func (r *InMemoryRepo) Delete(id string) error      { ... }

// All! InMemoryRepo automatically implements OrderRepository.
// No "implements" needed.
```

### Why it's cool:

```go
// The library has defined an interface:
type Writer interface {
    Write(p []byte) (n int, err error)
}

// Your type from another package:
type MyLogger struct{}
func (l *MyLogger) Write(p []byte) (int, error) { ... }

// MyLogger automatically implements io.Writer - even without importing the library!
// You can pass it to where io.Writer is expected.
```

---

## 🕳️ Empty interface `any` (interface{})

`any` is an alias for `interface{}`. It is satisfied by **any type** in Go,
because any type implements "zero methods".

```go
var x any = 42          // int
var y any = "hello"     // string
var z any = &Order{}    // *Order

fmt.Println(x, y, z) // works with any type
```

### When `any` is justified:

```go
// ✅ OK: universal logging/serialization functions
func log(fields map[string]any) { ... }

// ✅ OK: JSON parsing when the structure is unknown
var result map[string]any
json.Unmarshal(data, &result)

// ✅ OK: Event payload (event data of different types)
type Event struct {
    Type    string
Payload any // specific type depends on Type
}
```

### When `any` is a red flag:

```go
// ❌ BAD: lost typing - the compiler will not help
func ProcessOrder(order any) {
o := order.(*Order) // panic at runtime if not *Order was passed!
}

// ✅ GOOD: concrete type or interface
func ProcessOrder(order *Order) { ... }
// or
func ProcessOrder(order OrderDomainInterface) { ... }
```

---

## ⚠️ Nil Interface - the most common Go trap

This **must be understood**. Mishandling nil causes panic.

```go
// The interface inside is two fields: (type, value)
// nil interface: (nil, nil)
//NON-nil interface with nil value: (*PostgresRepo, nil)

func getRepo() OrderRepository {
var repo *PostgresRepo = nil // ← concrete type nil
    return repo // ATTENTION: this is NOT a nil interface!
}

var r OrderRepository = getRepo()
fmt.Println(r == nil) // false! ← shock

// Why? Because interface = (*PostgresRepo, nil)
// It contains type information - it's not nil!
```

### How to correctly:

```go
// ✅ Return nil explicitly:
func getRepo() OrderRepository {
    if someCondition {
return nil // real nil interface
    }
    return &PostgresRepo{}
}

// ✅ Or return an error instead of nil:
func getRepo() (OrderRepository, error) {
    if someCondition {
        return nil, errors.New("not configured")
    }
    return &PostgresRepo{}, nil
}
```

---

## 📐 How to design an interface correctly

### Principle: small interfaces are better than big ones (ISP from SOLID)

```go
// ❌ BAD: bold interface - difficult to mock
type OrderRepository interface {
    Save(order *Order) error
    FindByID(id string) (*Order, error)
    FindAll() ([]*Order, error)
    Delete(id string) error
    FindByCustomerID(customerID string) ([]*Order, error)
    FindByStatus(status OrderStatus) ([]*Order, error)
    FindByDateRange(from, to time.Time) ([]*Order, error)
    CountByCustomer(customerID string) (int, error)
    SumByCustomer(customerID string) (float64, error)
// ... 10 more methods
}

// ✅ GOOD: small interfaces for the task
type OrderWriter interface {
    Save(order *Order) error
    Delete(id string) error
}

type OrderReader interface {
    FindByID(id string) (*Order, error)
    FindAll() ([]*Order, error)
}

type OrderRepository interface {
    OrderWriter
    OrderReader
}

// Service for creating orders - only Writer is needed:
type CreateOrderService struct {
writer OrderWriter // not the entire repository!
}

// Reporting service - Reader only:
type ReportService struct {
    reader OrderReader
}
```

### Principle: the interface is determined by the consumer, not the manufacturer

```go
// ❌ WRONG: the postgres package defines an interface for itself
// package postgres
type PostgresRepository interface { ... }

// ✅ CORRECT: the service package defines what it needs
// package service
type OrderRepository interface {
    Save(order *Order) error
    FindByID(id string) (*Order, error)
}
// And postgres.Repo implicitly implements this interface!
```

---

## 📐 Repository Pattern

Repository is a pattern that hides the logic of working with data behind the interface.

```
┌─────────────┐        ┌─────────────────────┐
│ OrderService│───────▶│ OrderRepository │ ← interface (contract)
└─────────────┘        └─────────────────────┘
                              ▲         ▲         ▲
                    ┌─────────┘    ┌────┘    ┌────┘
               ┌────────────┐  ┌───────┐  ┌──────────┐
               │  InMemory  │  │Postgres│  │  Redis   │
               └────────────┘  └───────┘  └──────────┘
(tests) (prod) (cache)
```

The service only knows about the interface. He doesn't know what's inside.
This allows you to:
1. Write tests without a real database (InMemory)
2. Change the database without changing the service (Postgres → MySQL)
3. Add cache transparently (Decorator)

---

## 🎨 Decorator Pattern

Look at `LoggingRepository` in `repository.go`.
This is the **Decorator** pattern - we wrap one implementation into another.

```
Service
  │
  ▼
LoggingRepository ← adds logging
  │
  ▼
MetricsRepository ← adds metrics (you can add!)
  │
  ▼
InMemoryRepository ← real storage
```

Each layer implements the same `OrderRepository` interface.
The service does not know how many layers there are between it and the real storage.

### In a real project the chain looks like this:

```go
//Build the chain from bottom to top:
rawRepo   := postgres.NewRepository(db)
cachedRepo := cache.NewDecorator(rawRepo, redis)
loggedRepo := logging.NewDecorator(cachedRepo, logger)
metricsRepo := metrics.NewDecorator(loggedRepo, prometheus)

// The service receives the top layer - but the interface is the same:
svc := NewOrderService(metricsRepo)
```

---

## 🔑 SOLID principles in Go

### S — Single Responsibility
Each type does one thing:
- `Order` - domain model (business rules)
- `OrderRepository` - storage contract
- `OrderService` - business logic orchestration

### O — Open/Closed
Open to expansion, closed to change.
Do you want to add cache? Create a `CachedRepository` - don't change existing code.

### L — Liskov Substitution
Any implementation of `OrderRepository` is interchangeable.
`PostgresRepo`, `InMemoryRepo`, `MockRepo` - work the same from the service point of view.

### I — Interface Segregation
Small interfaces are better than one big one. Shown above.

### D — Dependency Inversion
The service depends on the **abstraction** (`OrderRepository`) rather than the **concreteness** (`PostgresRepo`).

---

## 🔍 Checking interface implementation at compilation stage

```go
// This is a compile-time check - the program will not compile
// if InMemoryOrderRepository does not implement OrderRepository
var _ OrderRepository = (*InMemoryOrderRepository)(nil)

// Can be added to the beginning of the implementation file.
// This is best practice - we explicitly declare the intention.
```

---

---

## 🏗️ Interface-First Design - design through interfaces

You're absolutely right: **interfaces are the main design tool** in Go.
A senior developer starts not with code, but with contracts.

### Process: from idea to code

```
STEP 1: Draw the system on paper
         "What should the system do?"

STEP 2: Describe CONTRACTS (interfaces)
         "What does each part need from the other parts?"

STEP 3: Write tests against contracts
         “What would the right job look like?”

STEP 4: Write Stubs - tests are compiled
         "Minimal implementation to avoid compilation errors"

STEP 5: Write real implementations
         "Now let's connect real SendGrid, PostgreSQL, Kafka..."

STEP 6: Replace stubs with real implementations
         "The service and tests do not change - we just pass another object"
```

### Example: notification system - STEP 2 (contracts only)

```go
// Describe WHAT is needed. HOW is not important yet.

type Notifier interface {
    NotifyOrderConfirmed(email, orderID string, amount float64) error
}

type WarehouseNotifier interface {
    NotifyNewOrder(orderID string, items []string) error
}

type AnalyticsTracker interface {
    TrackOrderConfirmed(orderID, customerID string, amount float64)
}

// The service is written through contracts - there are no implementations yet!
type OrderConfirmationService struct {
    orders    OrderRepository
    notifier  Notifier
    warehouse WarehouseNotifier
    analytics AnalyticsTracker
}
```

### STEP 4: Stubs vs Spy

```go
// Stub - just “silent”, does nothing.
// We need the code to compile while there is no implementation.
type StubNotifier struct{}
func (s *StubNotifier) NotifyOrderConfirmed(_, _ string, _ float64) error { return nil }

// Spy - remembers calls. Used in tests.
// "Did we call NotifyOrderConfirmed after confirmation?"
type SpyNotifier struct {
    Calls []NotifyCall
}
func (s *SpyNotifier) NotifyOrderConfirmed(email, orderID string, amount float64) error {
    s.Calls = append(s.Calls, NotifyCall{email, orderID, amount})
    return nil
}
```

### STEP 5: Real implementations - do not change the service!

```go
// SendGrid, Twilio, BigQuery - implement the same interfaces.
type SendGridNotifier struct { APIKey string }
func (n *SendGridNotifier) NotifyOrderConfirmed(email, orderID string, amount float64) error {
// real HTTP call to SendGrid
}

// Connect to main.go - the service does not change:
svc := NewOrderConfirmationService(
postgresRepo, // was: InMemoryRepo
    &SendGridNotifier{...}, // was: StubNotifier
    &TwilioNotifier{...}, // was: StubWarehouseNotifier
    &BigQueryTracker{...}, // was: StubAnalyticsTracker
)
```

### Why it works: one interface - many implementations

```
Notifier (interface)
    │
├── StubNotifier - development (does nothing)
    ├── SpyNotifier - tests (remembers calls)
    ├── SendGridNotifier - email production
    ├── SMTPNotifier - alternative email
    └── LoggingNotifier — Decorator: logs + delegates
```

The service accepts `Notifier` - it **doesn't care** what implementation is inside.

---

## 🔌 Hexagonal Architecture (Ports & Adapters)

Interfaces are **ports**. Implementations are **adapters**.

```
                    ┌─────────────────────────────┐
│ SYSTEM CORE │
                    │ (business logic) │
                    │                             │
HTTP request ──▶ │ OrderConfirmationService │
                    │ Uses only │
Kafka Event ──▶ │ INTERFACES (ports): │
                    │  - OrderRepository          │
CLI command ──▶ │ - Notifier │
                    │  - AnalyticsTracker         │
                    └─────────────────────────────┘
                           │         │
                    ┌──────┘         └──────┐
               PostgreSQL              SendGrid
(adapter) (adapter)
```

The core **doesn't know** what's outside. Outsiders **don't know** about the core.
Communication only through interfaces.

---

## 🎭 Three types of test doubles

| Type | What does | When to use |
|-----|-----------|-------------------|
| **Stub** | Returns canned responses, does not check anything | When a dependency is not important for a test |
| **Spy** | Remembers calls, you can check later | When you need to make sure that a method has been called |
| **Mock** | Sets expectations in advance | Module 5 - testify/mock |

```go
// Stub - “I don’t care, just work”
notifier := &StubNotifier{}

// Spy - “remember what happened”
spy := &SpyNotifier{}
// ... after the test:
assert.True(t, spy.Called())
assert.Equal(t, "alice@example.com", spy.Calls[0].Email)

// Mock (Module 5) - "I'm expecting a specific call"
mock := new(MockNotifier)
mock.On("NotifyOrderConfirmed", "alice@example.com", mock.Anything, 99.99).Return(nil)
```

---

## 🔄 context.Context - required first parameter

In Go, `context.Context` is passed as the **first parameter** to all interface methods,
that perform I/O operations (DB, HTTP, gRPC, files).

```go
// ❌ BAD: no context
type OrderRepository interface {
    Save(order *Order) error
    FindByID(id string) (*Order, error)
}

// ✅ GOOD: context as the first parameter
type OrderRepository interface {
    Save(ctx context.Context, order *Order) error
    FindByID(ctx context.Context, id string) (*Order, error)
}
```

**Why?**
- **Timeouts**: `ctx, cancel := context.WithTimeout(ctx, 5*time.Second)`
- **Cancel**: user has left → cancel the database request
- **Tracing**: request ID is passed through the entire call chain
- **Graceful shutdown**: server stops → all requests are canceled

> **🏭 In production:** if the interface method does not accept `context.Context` -
> this is a red flag. This means that it cannot be canceled and cannot be limited in time.

See `service.go`, `repository.go` - all methods accept `ctx`.

---

## ⚙️ Functional Options - flexible configuration

Pattern for constructors with optional parameters:

```go
// Instead of a constructor with 10 parameters:
svc := NewOrderService(repo, logger, publisher, timeout, maxRetries)

// Use options:
svc := NewOrderService(repo,
    WithLogger(logger),
    WithEventPublisher(pub),
)
```

See `options.go` - implementation of the pattern.

---

## 📁 Module files

| File | What does |
|------|-----------|
| `domain.go` | `Order` domain model - clean structure + business methods |
| `errors.go` | Sentinel errors - named errors for `errors.Is()` |
| `repository.go` | Interface + InMemory implementation + LoggingDecorator |
| `service.go` | Service - business logic through interfaces (DI) |
| `service_test.go` | Tests - table-driven, decorator pattern, sentinel errors |
| `design.go` | **Interface-First Design** - Stub, Spy, Hexagonal, Export patterns |
| `design_test.go` | Tests - implementation substitution, Spy checks, Cache Decorator |

---

## 🏆 Main rules of a senior developer

> **1. Accept an interface, return a concrete type.**
> Functions should accept interfaces (flexibility) and return concrete types (predictability).

> **2. Don't create an interface in advance - create it when the abstraction is needed.**
> If there is one implementation and no other is planned, the interface is redundant.

> **3. The interface should be small - 1-3 methods.**
> The smaller the interface, the easier it is to implement, test, and mock.

> **4. The interface is defined by the consumer (service package), not the producer (postgres package).**

> **5. Use compile-time check `var _ Interface = (*Impl)(nil)`.**

---

## ▶️ Launch

```bash
go test ./01_interfaces/... -v
```

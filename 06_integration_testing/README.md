# 🔗 Module 6 — Integration Tests and Test Suite

## ❓ What is testify Suite?

`testify/suite` lets you group tests into a struct with shared `Setup` and `TearDown`.

This is especially valuable for **integration tests**, where you need:
- To spin up the "infrastructure" (repository, service) once before every test
- To reset state before each test
- To stop resources after all tests

---

## 🗓️ Suite lifecycle

```
SetupSuite()         ← once: bring up Docker, connect to DB, run migrations
  ↓
SetupTest()          ← before EACH test: BEGIN TRANSACTION
  ↓
Test_A()             ← the test runs on clean data
  ↓
TearDownTest()       ← after EACH test: ROLLBACK (data cleared!)
  ↓
SetupTest()          ← clean state again
  ↓
Test_B()
  ↓
TearDownTest()
  ↓
TearDownSuite()      ← once: stop Docker, close the connection
```

---

## 🔀 Difference from unit tests (Module 5)

| | Unit test (Module 5) | Integration test (Module 6) |
|---|----------------------|------------------------------|
| Dependencies | Mock (fake) | Real implementation |
| Speed | Fast (ms) | Slower (seconds) |
| What it tests | One component's logic | Component interactions |
| Setup | Not needed | SetupSuite/SetupTest |
| Isolation | Automatic (mocks) | Via transactions/cleanup |

```
Unit test:        Service → MockRepo → ✅ logic only
Integration test: Service → InMemoryRepo → ✅ interactions
Production test:  Service → PostgresRepo → ✅ real DB
```

---

## 🏗️ Test isolation strategies

### 1. Transaction Rollback (recommended for databases)

```go
func (s *MySuite) SetupTest() {
    tx, _ := s.db.Begin()       // Start a transaction
    s.repo = NewRepo(tx)
}

func (s *MySuite) TearDownTest() {
    s.tx.Rollback()             // Roll back ALL changes
}
```

👉 **The fastest approach** — no real writes or deletes.
Used in module 10 (PostgreSQL).

### 2. Fresh State (used here)

```go
func (s *MySuite) SetupTest() {
    s.repo = NewInMemoryRepo()  // A fresh repository for each test
    s.service = NewService(s.repo, s.publisher)
}
```

👉 **A simple approach** — each test starts from zero.

### 3. Truncate Tables

```go
func (s *MySuite) SetupTest() {
    s.db.Exec("TRUNCATE orders, customers RESTART IDENTITY CASCADE")
}
```

👉 **Slower** than rollback but works across several transactions.

---

## 🐳 Testcontainers (advanced)

In production projects integration tests bring real dependencies
(PostgreSQL, Redis, Kafka) up via Docker right from the test code:

```go
//go:build integration

func (s *MySuite) SetupSuite() {
    ctx := context.Background()
    container, _ := postgres.RunContainer(ctx,
        testcontainers.WithImage("postgres:15"),
    )
    s.dbURL = container.ConnectionString(ctx)
    s.db = sqlx.MustConnect("postgres", s.dbURL)
}
```

### Build tags — splitting tests

```go
//go:build integration  ← this file is compiled ONLY with the tag

// go test ./... -tags=integration
```

```bash
# Unit tests (fast, in CI on every commit):
go test ./...

# Integration tests (slow, in CI before merge):
go test ./... -tags=integration
```

---

## ▶️ How to run

```bash
go test ./06_integration_testing/... -v
```

---

## ❌ Common mistakes

1. **Tests depend on order** → each test must create its own data
2. **No cleanup** → one test's data contaminates another's
3. **Docker booted on every test** → use SetupSuite (once), SetupTest (cleanup)
4. **Integration tests without build tags** → `go test ./...` fails without Docker

> **🏭 In production:**
> - Testcontainers for PostgreSQL/Redis/Kafka
> - Transaction rollback for isolation
> - Build tags to split unit/integration
> - CI runs integration tests as a separate stage

---

## 🔗 Related modules

- **← Module 05** (Unit Testing) — mocks vs real dependencies
- **→ Module 10** (Database) — transaction rollback for PostgreSQL
- **→ Module 12** (HTTP API) — httptest for testing the API

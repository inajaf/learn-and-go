# 🧪 Module 5 — Unit Tests with Mocks

## ❓ What is a mock and why do you need one?

A **mock** is a stub that mimics a real dependency.

Say your service uses an `OrderRepository` (a database).
In a unit test you don't want to spin up a real database. You want to:
1. Verify the service's logic **in isolation**
2. Control what the "database" returns
3. Assert the service called the right methods

---

## 🔀 Difference: Unit vs Integration test

| | Unit test | Integration test |
|---|-----------|------------------|
| Dependencies | Mocks (stubs) | Real (or close to real) |
| Speed | Very fast | Slower |
| Isolation | Complete | Tests interactions |
| What it checks | The logic of a single component | How components work together |

```
Unit test:
  OrderService → MockRepository → ✅ verifies ONLY the service's logic

Integration test:
  OrderService → InMemoryRepository → ✅ verifies the interaction
```

---

## 🎭 Two approaches to mocks in Go

### 1. Manual Mock — `mocks.go`

```go
type ManualMockRepository struct {
    SaveFunc     func(*Order) error
    SaveCalls    []*Order           // 👉 Recording calls so we can assert on them
}

func (m *ManualMockRepository) Save(order *Order) error {
    m.SaveCalls = append(m.SaveCalls, order)
    if m.SaveFunc != nil {
        return m.SaveFunc(order)
    }
    return nil
}
```

**Pros:** simple, clear, full control
**Cons:** lots of hand-written code

### 2. testify/mock — `service_test.go`

```go
type TestifyMockRepository struct { mock.Mock }

func (m *TestifyMockRepository) Save(order *Order) error {
    args := m.Called(order)
    return args.Error(0)
}

// In the test:
mockRepo.On("Save", mock.Anything).Return(nil)
mockRepo.AssertExpectations(t) // 👉 checks ALL expectations
```

**Pros:** less code, built-in call/argument checking
**Cons:** harder to read for newcomers, runtime checks (not compile-time)

---

## 📝 Table-Driven Tests — THE core pattern in Go

```go
func TestPlaceOrder_TableDriven(t *testing.T) {
    tests := []struct {
        name       string
        customerID string
        amount     float64
        wantErr    bool
    }{
        {"successful creation", "cust-1", 100.0, false},
        {"empty customerID", "", 100.0, true},
        {"zero amount", "cust-1", 0, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // 👉 Run in parallel
            // ... test
        })
    }
}
```

### Benefits of table-driven:
- All cases in one place — easy to see coverage
- Easy to add a case — one line
- `t.Run` creates a sub-test — the report shows **which** one failed
- You can run one case: `go test -run TestPlaceOrder/empty_customerID`

---

## 🔧 Useful testing tools

### t.Helper() — clean assertion helpers

```go
func assertOrderValid(t *testing.T, order *Order) {
    t.Helper() // 👉 On failure, show the CALLER's line, not a line inside the helper
    assert.NotEmpty(t, order.ID)
    assert.Greater(t, order.Amount, 0.0)
}
```

### t.Parallel() — parallel tests

```go
func TestSomething(t *testing.T) {
    t.Parallel() // 👉 This test runs in parallel with others

    // ⚠️ Safe only when there's no shared state!
}
```

### t.Cleanup() — deferred cleanup

```go
func TestWithDB(t *testing.T) {
    db := setupTestDB()
    t.Cleanup(func() {
        db.Close() // 👉 Runs AFTER the test (even on panic)
    })
}
```

### Subtests — grouping tests

```go
func TestOrderService(t *testing.T) {
    t.Run("create", func(t *testing.T) {
        t.Run("success", func(t *testing.T) { ... })
        t.Run("validation error", func(t *testing.T) { ... })
    })
    t.Run("get", func(t *testing.T) { ... })
}
```

```
=== RUN   TestOrderService/create/success        ✓
=== RUN   TestOrderService/create/validation_error ✓
=== RUN   TestOrderService/get                     ✓
```

---

## 📊 Benchmark — measuring performance

Go has built-in benchmarking. Functions prefixed with `Benchmark*`
and taking `*testing.B` are run via `go test -bench=.`.

```go
func BenchmarkPlaceOrder(b *testing.B) {
    repo := &ManualMockRepository{}
    pub := &ManualMockPublisher{}
    svc := NewOrderService(repo, pub)

    b.ResetTimer() // reset the timer after setup

    for b.Loop() { // Go 1.24+: new idiomatic syntax
        _, _ = svc.PlaceOrder("cust-bench", 99.99)
    }
}
```

Run with memory stats:
```bash
go test -bench=. -benchmem ./05_unit_testing/...
```

Output:
```
BenchmarkPlaceOrder-12    3985262    286.0 ns/op    697 B/op    8 allocs/op
                 │            │         │            │             │
                 │            │         │            │             └─ allocations/op
                 │            │         │            └─ bytes/op
                 │            │         └─ nanoseconds/op
                 │            └─ iterations
                 └─ GOMAXPROCS
```

### When to use benchmarks:
- Hot-path optimization (serialization, mapping, validation)
- Comparing two implementations (A vs B)
- Verifying a refactor didn't regress performance

> **🏭 In production:** `benchstat` for statistical comparison:
> ```bash
> go test -bench=. -count=10 > old.txt
> # ... make changes ...
> go test -bench=. -count=10 > new.txt
> benchstat old.txt new.txt
> ```

See `benchmark_test.go`.

---

## 🏆 Golden File Testing — comparing against a "golden" baseline

For complex output (JSON, YAML, generated code) instead of inline assertions
we compare against a baseline file in `testdata/`.

```go
var update = flag.Bool("update", false, "update golden files")

func TestGolden_OrderJSON(t *testing.T) {
    order := OrderJSON{ID: "ord-1", Amount: 99.99}
    actual, _ := json.MarshalIndent(order, "", "  ")

    path := filepath.Join("testdata", "order.golden")

    if *update {
        _ = os.WriteFile(path, actual, 0644)
        return
    }

    expected, _ := os.ReadFile(path)
    assert.Equal(t, string(expected), string(actual))
}
```

First run (creating baselines):
```bash
go test -run TestGolden -update
```

Subsequent runs (verification):
```bash
go test -run TestGolden
```

### When to use:
- JSON API responses (> 3-4 lines)
- Serialized protobuf/DTO
- Generated code/config

### ⚠️ Unstable fields:
`time.Now()`, UUIDs, local paths must be **normalized** before comparing against a golden file.
Otherwise the baseline will change every run.

```go
response := OrderJSON{
    ID:        "ord-NORMALIZED",          // instead of a real UUID
    CreatedAt: "2024-01-15T10:00:00Z",    // instead of time.Now()
}
```

> **🏭 In production:** golden files are committed to git.
> In a PR review you see EXACTLY what changed in the output — invaluable for API contracts.

See `golden_test.go` and `testdata/`.

---

## ▶️ Running tests

```bash
# All module tests:
go test ./05_unit_testing/... -v

# A specific case:
go test ./05_unit_testing/... -run TestPlaceOrder_TableDriven/empty

# With the race detector:
go test ./05_unit_testing/... -race

# With coverage:
go test ./05_unit_testing/... -cover
```

---

## ❌ Common mistakes

1. **A test depends on execution order** → every test creates its own data
2. **Shared state between tests** → only use `t.Parallel()` when there's no shared state
3. **Comparing error strings** → use `errors.Is` / `errors.As`, not `err.Error() == "..."`
4. **No t.Helper() in helpers** → on failure you can't see the call site
5. **One giant test** → split into table-driven with t.Run

> **🏭 In production:**
> - `go test -race` is MANDATORY in CI — it catches data races
> - `go test -cover` — track coverage, but don't chase 100%
> - Mocks for external dependencies, real implementations for integration tests

---

## 🔗 Related modules

- **← Module 01** (Interfaces) — without interfaces you can't swap dependencies
- **→ Module 06** (Integration Testing) — testing with real dependencies
- **→ Module 12** (HTTP API) — httptest for testing handlers

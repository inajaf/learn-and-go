# ⚡ Module 15: Concurrency Patterns - Concurrency for services

Go is built for competition. Goroutines + channels - the basis,
but in production you need the right patterns.

---

## 📚 What will you study

| Pattern        | File              | Why                                          |
|----------------|-------------------|----------------------------------------------|
| Worker Pool    | `worker_pool.go`  | Limited parallelism for N tasks              |
| Pipeline       | `worker_pool.go`  | Pipeline processing through channels         |
| Fan-Out/Fan-In | `fan_out.go`      | Parallel queries + aggregation               |
| errgroup       | `fan_out.go`      | Group of goroutines with shared cancellation |
| Rate Limiter   | `rate_limiter.go` | Token Bucket - RPS Limit                     |
| Semaphore      | `rate_limiter.go` | Limiting simultaneous operations             |

---

## 1️⃣ Worker Pool - controlled parallelism

```
Problem:
10_000 tasks → 10_000 goroutines → OOM / service overload

Solution:
10_000 tasks → 10 workers → controlled load
```

```
┌──────────┐     ┌─────────┐     ┌──────────┐     ┌─────────┐
│ Producer │────▶│  Jobs   │────▶│ Worker 1 │────▶│ Results │
│          │     │ Channel │     │ Worker 2 │     │ Channel │
│          │     │         │     │ Worker 3 │     │         │
└──────────┘     └─────────┘     └──────────┘     └─────────┘
```

```go
pool := NewWorkerPool(5, func(ctx context.Context, url string) (int, error) {
    resp, err := http.Get(url)
    return resp.StatusCode, err
})

results := pool.Process(ctx, urls)
```

---

## 2️⃣ Pipeline - conveyor processing

```
Generate → Filter → Transform → Collect
  []int    chan int   chan int   chan string

[1,2,3,4,5,6] → [2,4,6] → ["number_2","number_4","number_6"]
```

👉 Each stage is a separate goroutine. Data flows through channels.
In production: ETL, log processing, data pipeline.

---

## 3️⃣ Fan-Out / Fan-In with errgroup

```go
// Parallel check of the stock of 10 products
g, ctx := errgroup.WithContext(ctx)

for itemID, qty := range items {
    g.Go(func() error {
        return checker.CheckItem(ctx, itemID, qty)
    })
}

err := g.Wait() // First error → cancels all others
```

### With concurrency limitation

```go
g.SetLimit(5) // No more than 5 simultaneous requests
```

### Aggregator - data from several services

```
        ┌─── UserService.GetName()     (50ms)   ──┐
Request ├─── OrderService.CountOrders() (80ms)  ──├── Aggregate → Response
        └─── BalanceService.GetBalance() (60ms) ──┘

Serial: 190ms
Parallel: ~80ms (max of all)
```

---

## 4️⃣ Rate Limiter — Token Bucket

```
Cart: [●][●][●][ ][ ] (3 of 5 tokens)

The request has arrived → takes 1 token → [●][●][ ][ ][ ]
Replenishment → every 100ms → [●][●][●][ ][ ]

No tokens → 429 Too Many Requests
```

```go
limiter := NewTokenBucketLimiter(10, 100*time.Millisecond) // 10 req/sec

if !limiter.Allow() {
    http.Error(w, "Too Many Requests", 429)
    return
}
```

---

## 5️⃣ Semaphore - limiting simultaneous operations

```go
sem := NewSemaphore(5) // Max 5 simultaneous queries to the database

sem.Acquire(ctx) // Blocks if all 5 are busy
defer sem.Release()

db.Query(ctx, sql)
```

### Rate Limiter vs Semaphore

|         | Rate Limiter        | Semaphore               |
|---------|---------------------|-------------------------|
| Limits  | Speed ​​(RPS) | Parallelism             |
| Unit    | Requests/second     | Simultaneous operations |
| Example | 100 req/sec         | 5 DB connections        |

---

## 🧪 Running tests

```bash
go test ./15_concurrency_patterns/... -v
```

---

## ❌ Common mistakes

1. **Goroutine for each task** → OOM with 100K tasks, use worker pool
2. **sync.WaitGroup without errgroup** → manual error collection, easy to forget
3. **No context in goroutines** → impossible to cancel stuck operations
4. **Race condition** → always `go test -race ./...`
5. **Forgotten `close(channel)`** → goroutines hang forever (goroutine leak)

---

## 🔗 Related modules

- **← Module 13** (Production) - context.WithCancel to cancel goroutines
- **← Module 04** (Messaging) - event bus uses channels internally
- **→ Module 12** (HTTP API) - rate limiting middleware

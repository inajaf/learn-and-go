# 🏭 Module 13: Production Patterns - Production Code Patterns

This module is **the most important for moving from training code to real code**.
Here are the patterns that are found in EVERY production Go service.

---

## 📚 What will you study

| Topic               | File                 | Why                                             |
|---------------------|----------------------|-------------------------------------------------|
| `context.Context`   | `context.go`         | Timeouts, cancellation, request-scoped data     |
| Graceful Shutdown   | `shutdown.go`        | Correct completion without data loss            |
| Structured Logging  | `logging.go`         | `log/slog` instead of `fmt.Println`             |
| Configuration       | `config.go`          | 12-factor app, env → struct, functional options |
| Retry + Backoff     | `retry.go`           | Replays in case of temporary failures           |
| Circuit Breaker     | `circuit_breaker.go` | Protection against cascading failures           |

---

## 1️⃣ context.Context - the first parameter EVERYWHERE

### Problem without context

```
Client disconnected
       │
       ▼
HTTP Handler continues to work
       │
       ▼
Service calls the database (query for 30 seconds)
       │
       ▼
The database is executing a heavy query... in vain
```

### Solution with context

```
Client disconnected
       │
       ▼
ctx.Done() → HTTP Handler stops
       │
       ▼
ctx.Done() → Service stops running
       │
       ▼
ctx.Done() → The database cancels the request ← resources are freed!
```

### Three context tools

```go
// 1. Timeout - “do not wait longer than 2 seconds”
ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
defer cancel() // ← ALWAYS! Otherwise, goroutines leak

// 2. Manual cancellation - “cancel when the first result is found”
ctx, cancel := context.WithCancel(parentCtx)
defer cancel()

// 3. Values ​​- "forward request ID through all layers"
ctx = context.WithValue(ctx, requestIDKey, "req-abc-123")
```

### ⚠️ Rules context.Value

| ✅ Correct         | ❌ Wrong                  |
|--------------------|----------------------------|
| Request ID         | Application config         |
| Trace ID           | Connecting to the database |
| User ID (for logs) | Business parameters        |
| Correlation ID     | Passwords/secrets          |

> **🏭 In production:** each interface method begins with `ctx context.Context`.
> Without this, timeouts, tracing and graceful shutdown are impossible.

---

## 2️⃣ Graceful Shutdown - correct completion

### What happens without a graceful shutdown

```
kill process
       │
       ▼
HTTP requests are aborted (client: "connection reset")
DB transactions are not committed
Kafka messages are lost
Files are corrupted
```

### Correct termination order

```
SIGINT/SIGTERM received
       │
       ▼
1. Stop accepting NEW requests
       │
       ▼
2. Wait for the completion of CURRENT (timeout 30s)
       │
       ▼
3. Close the HTTP server
       │
       ▼
4. Close Kafka producer ← LIFO order:
       │ last registered
       ▼ the first one closes
5. Close the database pool
       │
       ▼
6. os.Exit(0)
```

```go
// Pattern in main()
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

server := NewGracefulServer(":8080", handler, logger)
server.RegisterCloseable(dbPool)
server.RegisterCloseable(kafkaProducer)

if err := server.Run(); err != nil {
logger.Error("server crashed", slog.String("error", err.Error()))
    os.Exit(1)
}
```

> **🏭 In production:** Kubernetes sends SIGTERM 30s before kill -9.
> If you don’t process it, users will receive 502.

---

## 3️⃣ Structured Logging with log/slog

### Before: fmt.Println (cannot be parsed)

```
2024-01-15 10:23:45 Order created: ord-123 for customer cust-456
```

### After: slog (machine readable JSON)

```json
{
  "time": "2024-01-15T10:23:45Z",
  "level": "INFO",
"msg": "order created",
  "order_id": "ord-123",
  "customer_id": "cust-456",
  "total": 99.99,
  "duration": "12.3ms"
}
```

### Why JSON is better

```
Elasticsearch ← parses JSON automatically
Grafana Loki ← filtering: {level="ERROR", service="order-svc"}
Kibana ← dashboards by field
AlertManager ← alerts by level=ERROR and duration > 5s
```

###Logging levels

| Level   | When to use        | Example                              |
|---------|--------------------|--------------------------------------|
| `DEBUG` | For the developer  | SQL queries, input parameters        |
| `INFO`  | Business Events    | "order created", "payment completed" |
| `WARN`  | Suspicious         | High latency, retry triggered        |
| `ERROR` | Need a reaction    | DB unavailable, payment declined     |

### Contextual Logger - logger with request ID

```go
// Middleware adds the logger to the context
ctx := WithLogger(r.Context(), logger.With(
    slog.String("request_id", reqID),
))

// Any code below gets the logger with request_id automatically
logger := LoggerFromContext(ctx)
logger.Info("operation completed") // includes request_id
```

> **🏭 In production:** slog for new projects, zerolog if needed
> maximum performance (zero-allocation).

---

## 4️⃣ Configuration - loading the config

### 12-Factor App: config from the environment

```
One Docker image → three environments:

development:  DATABASE_URL=postgres://localhost:5432/dev
staging:      DATABASE_URL=postgres://staging-db:5432/app
production:   DATABASE_URL=postgres://prod-cluster:5432/app

The code is the same, only the env is different.
```

### Pattern: struct + defaults + env + validate

```go
cfg, err := LoadConfig()
if err != nil {
log.Fatal("invalid config:", err) // ← fail fast!
}
```

### Functional Options - beautiful configuration of components

```go
// ❌ Unreadable
server := NewServer("8080", true, 25, 15*time.Second, nil)

// ✅ Functional Options
server := NewServerConfig(
    WithPort(8080),
    WithMaxConns(25),
    WithReadTimeout(30 * time.Second),
)
```

---

## 5️⃣ Retry with Exponential Backoff

### Problem without backoff

```
1000 clients
       │
       ▼
Service dropped for 5 seconds
       │
       ▼
All 1000 retrace AT THE SAME TIME → DDoS themselves!
```

### Solution: exponential backoff + jitter

```
Attempt 1: delay 100ms + random(0, 100ms)
Attempt 2: delay 200ms + random(0, 200ms)
Attempt 3: delay 400ms + random(0, 400ms)
Attempt 4: delay 800ms + random(0, 800ms)
                     ▲                ▲
              exponential          jitter
(grows x2) (spreads the load)
```

```go
err := Retry(ctx, DefaultRetryConfig(), func(ctx context.Context) error {
    return httpClient.Do(req)
})
```

---

## 6️⃣ Circuit Breaker - fuse

### State diagram

```
everything is fine N failures in a row
   ┌─────────────────┐      ┌────────────────────┐
   │     CLOSED      │─────▶│      OPEN          │
   │ (skips)         │      │ (fail fast)        │
   │                 │◀─────│                    │
   └─────────────────┘      └────────┬───────────┘
          ▲                          │
          │ trial                    │ resetTimeout
          │ request                  │ expired
          │ successful               │
   ┌──────┴──────────┐               │
   │   HALF-OPEN     │◀──────────────┘
   │ (1 trial)       │
   │                 │───▶ failure → back to OPEN
   └─────────────────┘
```

### Combination: Retry + Circuit Breaker

```go
// 👉 Retry OUTSIDE, CB INSIDE
err := Retry(ctx, retryCfg, func(ctx context.Context) error {
    return cb.Execute(ctx, func(ctx context.Context) error {
        return inventoryService.CheckStock(ctx, itemID)
    })
})
```

> **🏭 In production:** Circuit Breaker is present in every call to an external service.
> Without it, one failed service brings down the entire system (cascading failure).

---

## 🧪 Running tests

```bash
go test ./13_production_patterns/... -v
```

---

## ❌ Common mistakes

1. **Forget `defer cancel()`** after `context.WithTimeout` → goroutine leak
2. **Putting business data in `context.Value`** → not type-safe, difficult to debug
3. **`fmt.Println` instead of slog** → cannot be filtered in Grafana/ELK
4. **Retry without backoff** → DDoS your service in case of failures
5. **No Circuit Breaker** → one failed service crashes the entire system
6. **Config from files** → one image does not work in different environments

---

## 🔗 Related modules

- **← Module 01** (Interfaces) - here we add `context.Context` to the interfaces
- **← Module 08** (Communication Patterns) - retry/CB for Saga and sync calls
- **→ Module 12** (HTTP API) - middleware uses slog and context
- **→ Module 14** (Error Handling) - errors that retry should/should not repeat

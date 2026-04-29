# Module 10 - Working with a database (PostgreSQL)

## 📌 What will you study

- How to properly organize work with PostgreSQL in a microservice
- `sqlx` - a convenient wrapper over the standard `database/sql`
- SQL migrations via `golang-migrate`
- Three data layers: Domain / DB Row / DTO (repeat Module 2)
- Transactions - atomic saving of order + positions
- Soft delete - the record “disappears” but remains in the database
- Optimistic locking - protection against parallel updates
- JOIN queries - orders + positions in one SELECT
- Isolation of tests through transactions (rollback after each test)
- Docker Compose for local PostgreSQL

---

## 🏗️ Architecture

```
┌────────────────────────────────────────────────────┐
│                HTTP / gRPC                         │
│           CreateOrderRequest (DTO)                 │
└──────────────────────┬─────────────────────────────┘
                       │ mapper
                       ▼
┌────────────────────────────────────────────────────┐
│                  SERVICE                           │
│ OrderService - business logic, validation          │
│ Depends on OrderRepository (interface)             │
└──────────────────────┬─────────────────────────────┘
                       │ interface
                       ▼
┌────────────────────────────────────────────────────┐
│              REPOSITORY                            │
│ PostgresOrderRepository - SQL queries              │
│  sqlx.Get / sqlx.Select / QueryRowx                │
└──────────────────────┬─────────────────────────────┘
                       │ sql
                       ▼
┌────────────────────────────────────────────────────┐
│              PostgreSQL (Docker)                   │
│  tables: customers, orders, order_items            │
│  port: 5433                                        │
└────────────────────────────────────────────────────┘
```

---

## 🚀 Quick start

```bash
#1. Launch PostgreSQL
docker compose -f 10_database/docker-compose.yml up -d

# 2. Run tests (migrations will roll automatically)
go test ./10_database/... -v

#3. Stop and delete data
docker compose -f 10_database/docker-compose.yml down -v
```

---

## 🗄️ Database schema

```sql
customers
┌───────────────────────────────────────────────┐
│ id          TEXT PRIMARY KEY                  │
│ name        VARCHAR(255) NOT NULL             │
│ email       VARCHAR(255) NOT NULL UNIQUE      │
│ created_at  TIMESTAMPTZ DEFAULT NOW()         │
│ updated_at  TIMESTAMPTZ DEFAULT NOW()         │
└───────────────────────────────────────────────┘
                    │ 1
                    │
                    │ N
orders
┌───────────────────────────────────────────────┐
│ id           TEXT PRIMARY KEY                 │
│ customer_id  TEXT FK → customers.id           │
│ status       VARCHAR(50) pending/confirmed/cancelled │
│ total_amount NUMERIC(12,2)                    │
│ notes        TEXT (nullable)                  │
│ deleted_at TIMESTAMPTZ (NULL = not deleted) │ ← Soft Delete
│ version      INT DEFAULT 1                    │ ← Optimistic Lock
│ created_at   TIMESTAMPTZ                      │
│ updated_at   TIMESTAMPTZ                      │
└───────────────────────────────────────────────┘
                    │ 1
                    │
                    │ N
order_items
┌───────────────────────────────────────────────┐
│ id          TEXT PRIMARY KEY                  │
│ order_id    TEXT FK → orders.id (CASCADE)     │
│ product_id  VARCHAR(100)                      │
│ name        VARCHAR(255)                      │
│ quantity    INT CHECK > 0                     │
│ unit_price  NUMERIC(12,2)                     │
│ created_at  TIMESTAMPTZ                       │
└───────────────────────────────────────────────┘
```

---

## 📦 Dependencies

```
github.com/jmoiron/sqlx - convenient SELECT/Get into structures
github.com/lib/pq - PostgreSQL driver
github.com/golang-migrate/v4 - SQL migrations
```

---

## 🔑 Key techniques

### 1. sqlx.Get - get one row into the structure

```go
var row orderRow
err := db.Get(&row,
    `SELECT id, status, total_amount FROM orders WHERE id = $1`,
    id,
)
// db tags: in the structure → sqlx automatically maps columns
```

### 2. sqlx.Select - get several rows into a slice

```go
var rows []orderRow
err := db.Select(&rows,
    `SELECT id, status FROM orders WHERE customer_id = $1`,
    customerID,
)
```

### 3. Transaction - atomic INSERT of several tables

```go
tx, err := db.Beginx()
defer func() {
    if err != nil { tx.Rollback() }
}()

tx.QueryRowx(`INSERT INTO orders ...`).Scan(...)
tx.QueryRowx(`INSERT INTO order_items ...`).Scan(...)

return tx.Commit()
```

### 4. Soft Delete - we do not physically delete

```go
// Delete:
UPDATE orders SET deleted_at = NOW() WHERE id = $1

// Filtering (in all SELECTs):
WHERE deleted_at IS NULL
```

### 5. Optimistic Locking - protection against parallel updates

```go
result, _ := db.Exec(
    `UPDATE orders
     SET status = $1, version = version + 1
WHERE id = $2 AND version = $3`, // ← key condition
    newStatus, id, currentVersion,
)
if affected == 0 {
return ErrVersionConflict // someone has already updated!
}
```

### 6. JOIN + row reversal

```go
// One SELECT returns N rows per order (one row for each position)
SELECT o.id, o.status, oi.id as item_id, oi.name
FROM orders o
LEFT JOIN order_items oi ON oi.order_id = o.id

// In Go - collect in map[orderID]*Order, add Items
for _, row := range rows {
    order := ordersMap[row.OrderID]
    order.Items = append(order.Items, db.OrderItem{...})
}
```

### 7. Transactional test isolation

```go
// SetupTest - start the transaction
tx, _ := db.Beginx()
repo = NewPostgresOrderRepository(tx) // repo works through a transaction

// TearDownTest - rollback ALL test changes
tx.Rollback() // next test starts with a clean database!
```

---

## 🏊 Connection Pool Tuning

`database/sql` supports connection pooling automatically, but **default options
not suitable for production**. Set them up explicitly.

```go
db, _ := sqlx.Connect("postgres", dsn)

db.SetMaxOpenConns(25) // maximum simultaneous connections
db.SetMaxIdleConns(10) // keep them ready in the pool
db.SetConnMaxLifetime(5 * time.Minute) // close the connection after 5 minutes
db.SetConnMaxIdleTime(1 * time.Minute) // close idle time after 1 minute
```

### What does each parameter mean?

| Parameter          | What does                     | How to choose                                                              |
|--------------------|-------------------------------|----------------------------------------------------------------------------|
| `MaxOpenConns`     | Upper connection limit        | By `max_connections` in Postgres divided by the number of service replicas |
| `MaxIdleConns`     | How long to keep ready        | ~30-50% of `MaxOpenConns`                                                  |
| `ConnMaxLifetime`  | TTL connections               | 5-15 min - for LB balancer / IP rotation                                   |
| `ConnMaxIdleTime`  | How long to live without work | 1-5 min - saving connections to the database                               |

### How to choose MaxOpenConns

**Important:** do not consider “how much the service needs”, but “how much the database can handle”.

```
postgres max_connections = 100
service replicas = 4
reserved for admin/migrations = 20

MaxOpenConns = (100 - 20) / 4 = 20
```

If your service scales horizontally, don’t forget to recalculate.
**Typical error:** `MaxOpenConns=100` for 10 replicas → 1000 connections, the database dies.

### Why have limits at all?

Without setting:
- `MaxOpenConns` by default = **unlimited** → when the load surges, you can put the database
- `MaxIdleConns` = 2 → with high RPS we constantly open/close connections
- `ConnMaxLifetime` = **no timeout** → connections live forever, LB cannot rotate them

---

## 🩺 Health Checks — Liveness vs Readiness

The microservice needs **two different** health checks for Kubernetes/orchestrator:

```
┌────────────────────┐   GET /healthz   ┌──────────────────────────┐
│ Kubelet            │ ───────────────► │ Liveness                 │ “is the process alive?”
│                    └──────────────────┘ ↓ if not → kill & restart│
│                    │
│                    │   GET /ready     ┌───────────────────────────┐
│                    │ ───────────────► │ Readiness                 │ "ready to receive traffic?"
└────────────────────┘                  │ ↓ if not → exclude from LB│
```

|                | Liveness                   | Readiness                                     |
|----------------|----------------------------|-----------------------------------------------|
| **Question**   | Is the process alive?      | Ready to process requests?                    |
| **Checks**     | Only the process itself    | Process + all dependencies (DB, Kafka, cache) |
| **On failure** | Kubernetes kills pods      | Kubernetes removes pods from balancer         |
| **Frequency**  | Rarely (once every 10-30s) | More often (every 5-10s)                      |

### Why can't they be combined?

```go
// ❌ BAD: liveness checks the database
func Liveness(w http.ResponseWriter, r *http.Request) {
    if err := db.Ping(); err != nil {
w.WriteHeader(503) // DB blinked → pod killed
        return
    }
}
```

The database blinked for 2 seconds - **all** pods get 503 for liveness → Kubernetes kills
all pods at once → the service is completely down. This makes the problem worse rather than solving it.

### Correct

```go
// Liveness - checks ONLY the process itself
func Liveness(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(200) // if the process is running, it is alive
}

// Readiness - checks dependencies
func Readiness(hc *HealthChecker) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        status := hc.Check(r.Context())
        if !status.Healthy {
            w.WriteHeader(503)
            json.NewEncoder(w).Encode(status)
            return
        }
        w.WriteHeader(200)
    }
}
```

### Key nuances of HealthChecker

```go
func (hc *HealthChecker) Check(ctx context.Context) HealthStatus {
ctx, cancel := context.WithTimeout(ctx, hc.timeout) // 👈 ALWAYS timeout
    defer cancel()

    start := time.Now()
    err := hc.db.PingContext(ctx)
// ... fix latency, return status
}
```

**Timeout is required.** Without it, health-check will hang, kubelet will receive “not responding”
and will kill pod. With a timeout you will see "unhealthy" and pass the real error outside.

See `repository/health.go` for a complete implementation with the `Pinger` interface.

> **🏭 In production:** there is also a separate `/startup` probe - for a slow start
> (warming up caches, loading configs). Until it succeeds, Kubernetes does not check liveness/readiness.

---

## 📋 SQL migrations

Migrations are versioned SQL scripts.
Each migration has a version and two files: `up` and `down`.

```
migrations/
000001_init.up.sql ← create tables
000001_init.down.sql ← delete tables
000002_add_column.up.sql ← add a field
000002_add_column.down.sql ← delete the field
```

```go
// Apply all migrations:
m.Up()

// Roll back the last one:
m.Steps(-1)

// Roll back everything:
m.Down()
```

> **Rule:** never edit an applied migration.
> Create a new migration for changes.

---

## 🔄 Communication with other modules

```
Module 1 (interfaces) ← OrderRepository - interface
Module 2 (dto) ← orderRow = Persistence layer
Module 6 (suite) ← SetupTest/TearDownTest + transactions
Module 8 (patterns) ← service via interface (DI)
```

---

## 📁 Module files

| File                                       | What does                                          |
|--------------------------------------------|----------------------------------------------------|
| `docker-compose.yml`                       | PostgreSQL 15 on port 5433                         |
| `migrations/000001_init.up.sql`            | Create tables + indexes                            |
| `migrations/000001_init.down.sql`          | Removing tables                                    |
| `domain.go`                                | Domain models + repository interfaces              |
| `repository/postgres.go`                   | SQL implementation via sqlx                        |
| `repository/postgres_integration_test.go`  | Integration tests with real PG                     |
| `repository/health.go` ⭐                   | HealthChecker + PoolConfig + Liveness/Readiness   |
| `repository/health_test.go` ⭐              | Health check tests with mock pinger               |
| `service/service.go`                       | Business logic + DTO mapping                       |

---

## ▶️ Launch

```bash
# Start Postgres:
docker compose -f 10_database/docker-compose.yml up -d

# Tests (migrations will be rolled out automatically):
go test ./10_database/... -v

# View tables:
docker exec -it learning_path_postgres psql -U orders_user -d orders_db

# Inside psql:
\dt -- list of tables
\d orders -- orders table structure
SELECT * FROM customers;
SELECT * FROM orders;
SELECT * FROM order_items;
```

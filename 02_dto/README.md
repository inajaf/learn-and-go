# Module 2 — DTOs and data layers

## 📌 What you'll learn in this module

- Why you should split data into layers (Transport / Domain / Persistence)
- How to do mapping between layers correctly
- Partial Update — updating only the fields that were provided
- Validation at the DTO level vs the Domain level
- Why you can't "share" one struct everywhere
- Senior developer best practices

---

## ❓ Why split data into layers?

In a real application the same data looks different at different levels:

```
HTTP request         →  Business logic     →  Database
CreateOrderRequest      Order (domain)        OrderRow
{                       {                     {
  customer_id: "x"      ID: "ord-123"         id: "ord-123"
  amount: 100           CustomerID: "x"       customer_id: "x"
}                       Amount: 100           amount: 100
                        Status: pending       status: "pending"
                        CreatedAt: ...        created_at: ...
                        UpdatedAt: ...        updated_at: ...
                    }                         deleted_at: NULL
                                              version: 1
                                          }
```

### If you use one struct for everything — problems:

```go
// ❌ One struct for everything — BAD:
type Order struct {
    ID         string     `json:"id" db:"id"`
    CustomerID string     `json:"customer_id" db:"customer_id"`
    Amount     float64    `json:"amount" db:"amount"`
    Status     string     `json:"status" db:"status"`
    DeletedAt  *time.Time `json:"deleted_at" db:"deleted_at"` // internal field!
    CostPrice  float64    `json:"cost_price" db:"cost_price"` // secret field!
    Version    int        `json:"version" db:"version"`       // a DB technical artifact
}
```

**Problems:**
1. `deleted_at` and `cost_price` leak into the API response — **internal data leak!**
2. You changed the API (renamed a field) → you broke the DB schema
3. The client can send `version` → **vulnerability!**
4. Domain logic is mixed with HTTP logic and DB logic

---

## 🏗️ Three data layers

```
┌─────────────────────────────────────────────────────┐
│                  HTTP / gRPC                        │
│  CreateOrderRequest  ←→  OrderResponse              │
│        (json tags)                                  │
└─────────────────────┬───────────────────────────────┘
                      │  mapper.RequestToDomain()
                      ▼
┌─────────────────────────────────────────────────────┐
│                  DOMAIN                             │
│               OrderDomain                           │
│           (no tags, business only)                  │
└─────────────────────┬───────────────────────────────┘
                      │  mapper.DomainToDB()
                      ▼
┌─────────────────────────────────────────────────────┐
│                  DATABASE                           │
│                  OrderRow                           │
│             (db tags, soft delete, version)         │
└─────────────────────────────────────────────────────┘
```

| Layer           | Struct                                  | Where it lives | Tags | Who sees it               |
|-----------------|-----------------------------------------|----------------|------|---------------------------|
| **Transport**   | `CreateOrderRequest`, `OrderResponse`   | HTTP handler | `json:` | API client                |
| **Domain**      | `OrderDomain`                           | Service | no tags | Only service code         |
| **Persistence** | `OrderRow`                              | Repository | `db:` | Only the repository       |

---

## 🔄 Mapping between layers

```
                    Input
                      │
CreateOrderRequest ───┤
                      │  RequestToDomain()
                      ▼
                 OrderDomain  ──── DomainToResponse() ──▶ OrderResponse (Output)
                      │
                      │  DomainToDB()
                      ▼
                   OrderRow  ─────────────────────────▶ DB INSERT/UPDATE
                      │
                      │  DBToDomain()
                      ▼
                 OrderDomain  ←─────────────────────── DB SELECT
```

---

## ✏️ Partial Update Pattern

When the client wants to update only some of the fields — use a pointer:

```go
type UpdateOrderRequest struct {
    Amount *float64 `json:"amount,omitempty"`  // pointer — field is optional
    // If nil — the client did not pass the field → don't update
    // If not nil — the client sent a value → update
}

func ApplyUpdateRequest(order *OrderDomain, req UpdateOrderRequest) {
    if req.Amount != nil {
        order.Amount = *req.Amount  // dereference the pointer
    }
    order.UpdatedAt = time.Now()  // always refresh the timestamp
}
```

Why is this better than just `Amount float64`?

```go
// The problem with float64 (no pointer):
req := UpdateOrderRequest{Amount: 0}
// Amount == 0 — did the client want to zero it? Or just didn't send the field?
// Unknown!

// With a pointer it's unambiguous:
req1 := UpdateOrderRequest{Amount: nil}    // field not passed
req2 := UpdateOrderRequest{Amount: &zero}  // explicitly zeroed
```

---

## ✅ Validation: DTO vs Domain

There are two validation levels and both are needed:

```go
// Level 1: DTO validation (syntax)
// Check that data arrived in the correct format
func (req CreateOrderRequest) Validate() error {
    if req.CustomerID == "" {
        return errors.New("customer_id is required")
    }
    if req.Amount <= 0 {
        return fmt.Errorf("amount must be positive, got %.2f", req.Amount)
    }
    return nil
}

// Level 2: Domain validation (business rules)
// Check business invariants
func (o *OrderDomain) Validate() error {
    if o.Amount > 1_000_000 {
        return errors.New("order amount exceeds maximum limit")
    }
    if len(o.CustomerID) < 3 {
        return errors.New("invalid customer ID format")
    }
    return nil
}
```

---

## 💡 Why this is critical in microservices

In microservices, services communicate via gRPC or REST.
Each service has its own "truth" about the same entity:

```
OrderService                    InventoryService
{                               {
  order_id: "123"                 order_id: "123"
  customer_id: "456"              items: [...]
  total_amount: 100               warehouse_id: "wh-1"
  status: "confirmed"           }
  payment_method: "card"
}
```

A DTO is a **public contract between services**. It must be:
- **Versioned** (`/api/v1/orders`, `/api/v2/orders`)
- **Backward compatible** (you can't delete fields)
- **Minimal** (only what the consumer needs)

---

## 🏆 Senior developer's top rules

> **1. Never return a domain model directly in an HTTP response.**
> Always map through a DTO. Otherwise you risk leaking internal fields.

> **2. Never accept ID, CreatedAt, Status from the client on create.**
> These fields are generated by the server. The client must not set them.

> **3. Use pointer fields for partial updates.**
> `*float64` ≠ `float64` — it's a semantic "passed/not passed" distinction.

> **4. Validate DTOs on input, business rules in the domain.**
> Two layers of defense are better than one.

> **5. Format time as a string in DTOs, use time.Time in the domain.**
> Clients prefer strings; inside your code — time.Time.

---

## 📁 Module files

| File             | What it does                                                  |
|------------------|---------------------------------------------------------------|
| `models.go`      | Three layers: `CreateOrderRequest`, `OrderDomain`, `OrderRow` |
| `mapper.go`      | Conversion functions between layers                           |
| `mapper_test.go` | Mapping and round-trip tests                                  |

---

## ▶️ How to run

```bash
go test ./02_dto/... -v
```

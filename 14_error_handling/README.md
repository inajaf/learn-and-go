# ⚠️ Module 14: Error Handling - Error Handling Patterns

In Go, errors are **values**, not exceptions. This gives you complete control
but requires the right patterns.

---

## 📚 What will you study

| Level | Pattern | When to use |
|---------|---------|-------------------|
| Junior | Sentinel errors | Simple facts: "not found", "invalid" |
| Middle | Wrapping (`%w`) | Adding context when forwarding |
| Senior | Custom types | Data errors: which resource, which field |
| Senior+ | Behavioral errors | Checking through interfaces, without binding to type |

---

## 1️⃣ Sentinel Errors - named errors

```go
var ErrNotFound = errors.New("not found")

// Examination:
if errors.Is(err, ErrNotFound) { ... }
```

**When:** the error does not carry data, only a fact.

---

## 2️⃣ Wrapping - adding context

```
❌ return err
→ "sql: no rows" - WHERE???

✅ return fmt.Errorf("search for order %s: %w", id, err)
→ "order search ord-123: sql: no rows" - CLEAR!
```

```
Repository:  sql: no rows in result set
     ↓ %w
Service: order search ord-123: sql: no rows in result set
     ↓ %w
Handler: GET /orders/ord-123: order search ord-123: sql: no rows
```

> **Important:** `%w` wraps (errors.Is works), `%v` does NOT wrap (the chain is lost).

---

## 3️⃣ Custom Error Types - errors with data

```go
type NotFoundError struct {
    Resource string  // "order", "customer"
    ID       string  // "ord-123"
}

func (e *NotFoundError) Error() string { ... }
func (e *NotFoundError) Is(target error) bool {
return target == ErrNotFound // compatible with sentinel
}
```

### Checking through errors.As:

```go
var nfe *NotFoundError
if errors.As(err, &nfe) {
// Data access: nfe.Resource, nfe.ID
log.Printf("resource %s with ID %s not found", nfe.Resource, nfe.ID)
}
```

### ValidationError - collection of ALL errors:

```go
ve := &ValidationError{}
ve.Add("email", "cannot be empty")
ve.Add("price", "must be > 0")

if ve.HasErrors() {
return ve // ​​The client will receive ALL errors, not just one
}
```

---

## 4️⃣ Behavioral Errors - checking through the interface

```
Problem:
errors.As(err, &NotFoundError{}) ← binding to a specific type
Package B must import package A

Solution:
IsNotFound(err) ← checks interface { NotFound() bool }
No dependency on a specific type
```

```go
// Any error can be "not found" if it implements the interface
type NotFoundChecker interface {
    NotFound() bool
}

func IsNotFound(err error) bool {
    var nf NotFoundChecker
    if errors.As(err, &nf) {
        return nf.NotFound()
    }
    return errors.Is(err, ErrNotFound)
}
```

---

## 5️⃣ errors.Join (Go 1.20+) - multiple errors

```go
var errs []error
if name == "" { errs = append(errs, fmt.Errorf("name: required")) }
if age < 0 { errs = append(errs, fmt.Errorf("age: must be >= 0")) }

return errors.Join(errs...) // nil if there are no errors
```

---

## 6️⃣ Error Mapping — Domain → HTTP

```go
func HTTPStatusCode(err error) int {
    switch {
    case errors.Is(err, ErrNotFound):    return 404
    case errors.Is(err, ErrInvalidInput): return 400
    case errors.Is(err, ErrConflict):     return 409
    default:                              return 500
    }
}
```

```
Domain Layer:   NewNotFoundError("order", "ord-123")
                        ↓
Service Layer:  NewOperationError("GetOrder", "order", "ord-123", err)
                        ↓
HTTP Handler:   HTTPStatusCode(err) → 404
                        ↓
Client: {"code": 404, "message": "order with ID ord-123 not found"}
```

---

## 🧪 Running tests

```bash
go test ./14_error_handling/... -v
```

---

## ❌ Common mistakes

1. **`err.Error() == "not found"` instead of `errors.Is`** → will break when wrapping
2. **`err.(*NotFoundError)` instead of `errors.As`** → not visible via Unwrap
3. **`%v` instead of `%w`** → the chain of errors is lost
4. **`return err` without context** → "sql: no rows" instead of "GetOrder(ord-123): sql: no rows"
5. **One validation error** instead of collecting all → client corrects one at a time

---

## 🔗 Related modules

- **← Module 01** (Interfaces) - `errors.go` with sentinel errors
- **← Module 13** (Production) — retry with `IsTemporary(err)`
- **→ Module 12** (HTTP API) - mapping errors to HTTP statuses
- **→ Module 10** (Database) — ConflictError with optimistic locking

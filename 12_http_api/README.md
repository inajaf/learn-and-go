# 🌐 Module 12: HTTP API - REST server on net/http

The course refers to REST/HTTP in every module, but nowhere shows how to accept
HTTP requests. This module closes the gap.

---

## 📚 What will you study

| Topic              | File               | Why                                                     |
|--------------------|--------------------|---------------------------------------------------------|
| Routing (Go 1.22+) | `server.go`        | `GET /orders/{id}` — path params in stdlib              |
| Handlers           | `handlers.go`      | Pattern: decode → validate → service → encode           |
| Middleware         | `middleware.go`    | Request ID, logging, recovery, CORS, auth               |
| API errors         | `server.go`        | JSON errors instead of text errors, mapping domain→HTTP |
| Testing            | `handlers_test.go` | `httptest.NewRecorder` + `httptest.NewServer`           |

---

## 🏗️ Architecture

```
HTTP Request
       │
       ▼
┌───────────────────────┐
│    Middleware Chain   │
│  ┌─────────────────┐  │
│  │ 1. Request ID   │  │ ← generates/forwards X-Request-ID
│  │ 2. Logging      │  │ ← logs method, path, status, duration
│  │ 3. Recovery     │  │ ← panic → 500 (does not crash the server)
│  └─────────────────┘  │
└──────────┬────────────┘
           │
           ▼
┌─────────────────────┐
│     Handler         │
│  1. Decode (JSON)   │  ← json.NewDecoder + DisallowUnknownFields
│ 2. Validate (DTO)   │ ← collects ALL errors
│ 3. Call Service     │ ← via interface, with ctx
│  4. Encode (JSON)   │  ← writeJSON / writeError
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ OrderService (i/f)  │ ← Handler does not know the implementation
│ (Module 01)         │
└─────────────────────┘
```

---

## 1️⃣ Routing - Go 1.22+ Pattern Routing

```go
mux := http.NewServeMux()

// 👉 METHOD /path - method right in the pattern!
mux.HandleFunc("POST /api/v1/orders", h.CreateOrder)
mux.HandleFunc("GET /api/v1/orders/{id}", h.GetOrder)
mux.HandleFunc("GET /api/v1/orders", h.ListOrders)
mux.HandleFunc("POST /api/v1/orders/{id}/cancel", h.CancelOrder)
```

```go
// Path parameter:
id := r.PathValue("id")  // Go 1.22+
```

### Before Go 1.22 vs After

|                | Up to 1.22 (chi/gorilla)   | After 1.22 (stdlib)      |
|----------------|----------------------------|--------------------------|
| Path params    | `chi.URLParam(r, "id")`    | `r.PathValue("id")`      |
| Method routing | `r.Get("/path", h)`        | `"GET /path"` in pattern |
| Dependencies   | External library           | Standard Library         |

> **🏭 In production:** stdlib is enough for 90% of APIs. chi/gin are needed if:
> - We need route groups with middleware
> - Need an OpenAPI generator
> - Very complex routing

---

## 2️⃣ Handler - pattern decode → validate → service → encode

```go
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    // 1. Decode
    var req CreateOrderRequest
    if err := decodeJSON(r, &req); err != nil {
writeError(w, &APIError{Code: 400, Message: "invalid JSON"})
        return
    }

    // 2. Validate
    if err := req.Validate(); err != nil { ... }

// 3. Call Service (with context!)
    order, err := h.service.CreateOrder(r.Context(), req)

    // 4. Encode
    writeJSON(w, http.StatusCreated, order)
}
```

### Handler as struct, not closure

```go
// ✅ Struct - easy to test
type OrderHandler struct {
service OrderService // interface
    logger  *slog.Logger
}

// ❌ Closure - dependencies are closed, difficult to replace
func makeHandler(svc OrderService) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) { ... }
}
```

---

## 3️⃣ Middleware - intermediate processors

### Middleware pattern in Go

```go
func MyMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// BEFORE handler
            next.ServeHTTP(w, r)
// AFTER handler
        })
    }
}
```

### Middleware chain

```go
var handler http.Handler = mux
handler = RecoveryMiddleware(logger)(handler)      // 3
handler = RequestLoggingMiddleware(logger)(handler) // 2
handler = RequestIDMiddleware()(handler)            // 1
```

```
Request → RequestID(1) → Logging(2) → Recovery(3) → Handler → Reply
```

### Recovery - why catch panic

```
❌ Without Recovery:
panic in one handler → ENTIRE server crashes → ALL clients disconnected

✅ With Recovery:
panic in one handler → 500 to this client → the rest work
+ stack trace logged for debugging
```

---

## 4️⃣ JSON errors are always structured

```
❌ Text error:
   404 Not Found

✅ JSON error:
   {
     "error": {
       "code": 404,
"message": "order not found",
"details": ["order with ID ord-123 does not exist"]
     }
   }
```

```go
// Domain errors don't know about HTTP
var ErrOrderNotFound = errors.New("order not found")

// Mapping at the transport level
func MapDomainError(err error) *APIError {
    switch {
    case errors.Is(err, ErrOrderNotFound): return &APIError{Code: 404, ...}
    case errors.Is(err, ErrInvalidInput):  return &APIError{Code: 400, ...}
    default:                               return &APIError{Code: 500, ...}
    }
}
```

---

## 5️⃣ HTTP Testing

### httptest.NewRecorder - unit test of the handler

```go
req := httptest.NewRequest("GET", "/api/v1/orders/123", nil)
rec := httptest.NewRecorder()

router.ServeHTTP(rec, req)

assert.Equal(t, http.StatusOK, rec.Code)
```

### httptest.NewServer - integration test

```go
server := httptest.NewServer(router)
defer server.Close()

resp, err := http.Get(server.URL + "/api/v1/orders")
```

---

## 🧪 Running tests

```bash
go test ./12_http_api/... -v
```

---

## ❌ Common mistakes

1. **`ioutil.ReadAll` for JSON** → use `json.NewDecoder` (streams, does not load everything into memory)
2. **No `MaxBytesReader`** → client can send 10GB and put the service
3. **Text errors** instead of JSON → frontend cannot parse
4. **Domain errors in HTTP response** → “sql: no rows” instead of “order not found”
5. **No Recovery middleware** → one panic crashes the entire server
6. **No Request ID** → it is impossible to track the request through logs

---

## 🔗 Related modules

- **← Module 01** (Interfaces) - `OrderService` - interface for DI
- **← Module 02** (DTO) - `CreateOrderRequest`/`OrderResponse` - data layers
- **← Module 13** (Production) — context, slog, graceful shutdown
- **← Module 14** (Errors) - mapping domain → HTTP errors
- **→ Module 05** (Unit Testing) - httptest pattern

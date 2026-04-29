# Module 3 - gRPC

## 📌 What will you learn in this module

- What are gRPC and Protobuf and why are they needed?
- gRPC vs REST - when to choose what
- 4 types of gRPC calls (Unary, Server/Client/Bidirectional Streaming)
- gRPC error codes - analogous to HTTP statuses
- Testing gRPC via bufconn (without a real server)
- Metadata and Interceptors - middleware for gRPC
- Best practices of a senior developer

---

## ❓ What is gRPC?

gRPC = Google Remote Procedure Call.

This is a framework for calling functions on a remote server.
Instead of “you send an HTTP request and parse JSON” -
"you call a function and get a Go structure."

```
Without gRPC (REST):                With gRPC:
──────────────────────────────      ─────────────────────────────
http.Post("/orders", jsonBody)  vs  client.CreateOrder(ctx, req)
↓ marshal JSON ↓ Protobuf encode (binary)
↓ HTTP POST                         ↓ HTTP/2
↓ unmarshal JSON                    ↓ Protobuf decode
↓ check the fields manually ↓ ready Go structure
```

---

## 📊 gRPC vs REST - detailed comparison

| Criterion           | REST/HTTP                  | gRPC                                   |
|---------------------|----------------------------|----------------------------------------|
| **Data Format**     | JSON (text)                | Protobuf (binary, ~3-10x more compact) |
| **Speed**           | Slower                     | 5-10x faster                           |
| **Typification**    | Weak (JSON)                | Strict (schema-first)                  |
| **Scheme**          | OpenAPI (optional!)        | `.proto` (required)                    |
| **Streaming**       | Complex (WebSocket, SSE)   | Native support                         |
| **Browser support** | Excellent                  | Bad (needs grpc-web proxy)             |
| **Readability**     | We read JSON with our eyes | Binary, need a tool                    |
| **Versioning**      | URL (`/v1/`, `/v2/`)       | Via `reserved` in proto                |
| **Code Generation** | Optional                   | Required (protoc)                      |
| **When to use**     | Public API, browser        | Internal services                      |

---

## 🤝 When to use gRPC vs REST

```
PUBLIC API (browser, mobile)
         │
         ▼
    REST / HTTP
(JSON, readable, compatible)

INTERNAL MICROSERVICES
         │
         ▼
    gRPC + Protobuf
(fast, typed, reliable)
```

### Rule of thumb:
- **REST** — if the consumer is a browser or a third-party developer
- **gRPC** - if this is an internal service and performance is needed
- **Messaging (Kafka/RabbitMQ)** - if asynchrony is needed (see Module 4 and 8)

---

## 📄 .proto file - contract between services

```protobuf
syntax = "proto3";

service OrderService {
// Unary RPC - one request, one response (like regular HTTP)
  rpc CreateOrder(CreateOrderRequest) returns (OrderResponse);

// Server-Side Streaming - one request, stream of responses
  rpc ListOrders(ListOrdersRequest) returns (stream OrderResponse);

// Client-Side Streaming - stream of requests, one response
  rpc BulkCreateOrders(stream CreateOrderRequest) returns (BulkCreateResponse);

// Bidirectional Streaming - back and forth flow (chat, real-time)
  rpc OrderChat(stream ChatMessage) returns (stream ChatMessage);
}
```

### 4 types of gRPC calls:

```
1. Unary (regular):
   Client ──request──▶ Server ──response──▶ Client

2. Server Streaming (pagination, large lists):
   Client ──request──▶ Server ──response1──▶
                              ──response2──▶
                              ──response3──▶ Client

3. Client Streaming (file download, batch insert):
   Client ──request1──▶
          ──request2──▶
          ──request3──▶ Server ──response──▶ Client

4. Bidirectional Streaming (chat, real-time):
   Client ──msg──▶ Server ──msg──▶ Client
          ◀──msg── Server ◀──msg──
```

---

## 🚦 gRPC error codes

gRPC has its own codes - not HTTP statuses, but similar:

| gRPC code                 | Similar to HTTP | When to use         |
|---------------------------|-----------------|---------------------|
| `codes.OK`                | 200             | Success             |
| `codes.InvalidArgument`   | 400             | Invalid parameters  |
| `codes.NotFound`          | 404             | Resource not found  |
| `codes.AlreadyExists`     | 409             | Already exists      |
| `codes.PermissionDenied`  | 403             | No access           |
| `codes.Unauthenticated`   | 401             | Not authorized      |
| `codes.ResourceExhausted` | 429             | Limit exceeded      |
| `codes.Internal`          | 500             | Internal error      |
| `codes.Unavailable`       | 503             | Service unavailable |
| `codes.DeadlineExceeded`  | 504             | Timeout             |

```go
// How to return errors in gRPC:
if req.GetCustomerId() == "" {
    return nil, status.Error(codes.InvalidArgument, "customer_id is required")
}

// How to check for errors on the client side:
_, err := client.CreateOrder(ctx, req)
if st, ok := status.FromError(err); ok {
    switch st.Code() {
    case codes.NotFound:
fmt.Println("order not found")
    case codes.InvalidArgument:
fmt.Println("invalid parameters:", st.Message())
    }
}
```

---

## 🔧 Interceptors - middleware for gRPC

Interceptor is middleware for gRPC. Analogous to HTTP middleware.
Added once per server/client and applied to all calls:

```go
// Example: logging interceptor on the server
func loggingInterceptor(
    ctx context.Context,
    req interface{},
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (interface{}, error) {
    start := time.Now()
resp, err := handler(ctx, req) // call the real handler
    log.Printf("method=%s duration=%s err=%v",
        info.FullMethod, time.Since(start), err)
    return resp, err
}

// Register on the server:
grpc.NewServer(
    grpc.UnaryInterceptor(loggingInterceptor),
    grpc.UnaryInterceptor(authInterceptor),
    grpc.UnaryInterceptor(metricsInterceptor),
)
```

---

## 🧪 gRPC testing via bufconn

`bufconn` - in-memory "pipe". The server and client connect without a real network port.

```go
func setupTestServer(t *testing.T) orderpb.OrderServiceClient {
// Create an in-memory listener
    lis := bufconn.Listen(1024 * 1024)

// Start the gRPC server
    srv := grpc.NewServer()
    orderpb.RegisterOrderServiceServer(srv, NewOrderServer())
    go srv.Serve(lis)

// Connect the client via the same bufconn
    conn, _ := grpc.DialContext(ctx, "bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.Dial()
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    return orderpb.NewOrderServiceClient(conn)
}
```

---

## 📁 Module files

| File                    | What does                                                |
|-------------------------|----------------------------------------------------------|
| `proto/order.proto`     | Service schema - contract between services               |
| `gen/`                  | Generated Go code (do not edit!)                         |
| `server/server.go`      | Implementation of gRPC server (Unary + Server Streaming) |
| `server/server_test.go` | Test via in-memory bufconn                               |

---

## 🏆 Best Practices gRPC

> **1. Always inline `Unimplemented*Server`.**
> Protection from broken builds when adding new methods to proto.

> **2. Use `context` everywhere - it contains deadline and cancellation.**
> `ctx.Done()` - signal that the client has disconnected. Stop working.

> **3. Return correct gRPC codes, do not write lines.**
> `codes.NotFound` is not `fmt.Errorf("not found")`.

> **4. Version proto files - `reserved` for deleted fields.**
> Old clients should not break when upgrading the server.

> **5. Never edit `gen/` - this is generated code.**
> Regenerate via `protoc` when `.proto` changes.

---

## ▶️ Launch

```bash
go test ./03_grpc/... -v
```

> The generated code is already in the `gen/` folder - protoc is not needed.

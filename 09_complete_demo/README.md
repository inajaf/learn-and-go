# Module 9 - Full Demo: Mini Microservices System

## 📌 What will you study

- How **everything works together**: interfaces + DTO + messaging + tests
- Complete architecture of a mini-system of 3 services
- How to start the system and see how services communicate
- Trace request from API to events

---

## 🏗️ System architecture

```
                     ┌──────────────┐
                     │ HTTP API     │ ← entry point (main.go)
                     │ (REST JSON)  │
                     └──────┬───────┘
                            │
                            ▼
         ┌──────────────────────────────────────┐
         │           ORDER SERVICE              │
         │ - Accepts HTTP requests              │
         │ - Validates DTO                      │
         │ - Executes business logic            │
         │ - Saves via Repository               │
         │ - Publishes events                   │
         └──────────┬──────────────┬────────────┘
                    │              │
         Sync gRPC  │              │ Async Event
         (check)    │              │ (order.created)
                    ▼              ▼
         ┌──────────────┐  ┌──────────────────────────┐
         │  INVENTORY   │  │       EVENT BUS          │
         │   SERVICE    │  │  (Kafka in production)   │
         │              │  └────────┬───────────┬─────┘
         │ Checks       │           │           │
         │ availability │           ▼           ▼
         └──────────────┘  ┌─────────────┐ ┌───────────┐
                           │NOTIFICATION │ │ANALYTICS  │
                           │  SERVICE    │ │ SERVICE   │
                           │ (email sim) │ │(tracking) │
                           └─────────────┘ └───────────┘
```

---

## 🚀 How to launch

```bash
# Launch demo application:
go run ./09_complete_demo/cmd/main.go

# Or running tests:
go test ./09_complete_demo/... -v
```

---

## 📋 What happens at startup

```
1. Initialize all services (DI manually)
2. Start the system
3. Create an order → see the whole path:
- DTO Validation
- Warehouse check (synchronously)
- Saving to repository
- Event publication
- Notification (async)
   - Analytics tracking (async)
4. We receive the order
5. Cancel the order
6. We display final statistics
```

---

## 📁 Module files

| File           | What does                                   |
|----------------|---------------------------------------------|
| `system.go`    | All services and their wire-up (DI)         |
| `demo_test.go` | Test of the full cycle of system operation  |
| `cmd/main.go`  | Running demo                                |
                            --- 

## ▶️ Launch

```bash
# Run interactive demo:
go run ./09_complete_demo/cmd/main.go

# Tests only:
go test ./09_complete_demo/... -v
```

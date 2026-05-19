# Learn and Go - Module Study Guide

This guide is the simpler companion to the module READMEs. Use it when a module feels too large: read the card, open the suggested files, run the tests, then return to the full README.

## How To Study A Module

1. Read the "Plain English" paragraph.
2. Open the "Read first" files in order.
3. Run the module test command.
4. Do the "Practice" task.
5. Explain the "Checkpoint" out loud before moving on.

## Module 11 - Pointers

**Plain English:** A pointer is an address to a value. Use pointers when you need to mutate the original value, avoid copying a large value, or express "not provided" with `nil`.

**Read first:** `11_pointers/README.md`, then `pointers.go`, then `pointers_test.go`.

**Practice:** Add a small function that updates a struct through a pointer, then write the same function with a value receiver and compare the result.

**Checkpoint:** You can explain the difference between `*T`, `&value`, `*ptr`, and a `nil` pointer.

## Module 01 - Interfaces

**Plain English:** Interfaces let business code depend on behavior instead of a concrete database, broker, logger, or HTTP client.

**Read first:** `01_interfaces/domain.go`, `repository.go`, `service.go`, then `design.go`.

**Practice:** Create a second repository implementation or decorator and pass it to the same service without changing the service.

**Checkpoint:** You can explain "accept interfaces, return concrete types" and why the consumer should define the interface.

## Module 14 - Error Handling

**Plain English:** Errors should carry meaning. Wrap errors to preserve context, use `errors.Is` for known categories, and use `errors.As` when an error has structured data.

**Read first:** `14_error_handling/errors.go`, then `errors_test.go`.

**Practice:** Add a new domain error and map it to an HTTP status code.

**Checkpoint:** You can explain when to use sentinel errors, custom error types, behavioral errors, and `errors.Join`.

## Module 02 - DTOs

**Plain English:** Do not use one struct everywhere. Requests, domain models, database rows, and responses change for different reasons.

**Read first:** `02_dto/models.go`, `mapper.go`, `validation.go`.

**Practice:** Add one internal-only domain field and prove it does not leak to the response DTO.

**Checkpoint:** You can draw the path: request DTO -> validation -> domain model -> persistence model -> response DTO.

## Module 13 - Production Patterns

**Plain English:** Production code is code that behaves well under failure: it can be canceled, configured, logged, retried, stopped, and protected from dependency outages.

**Read first:** `context.go`, `config.go`, `logging.go`, `retry.go`, `circuit_breaker.go`, `shutdown.go`.

**Practice:** Add a timeout to a call path and test that cancellation returns quickly.

**Checkpoint:** You can explain why `context.Context` is the first argument and why retry needs jitter.

## Module 12 - HTTP API

**Plain English:** An HTTP handler is a thin adapter: decode input, validate it, call the service, map errors, encode output.

**Read first:** `12_http_api/server.go`, `handlers.go`, `middleware.go`, then `handlers_test.go`.

**Practice:** Add one route and test it with `httptest`.

**Checkpoint:** You can explain why handlers should not contain business logic.

## Module 03 - gRPC

**Plain English:** gRPC is a strongly typed internal API. The `.proto` file is the contract, generated Go code is the adapter, and the server implements business operations.

**Read first:** `03_grpc/proto/order.proto`, `server/server.go`, `server/interceptors.go`, `server/server_test.go`.

**Practice:** Add one field to the proto response, regenerate code, and update the server/tests.

**Checkpoint:** You can explain unary RPC, server streaming, status codes, interceptors, and `bufconn`.

## Module 04 - Messaging

**Plain English:** Messaging lets services react to facts without calling each other directly. Because messages may be delivered more than once, handlers must be idempotent.

**Read first:** `04_messaging/bus.go`, `idempotency.go`, `dlq.go`.

**Practice:** Publish the same event twice and prove the handler only applies the business effect once.

**Checkpoint:** You can explain event vs command vs query, at-least-once delivery, idempotency, and dead-letter queues.

## Module 05 - Unit Testing

**Plain English:** Unit tests isolate one piece of behavior. Mocks, stubs, and spies replace dependencies so the test can focus on one decision.

**Read first:** `05_unit_testing/service.go`, `mocks.go`, `service_test.go`, `table_driven_test.go`, `golden_test.go`.

**Practice:** Add a validation rule to `PlaceOrder`, then cover it with a table-driven test.

**Checkpoint:** You can choose between a manual fake, a spy, and `testify/mock`.

## Module 06 - Integration Testing

**Plain English:** Integration tests prove real components work together. They are slower than unit tests, so they need strong setup, teardown, and isolation.

**Read first:** `06_integration_testing/suite_test.go`, then `testcontainers_example_test.go`.

**Practice:** Add a test that writes two records and proves each can be read independently.

**Checkpoint:** You can explain suite lifecycle, fresh state, transaction rollback, and build tags for Docker-based tests.

## Module 15 - Concurrency Patterns

**Plain English:** Concurrency is not "start unlimited goroutines." It is controlled work: worker pools, pipelines, fan-out/fan-in, rate limits, and semaphores.

**Read first:** `worker_pool.go`, `fan_out.go`, `rate_limiter.go`, then `patterns_test.go`.

**Practice:** Add a concurrency limit to a fan-out operation and verify no more than N workers run at once.

**Checkpoint:** You can explain the difference between a rate limiter and a semaphore.

## Module 08 - Communication Patterns

**Plain English:** Architecture starts with one question: do you need an answer now? If yes, use sync communication. If no, use async communication.

**Read first:** `08_communication_patterns/README.md`, `patterns.go`, `retry.go`, `circuit_breaker.go`.

**Practice:** Pick a real feature, decide REST/gRPC/Kafka/RabbitMQ/NATS, and write down why.

**Checkpoint:** You can explain Saga, Outbox, CQRS, retry, and circuit breaker without mixing their responsibilities.

## Module 07 - Capstone

**Plain English:** This module combines the early patterns into one service: DTOs at the edge, domain logic in the middle, repository and publisher behind interfaces.

**Read first:** `07_capstone/service.go`, then `service_test.go`.

**Practice:** Add a new event after an order is canceled and test that the service still works if publishing fails.

**Checkpoint:** You can trace `CreateOrder` from request DTO to saved domain object to response DTO.

## Module 09 - Complete Demo

**Plain English:** This is the mini-system: order, inventory, notifications, analytics, and events are wired together so you can see how modules cooperate.

**Read first:** `09_complete_demo/system.go`, `cmd/main.go`, `demo_test.go`.

**Practice:** Add a new subscriber to `order.created` and show that `OrderService` does not need to know it exists.

**Checkpoint:** You can explain dependency injection, sync inventory checks, async event subscribers, and system wiring.

## Module 10 - Database

**Plain English:** Database code must protect data consistency: migrations shape the schema, transactions keep multi-table writes atomic, and tests isolate state.

**Read first:** `10_database/README.md`, `migrations/000001_init.up.sql`, `repository/postgres.go`, `repository/health.go`.

**Practice:** Add one query method and cover it with an integration test.

**Checkpoint:** You can explain migrations, transactions, soft delete, optimistic locking, connection pools, and readiness checks.

## Final Review Checklist

- I can identify the boundary between transport, service, domain, and infrastructure.
- I can replace an implementation through an interface without changing business code.
- I can choose sync or async communication for a concrete scenario.
- I can test a module with mocks and test a flow with real components.
- I can explain what happens when a dependency is slow, down, or returns invalid data.


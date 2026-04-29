//Package communication demonstrates communication patterns between services.
//
//In this module we show:
//1. Synchronous call via interface (analogous to gRPC)
//2. Asynchronous pub/sub via interface (analogous to Kafka/RabbitMQ)
//3. Saga Pattern - distributed transactions
//4. Outbox Pattern - reliable event publishing
//
//Everything works in-process - without a real gRPC server and broker.
//The ideas and interfaces are the same as in production.
package communication

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// DOMAIN
// ═══════════════════════════════════════════════════════════════

//OrderStatus order status.
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusCancelled OrderStatus = "cancelled"
	StatusFailed    OrderStatus = "failed"
)

var (
	ErrOrderNotFound     = errors.New("order not found")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrPaymentFailed     = errors.New("payment failed")
)

//Order - domain model.
type Order struct {
	ID          string
	CustomerID  string
	ProductID   string
	Quantity    int
	TotalAmount float64
	Status      OrderStatus
	CreatedAt   time.Time
}

// ═══════════════════════════════════════════════════════════════
//PATTERN 1: SYNCHRONOUS COMMUNICATION (similar to gRPC)
//
//InventoryChecker - interface for synchronous warehouse checking.
//In a real project: gRPC client for InventoryService.
//Here: in-memory implementation for demonstration.
// ═══════════════════════════════════════════════════════════════

//InventoryChecker - synchronous request to the warehouse.
//👉 The service is WAITING for a response - you need to know “is there a product?” right now.
type InventoryChecker interface {
	CheckStock(productID string, quantity int) (bool, error)
	ReserveStock(productID string, quantity int) error
	ReleaseStock(productID string, quantity int) error
}

//PaymentProcessor - synchronous payment processing.
//👉 An immediate answer is needed - whether the payment went through.
type PaymentProcessor interface {
	ProcessPayment(customerID string, amount float64) (string, error)
	RefundPayment(transactionID string) error
}

//InMemoryInventory is an in-memory implementation of InventoryChecker.
//In a real project this would be a gRPC client.
type InMemoryInventory struct {
	mu    sync.Mutex
	stock map[string]int
}

//NewInMemoryInventory creates an inventory with opening balances.
func NewInMemoryInventory(initial map[string]int) *InMemoryInventory {
	return &InMemoryInventory{stock: initial}
}

func (i *InMemoryInventory) CheckStock(productID string, quantity int) (bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	available, ok := i.stock[productID]
	if !ok {
		return false, nil
	}
	return available >= quantity, nil
}

func (i *InMemoryInventory) ReserveStock(productID string, quantity int) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.stock[productID] < quantity {
		return ErrInsufficientStock
	}
	i.stock[productID] -= quantity
	return nil
}

func (i *InMemoryInventory) ReleaseStock(productID string, quantity int) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.stock[productID] += quantity
	return nil
}

//InMemoryPayment is an in-memory implementation of PaymentProcessor.
type InMemoryPayment struct {
	mu           sync.Mutex
	transactions map[string]float64
	failFor      map[string]bool //clients whose payment “fails”
}

//NewInMemoryPayment creates a payment processor.
func NewInMemoryPayment() *InMemoryPayment {
	return &InMemoryPayment{
		transactions: make(map[string]float64),
		failFor:      make(map[string]bool),
	}
}

//SetFailFor specifies the client whose payment will fail (for tests).
func (p *InMemoryPayment) SetFailFor(customerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failFor[customerID] = true
}

func (p *InMemoryPayment) ProcessPayment(customerID string, amount float64) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failFor[customerID] {
		return "", ErrPaymentFailed
	}
	txID := fmt.Sprintf("tx-%s-%d", customerID, time.Now().UnixNano())
	p.transactions[txID] = amount
	return txID, nil
}

func (p *InMemoryPayment) RefundPayment(transactionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.transactions, transactionID)
	return nil
}

// ═══════════════════════════════════════════════════════════════
//PATTERN 2: ASYNCHRONOUS COMMUNICATION (analogous to Kafka/RabbitMQ)
//
//EventBus is an interface for asynchronously publishing events.
//In a real project: Kafka producer/consumer.
// ═══════════════════════════════════════════════════════════════

//Event - the basic structure of an event.
//👉 All events: type + payload + metadata for tracing.
type Event struct {
	ID            string    //unique event ID (for idempotency)
	Type          string    // "order.created", "payment.completed", etc.
	CorrelationID string    //Request ID for tracing through services
	Payload       any       //event data
	OccurredAt    time.Time //when did it happen
}

//EventHandler - event handler function.
type EventHandler func(event Event) error

//EventPublisher - publishes events.
type EventPublisher interface {
	Publish(event Event) error
}

//EventSubscriber - subscribes to events.
type EventSubscriber interface {
	Subscribe(eventType string, handler EventHandler)
}

//EventBus - combines publisher + subscriber.
type EventBus interface {
	EventPublisher
	EventSubscriber
}

//InMemoryEventBus is an in-memory implementation of EventBus.
//👉 Similar to Kafka, but in memory. The interface is the same!
type InMemoryEventBus struct {
	mu       sync.RWMutex
	handlers map[string][]EventHandler
	events   []Event //event log - like Kafka topic
}

//NewInMemoryEventBus creates an event bus.
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers: make(map[string][]EventHandler),
	}
}

func (b *InMemoryEventBus) Publish(event Event) error {
	b.mu.Lock()
	b.events = append(b.events, event) //log the event
	handlers := append([]EventHandler(nil), b.handlers[event.Type]...)
	b.mu.Unlock()

	//👉 In real Kafka: events fall into topic and are processed asynchronously.
	//Here: synchronous call of handlers for ease of testing.
	for _, h := range handlers {
		if err := h(event); err != nil {
			return fmt.Errorf("handler error for %s: %w", event.Type, err)
		}
	}
	return nil
}

func (b *InMemoryEventBus) Subscribe(eventType string, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

//EventLog returns all published events (for tests).
func (b *InMemoryEventBus) EventLog() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]Event, len(b.events))
	copy(result, b.events)
	return result
}

// ═══════════════════════════════════════════════════════════════
//PATTERN 3: SAGA PATTERN
//
//Saga is a way to implement a “transaction” across multiple services.
//If one step fails, we perform compensatory actions.
//
//Example: CreateOrder = ReserveStock + ProcessPayment
//If ProcessPayment fell → ReleaseStock (compensation)
// ═══════════════════════════════════════════════════════════════

//OrderSagaService - orchestrates order creation.
//Uses SYNCHRONOUS calls (gRPC) for critical steps.
//Uses ASYNCHRONOUS events for notifications.
type OrderSagaService struct {
	inventory InventoryChecker //synchronous call (gRPC analogue)
	payment   PaymentProcessor //synchronous call (gRPC analogue)
	publisher EventPublisher   //asynchronous (Kafka analogue)
	orders    map[string]*Order
	mu        sync.Mutex
}

//NewOrderSagaService creates a service with the saga pattern.
func NewOrderSagaService(
	inventory InventoryChecker,
	payment PaymentProcessor,
	publisher EventPublisher,
) *OrderSagaService {
	return &OrderSagaService{
		inventory: inventory,
		payment:   payment,
		publisher: publisher,
		orders:    make(map[string]*Order),
	}
}

//CreateOrder - saga for creating an order.
//
//Steps:
//1. Check product availability (gRPC → InventoryService)
//2. Reserve goods (gRPC → InventoryService)
//3. Process the payment (gRPC → PaymentService)
//↑ If step 3 fails → compensation: release reserve
//4. Publish event "order.created" (Kafka -> all listeners)
func (s *OrderSagaService) CreateOrder(customerID, productID string, quantity int, pricePerItem float64) (*Order, error) {
	total := float64(quantity) * pricePerItem

	//─── Step 1: Synchronous call - checking the warehouse ───────────
	//👉 We need an answer right now. We use a synchronous interface.
	available, err := s.inventory.CheckStock(productID, quantity)
	if err != nil {
		return nil, fmt.Errorf("CreateOrder: check stock: %w", err)
	}
	if !available {
		return nil, fmt.Errorf("CreateOrder: %w", ErrInsufficientStock)
	}

	//─── Step 2: Synchronous call - reserve the goods ─────────
	if err := s.inventory.ReserveStock(productID, quantity); err != nil {
		return nil, fmt.Errorf("CreateOrder: reserve stock: %w", err)
	}

	//─── Step 3: Synchronous call - payment ────────────────────
	//👉 If the payment fails, you need to release the reserve!
	//This COMPENSATING action is the essence of the Saga pattern.
	txID, err := s.payment.ProcessPayment(customerID, total)
	if err != nil {
		//💡 COMPENSATION: we cancel the reserve
		_ = s.inventory.ReleaseStock(productID, quantity)
		return nil, fmt.Errorf("CreateOrder: payment failed (stock released): %w", err)
	}

	//─── Create an order ──────────────────── ────────────────────
	order := &Order{
		ID:          fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID:  customerID,
		ProductID:   productID,
		Quantity:    quantity,
		TotalAmount: total,
		Status:      StatusConfirmed,
		CreatedAt:   time.Now(),
	}

	s.mu.Lock()
	s.orders[order.ID] = order
	s.mu.Unlock()

	//─── Step 4: Asynchronous event - notify everyone ────────
	//👉 We publish a FACT. We are not waiting for an answer.
	//Notification, Analytics, Warehouse will pick it up themselves.
	_ = s.publisher.Publish(Event{
		ID:            fmt.Sprintf("evt-%s", order.ID),
		Type:          "order.created",
		CorrelationID: order.ID,
		Payload: map[string]any{
			"order_id":       order.ID,
			"customer_id":    customerID,
			"product_id":     productID,
			"quantity":       quantity,
			"total_amount":   total,
			"transaction_id": txID,
		},
		OccurredAt: time.Now(),
	})

	return order, nil
}

//GetOrder returns an order by ID.
func (s *OrderSagaService) GetOrder(id string) (*Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return nil, ErrOrderNotFound
	}
	return o, nil
}

// ═══════════════════════════════════════════════════════════════
//PATTERN 4: OUTBOX PATTERN
//
//Problem: saved the order in the database + published the event.
//What if it fell between these two operations?
//
//Solution: save the event in the same transaction as the order.
//A separate worker reads the outbox and publishes events.
// ═══════════════════════════════════════════════════════════════

//OutboxEvent – ​​record in the outbox table.
type OutboxEvent struct {
	ID          string
	EventType   string
	Payload     any
	ProcessedAt *time.Time //nil = not processed
	CreatedAt   time.Time
}

//OutboxStore - storage of outbox events (usually the same database as orders).
type OutboxStore interface {
	SaveEvent(event OutboxEvent) error
	GetUnprocessed() ([]OutboxEvent, error)
	MarkProcessed(id string) error
}

//InMemoryOutboxStore - in-memory implementation.
type InMemoryOutboxStore struct {
	mu     sync.Mutex
	events map[string]*OutboxEvent
}

//NewInMemoryOutboxStore creates a store.
func NewInMemoryOutboxStore() *InMemoryOutboxStore {
	return &InMemoryOutboxStore{events: make(map[string]*OutboxEvent)}
}

func (s *InMemoryOutboxStore) SaveEvent(event OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[event.ID] = &event
	return nil
}

func (s *InMemoryOutboxStore) GetUnprocessed() ([]OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []OutboxEvent
	for _, e := range s.events {
		if e.ProcessedAt == nil {
			result = append(result, *e)
		}
	}
	return result, nil
}

func (s *InMemoryOutboxStore) MarkProcessed(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.events[id]; ok {
		now := time.Now()
		e.ProcessedAt = &now
	}
	return nil
}

//OutboxWorker is a background process that publishes events from outbox.
//👉 In a real project: runs periodically (cron or goroutine).
type OutboxWorker struct {
	store     OutboxStore
	publisher EventPublisher
}

//NewOutboxWorker creates a worker.
func NewOutboxWorker(store OutboxStore, publisher EventPublisher) *OutboxWorker {
	return &OutboxWorker{store: store, publisher: publisher}
}

//ProcessPending processes all unhandled events from the outbox.
func (w *OutboxWorker) ProcessPending() error {
	events, err := w.store.GetUnprocessed()
	if err != nil {
		return fmt.Errorf("OutboxWorker: get unprocessed: %w", err)
	}

	for _, e := range events {
		//Publish an event to the broker
		if err := w.publisher.Publish(Event{
			ID:         e.ID,
			Type:       e.EventType,
			Payload:    e.Payload,
			OccurredAt: e.CreatedAt,
		}); err != nil {
			//We don't update ProcessedAt - we'll try again next time
			continue
		}

		//Mark as processed only after successful publication
		if err := w.store.MarkProcessed(e.ID); err != nil {
			return fmt.Errorf("OutboxWorker: mark processed: %w", err)
		}
	}
	return nil
}

//OrderWithOutboxService is a service that uses the Outbox Pattern.
//Demonstrates atomic persistence of order + events.
type OrderWithOutboxService struct {
	orders map[string]*Order
	outbox OutboxStore
	mu     sync.Mutex
}

//NewOrderWithOutboxService creates a service with the Outbox Pattern.
func NewOrderWithOutboxService(outbox OutboxStore) *OrderWithOutboxService {
	return &OrderWithOutboxService{
		orders: make(map[string]*Order),
		outbox: outbox,
	}
}

//CreateOrder atomically stores the order and event in an outbox.
//👉 In a real project, both INSERTs are in one SQL transaction.
func (s *OrderWithOutboxService) CreateOrder(customerID string, amount float64) (*Order, error) {
	order := &Order{
		ID:          fmt.Sprintf("order-%d", time.Now().UnixNano()),
		CustomerID:  customerID,
		TotalAmount: amount,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
	}

	//We simulate a transaction: we save the order AND the event “together”
	s.mu.Lock()
	s.orders[order.ID] = order

	//We save the event in outbox - in the same “transaction”
	outboxEvent := OutboxEvent{
		ID:        fmt.Sprintf("outbox-%s", order.ID),
		EventType: "order.created",
		Payload: map[string]any{
			"order_id":    order.ID,
			"customer_id": customerID,
			"amount":      amount,
		},
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()

	if err := s.outbox.SaveEvent(outboxEvent); err != nil {
		return nil, fmt.Errorf("CreateOrder: save outbox event: %w", err)
	}

	//The event is NOT published here - OutboxWorker does!
	return order, nil
}

//GetOrder returns the order.
func (s *OrderWithOutboxService) GetOrder(id string) (*Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return nil, ErrOrderNotFound
	}
	return o, nil
}

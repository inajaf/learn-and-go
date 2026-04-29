//Package demo contains a complete demonstration of a mini-system of microservices.
//
//This is the "final project" of the course - this is where everything works together:
//- Interfaces and Dependency Injection (Module 1)
//- DTO and layer mapping (Module 2)
//- Messaging / Event Bus (Module 4)
//- Unit tests with mocks (Module 5)
//- Communication Patterns: Saga (Module 8)
//
//The system consists of:
//- OrderService - creates orders, orchestrates saga
//- InventoryService - checks the availability of goods (synchronously)
//- NotificationService - sends notifications (asynchronously)
//- AnalyticsService - records events (asynchronously)
//- EventBus - connects services asynchronously
package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════
//DOMAIN - pure business models (Module 1: interfaces)
// ═══════════════════════════════════════════════════════════════

var (
	ErrOrderNotFound     = errors.New("order not found")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrInvalidInput      = errors.New("invalid input")
)

//OrderStatus - Value Object (Module 1: domain.go)
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusCancelled OrderStatus = "cancelled"
)

//Order - domain model. No tags, no dependencies.
//👉 Module 1: domain object
type Order struct {
	ID          string
	CustomerID  string
	Items       []OrderItem
	TotalAmount float64
	Status      OrderStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

//OrderItem - order item.
type OrderItem struct {
	ProductID string
	Name      string
	Quantity  int
	UnitPrice float64
}

//Total calculates the sum of the positions.
func (o *Order) Total() float64 {
	var sum float64
	for _, item := range o.Items {
		sum += float64(item.Quantity) * item.UnitPrice
	}
	return sum
}

//Cancel cancels the order.
func (o *Order) Cancel() error {
	if o.Status == StatusCancelled {
		return fmt.Errorf("order already cancelled")
	}
	o.Status = StatusCancelled
	o.UpdatedAt = time.Now()
	return nil
}

// ═══════════════════════════════════════════════════════════════
//DTOs - transport objects (Module 2: dto)
// ═══════════════════════════════════════════════════════════════

//CreateOrderRequest - DTO of the incoming request.
//👉 Module 2: only the fields required by the client, no ID/Status/CreatedAt
type CreateOrderRequest struct {
	CustomerID string              `json:"customer_id"`
	Items      []CreateItemRequest `json:"items"`
}

//CreateItemRequest - position in the request.
type CreateItemRequest struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

//OrderResponse - DTO of the response.
//👉 Module 2: only what the client needs, without internal fields
type OrderResponse struct {
	ID          string         `json:"id"`
	CustomerID  string         `json:"customer_id"`
	Items       []ItemResponse `json:"items"`
	TotalAmount float64        `json:"total_amount"`
	Status      string         `json:"status"`
	CreatedAt   string         `json:"created_at"` //string, not time.Time
}

//ItemResponse - position in the response.
type ItemResponse struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Subtotal  float64 `json:"subtotal"`
}

// ═══════════════════════════════════════════════════════════════
//INTERFACES - contracts (Module 1: repository pattern)
// ═══════════════════════════════════════════════════════════════

//OrderRepository - order storage contract.
//👉 context.Context is the first parameter (Go standard).
type OrderRepository interface {
	Save(ctx context.Context, order *Order) error
	FindByID(ctx context.Context, id string) (*Order, error)
	FindAll(ctx context.Context) ([]*Order, error)
}

//StockChecker is a synchronous warehouse checking contract.
//👉 In a real project - gRPC client for InventoryService
//👉Module 8: Synchronous Communication
type StockChecker interface {
	HasStock(ctx context.Context, productID string, quantity int) (bool, error)
	Reserve(ctx context.Context, productID string, quantity int) error
}

//EventPublisher is an asynchronous event publishing contract.
//👉 In a real project - Kafka producer
//👉Module 8: Asynchronous Communication
type EventPublisher interface {
	Publish(eventType string, payload map[string]any) error
}

//EventSubscriber - subscription to events.
type EventSubscriber interface {
	Subscribe(eventType string, handler func(eventType string, payload map[string]any))
}

//Compile-time checks (Module 1: best practice)
var _ OrderRepository = (*InMemoryOrderRepository)(nil)
var _ StockChecker = (*InMemoryInventoryService)(nil)
var _ EventPublisher = (*InMemoryEventBus)(nil)

// ═══════════════════════════════════════════════════════════════
//MAPPER - conversion between layers (Module 2: mapper)
// ═══════════════════════════════════════════════════════════════

//requestToDomain converts DTO → domain model with validation.
//👉 Module 2: mapper, validation at the DTO level
func requestToDomain(req CreateOrderRequest) (*Order, error) {
	if req.CustomerID == "" {
		return nil, fmt.Errorf("%w: customer_id is required", ErrInvalidInput)
	}
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("%w: at least one item is required", ErrInvalidInput)
	}

	items := make([]OrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		if item.Quantity <= 0 {
			return nil, fmt.Errorf("%w: item quantity must be positive", ErrInvalidInput)
		}
		if item.UnitPrice < 0 {
			return nil, fmt.Errorf("%w: item price cannot be negative", ErrInvalidInput)
		}
		items = append(items, OrderItem{
			ProductID: item.ProductID,
			Name:      item.Name,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
		})
	}

	now := time.Now()
	return &Order{
		ID:         fmt.Sprintf("order-%d", now.UnixNano()),
		CustomerID: req.CustomerID,
		Items:      items,
		Status:     StatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

//domainToResponse converts domain model → response DTO.
//👉Module 2: output mapping
func domainToResponse(order *Order) OrderResponse {
	items := make([]ItemResponse, len(order.Items))
	for i, item := range order.Items {
		items[i] = ItemResponse{
			ProductID: item.ProductID,
			Name:      item.Name,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
			Subtotal:  float64(item.Quantity) * item.UnitPrice, //calculate on the fly
		}
	}
	return OrderResponse{
		ID:          order.ID,
		CustomerID:  order.CustomerID,
		Items:       items,
		TotalAmount: order.TotalAmount,
		Status:      string(order.Status),
		CreatedAt:   order.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ═══════════════════════════════════════════════════════════════
//ORDER SERVICE - business logic + Saga orchestration
//Module 1: service + DI
//Module 8: Saga Pattern
// ═══════════════════════════════════════════════════════════════

//OrderService is the main service of the system.
//Depends only on interfaces - does not know about specific implementations.
type OrderService struct {
	repo      OrderRepository //Module 1: DI via Interface
	inventory StockChecker    //synchronous call (gRPC in production)
	events    EventPublisher  //asynchronous events (Kafka in production)
}

//NewOrderService - constructor with Dependency Injection.
func NewOrderService(repo OrderRepository, inventory StockChecker, events EventPublisher) *OrderService {
	return &OrderService{
		repo:      repo,
		inventory: inventory,
		events:    events,
	}
}

//CreateOrder - full cycle of order creation (Saga Pattern).
//
// Saga steps:
//1. DTO validation (sync)
//2. Warehouse check (sync - response needed)
//3. Reservation of goods (sync - reply needed)
//4. Save order (sync)
//5. Publishing an event (async - no waiting)
func (s *OrderService) CreateOrder(ctx context.Context, req CreateOrderRequest) (OrderResponse, error) {
	//─── Step 1: Validation + DTO mapping → Domain ─────────────
	order, err := requestToDomain(req)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("CreateOrder: %w", err)
	}
	order.TotalAmount = order.Total()

	//─── Step 2: Synchronous warehouse check (similar to gRPC) ─────
	//👉 We need an answer right now - you cannot create an order for a non-existent product
	for _, item := range order.Items {
		ok, err := s.inventory.HasStock(ctx, item.ProductID, item.Quantity)
		if err != nil {
			return OrderResponse{}, fmt.Errorf("CreateOrder: check stock %s: %w", item.ProductID, err)
		}
		if !ok {
			return OrderResponse{}, fmt.Errorf("CreateOrder: %w: product %s (qty: %d)",
				ErrInsufficientStock, item.ProductID, item.Quantity)
		}
	}

	//─── Step 3: Synchronous reservation
	reservedItems := make([]OrderItem, 0)
	for _, item := range order.Items {
		if err := s.inventory.Reserve(ctx, item.ProductID, item.Quantity); err != nil {
			//COMPENSATION: we release already reserved
			for _, reserved := range reservedItems {
				_ = s.inventory.Reserve(ctx, reserved.ProductID, -reserved.Quantity)
			}
			return OrderResponse{}, fmt.Errorf("CreateOrder: reserve %s: %w", item.ProductID, err)
		}
		reservedItems = append(reservedItems, item)
	}

	//─── Step 4: Saving to the repository ─────────────────────
	order.Status = StatusConfirmed
	if err := s.repo.Save(ctx, order); err != nil {
		return OrderResponse{}, fmt.Errorf("CreateOrder: save: %w", err)
	}

	//─── Step 5: Asynchronous event (similar to Kafka) ────────────
	//👉 Fire-and-forget: we publish and DO NOT wait for a response.
	//Notification and Analytics will pick up the event themselves.
	_ = s.events.Publish("order.created", map[string]any{
		"order_id":    order.ID,
		"customer_id": order.CustomerID,
		"total":       order.TotalAmount,
		"item_count":  len(order.Items),
	})

	//─── Step 6: Mapping Domain → Response DTO ────────────────
	return domainToResponse(order), nil
}

//GetOrder returns an order by ID.
func (s *OrderService) GetOrder(ctx context.Context, id string) (OrderResponse, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("GetOrder: %w", err)
	}
	return domainToResponse(order), nil
}

//CancelOrder cancels the order.
func (s *OrderService) CancelOrder(ctx context.Context, id string) (OrderResponse, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("CancelOrder: %w", err)
	}

	if err := order.Cancel(); err != nil {
		return OrderResponse{}, fmt.Errorf("CancelOrder: %w", err)
	}

	if err := s.repo.Save(ctx, order); err != nil {
		return OrderResponse{}, fmt.Errorf("CancelOrder: save: %w", err)
	}

	_ = s.events.Publish("order.cancelled", map[string]any{
		"order_id":    order.ID,
		"customer_id": order.CustomerID,
	})

	return domainToResponse(order), nil
}

//ListOrders returns all orders.
func (s *OrderService) ListOrders(ctx context.Context) ([]OrderResponse, error) {
	orders, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListOrders: %w", err)
	}
	result := make([]OrderResponse, len(orders))
	for i, o := range orders {
		result[i] = domainToResponse(o)
	}
	return result, nil
}

// ═══════════════════════════════════════════════════════════════
//INFRASTRUCTURE - interface implementations
// ═══════════════════════════════════════════════════════════════

//InMemoryOrderRepository - implementation of Repository in memory.
//👉 Module 1: InMemory implementation with sync.RWMutex
type InMemoryOrderRepository struct {
	mu     sync.RWMutex
	orders map[string]*Order
}

//NewInMemoryOrderRepository creates a repository.
func NewInMemoryOrderRepository() *InMemoryOrderRepository {
	return &InMemoryOrderRepository{orders: make(map[string]*Order)}
}

func (r *InMemoryOrderRepository) Save(_ context.Context, order *Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := *order
	items := make([]OrderItem, len(order.Items))
	copy(items, order.Items)
	copied.Items = items
	r.orders[order.ID] = &copied
	return nil
}

func (r *InMemoryOrderRepository) FindByID(_ context.Context, id string) (*Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.orders[id]
	if !ok {
		return nil, fmt.Errorf("FindByID %q: %w", id, ErrOrderNotFound)
	}
	copied := *o
	return &copied, nil
}

func (r *InMemoryOrderRepository) FindAll(_ context.Context) ([]*Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Order, 0, len(r.orders))
	for _, o := range r.orders {
		copied := *o
		result = append(result, &copied)
	}
	return result, nil
}

//InMemoryInventoryService is an in-memory implementation of StockChecker.
//👉 In a real project, this is a gRPC client for InventoryService
//👉Module 8: Synchronous communication via interface
type InMemoryInventoryService struct {
	mu    sync.Mutex
	stock map[string]int
	log   []string //operation log for demo
}

//NewInMemoryInventoryService creates an inventory service.
func NewInMemoryInventoryService(initial map[string]int) *InMemoryInventoryService {
	stock := make(map[string]int)
	for k, v := range initial {
		stock[k] = v
	}
	return &InMemoryInventoryService{stock: stock}
}

func (i *InMemoryInventoryService) HasStock(_ context.Context, productID string, quantity int) (bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	available := i.stock[productID]
	has := available >= quantity
	i.log = append(i.log, fmt.Sprintf("HasStock(%s, %d) = %v (available: %d)",
		productID, quantity, has, available))
	return has, nil
}

func (i *InMemoryInventoryService) Reserve(_ context.Context, productID string, quantity int) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if quantity > 0 && i.stock[productID] < quantity {
		return ErrInsufficientStock
	}
	i.stock[productID] -= quantity
	i.log = append(i.log, fmt.Sprintf("Reserve(%s, %d) → stock now: %d",
		productID, quantity, i.stock[productID]))
	return nil
}

//StockLevel returns the current stock of the product.
func (i *InMemoryInventoryService) StockLevel(productID string) int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.stock[productID]
}

//Log returns a log of operations.
func (i *InMemoryInventoryService) Log() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	result := make([]string, len(i.log))
	copy(result, i.log)
	return result
}

// ═══════════════════════════════════════════════════════════════
//EVENT BUS - asynchronous communication
//👉 Module 4: Pub/Sub
//👉 Module 8: When to use async
// ═══════════════════════════════════════════════════════════════

//EventRecord - a record of a published event.
type EventRecord struct {
	Type        string
	Payload     map[string]any
	PublishedAt time.Time
}

//InMemoryEventBus is an in-memory implementation of EventBus.
//In a real project: Kafka producer + consumer.
type InMemoryEventBus struct {
	mu       sync.RWMutex
	handlers map[string][]func(string, map[string]any)
	log      []EventRecord //log of all events
}

//NewInMemoryEventBus creates an event bus.
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers: make(map[string][]func(string, map[string]any)),
	}
}

func (b *InMemoryEventBus) Publish(eventType string, payload map[string]any) error {
	b.mu.Lock()
	b.log = append(b.log, EventRecord{
		Type:        eventType,
		Payload:     payload,
		PublishedAt: time.Now(),
	})
	existingHandlers := b.handlers[eventType]
	handlers := make([]func(string, map[string]any), len(existingHandlers))
	copy(handlers, existingHandlers)
	b.mu.Unlock()

	for _, h := range handlers {
		h(eventType, payload)
	}
	return nil
}

func (b *InMemoryEventBus) Subscribe(eventType string, handler func(string, map[string]any)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

//EventLog returns a log of all events.
func (b *InMemoryEventBus) EventLog() []EventRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]EventRecord, len(b.log))
	copy(result, b.log)
	return result
}

// ═══════════════════════════════════════════════════════════════
//NOTIFICATION SERVICE - asynchronous subscriber
//👉Module 4: Consumer
// ═══════════════════════════════════════════════════════════════

//NotificationService - sends notifications based on events.
//Doesn't know about OrderService - subscribed via EventBus.
type NotificationService struct {
	mu         sync.Mutex
	sentEmails []string
}

//NewNotificationService creates and registers a service with the EventBus.
func NewNotificationService(bus *InMemoryEventBus) *NotificationService {
	svc := &NotificationService{}

	//Subscribe to events - OrderService doesn’t know about this!
	bus.Subscribe("order.created", svc.handleOrderCreated)
	bus.Subscribe("order.cancelled", svc.handleOrderCancelled)

	return svc
}

func (n *NotificationService) handleOrderCreated(eventType string, payload map[string]any) {
	n.mu.Lock()
	defer n.mu.Unlock()
	msg := fmt.Sprintf("[EMAIL] Order %s created for customer %s (total: %.2f)",
		payload["order_id"], payload["customer_id"], payload["total"])
	n.sentEmails = append(n.sentEmails, msg)
}

func (n *NotificationService) handleOrderCancelled(eventType string, payload map[string]any) {
	n.mu.Lock()
	defer n.mu.Unlock()
	msg := fmt.Sprintf("[EMAIL] Order %s cancelled for customer %s",
		payload["order_id"], payload["customer_id"])
	n.sentEmails = append(n.sentEmails, msg)
}

//SentEmails returns a list of sent notifications.
func (n *NotificationService) SentEmails() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	result := make([]string, len(n.sentEmails))
	copy(result, n.sentEmails)
	return result
}

// ═══════════════════════════════════════════════════════════════
//ANALYTICS SERVICE - asynchronous subscriber
// ═══════════════════════════════════════════════════════════════

//AnalyticsService - records analytics on events.
type AnalyticsService struct {
	mu     sync.Mutex
	events []string
	stats  map[string]int
}

//NewAnalyticsService creates and registers a service with EventBus.
func NewAnalyticsService(bus *InMemoryEventBus) *AnalyticsService {
	svc := &AnalyticsService{
		stats: make(map[string]int),
	}

	bus.Subscribe("order.created", svc.trackEvent)
	bus.Subscribe("order.cancelled", svc.trackEvent)

	return svc
}

func (a *AnalyticsService) trackEvent(eventType string, payload map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stats[eventType]++
	a.events = append(a.events, fmt.Sprintf("[ANALYTICS] %s: %v", eventType, payload))
}

//Stats returns event statistics.
func (a *AnalyticsService) Stats() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make(map[string]int)
	for k, v := range a.stats {
		result[k] = v
	}
	return result
}

//Events returns a log of analytical events.
func (a *AnalyticsService) Events() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]string, len(a.events))
	copy(result, a.events)
	return result
}

// ═══════════════════════════════════════════════════════════════
//SYSTEM - assembly of the entire system (Dependency Injection / Wiring)
// ═══════════════════════════════════════════════════════════════

//System - the entire system assembled together.
//👉 This is the analogue of “main.go” in a microservice - wire-up happens here.
//
//In a real project they use wire (google/wire) or fx (uber/fx).
type System struct {
	OrderSvc     *OrderService
	Notification *NotificationService
	Analytics    *AnalyticsService
	Inventory    *InMemoryInventoryService
	EventBus     *InMemoryEventBus
}

//NewSystem creates the entire system: initializes services, connects dependencies.
//
//The initialization order is important:
//1. Infrastructure (EventBus, Inventory, Repository)
//2. Subscribers (Notification, Analytics - registered in EventBus)
//3. Business services (OrderService - receives dependencies via DI)
func NewSystem(initialStock map[string]int) *System {
	//Infrastructure
	bus := NewInMemoryEventBus()
	repo := NewInMemoryOrderRepository()
	inventory := NewInMemoryInventoryService(initialStock)

	//Asynchronous subscribers (register with EventBus)
	notification := NewNotificationService(bus)
	analytics := NewAnalyticsService(bus)

	//Main service (receives dependencies via DI)
	orderSvc := NewOrderService(repo, inventory, bus)

	return &System{
		OrderSvc:     orderSvc,
		Notification: notification,
		Analytics:    analytics,
		Inventory:    inventory,
		EventBus:     bus,
	}
}

//PrintStatus prints the system status.
func (s *System) PrintStatus() string {
	var sb strings.Builder

	sb.WriteString("═══════════════════════════════════════\n")
	sb.WriteString("SYSTEM STATUS\\n")
	sb.WriteString("═══════════════════════════════════════\n")

	//Orders
	orders, _ := s.OrderSvc.ListOrders(context.Background())
	sb.WriteString(fmt.Sprintf("\\n📦 Orders in the system: %d\\n", len(orders)))
	for _, o := range orders {
		sb.WriteString(fmt.Sprintf("- %s [%s] %.2f rub. (%d positions)\\\\n",
			o.ID, o.Status, o.TotalAmount, len(o.Items)))
	}

	//Notifications
	emails := s.Notification.SentEmails()
	sb.WriteString(fmt.Sprintf("\\n📧 Notifications sent: %d\\n", len(emails)))
	for _, e := range emails {
		sb.WriteString(fmt.Sprintf("   %s\n", e))
	}

	//Analytics
	stats := s.Analytics.Stats()
	sb.WriteString("\\n📊 Analytics:\\n")
	for eventType, count := range stats {
		sb.WriteString(fmt.Sprintf("   %s: %d\n", eventType, count))
	}

	//Event log
	eventLog := s.EventBus.EventLog()
	sb.WriteString(fmt.Sprintf("\\n📋 Bus events: %d\\n", len(eventLog)))
	for _, e := range eventLog {
		sb.WriteString(fmt.Sprintf("   [%s] %s\n",
			e.PublishedAt.Format("15:04:05.000"), e.Type))
	}

	return sb.String()
}

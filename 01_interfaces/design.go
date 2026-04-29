package interfaces

import (
	"context"
	"fmt"
)

// ─────────────────────────────────────────────────────────────────
//INTERFACES AS DESIGN TOOLS
//
//You're absolutely right: interfaces are, first of all, a way
//design the system BEFORE implementation.
//
//Senior developer process:
//
//STEP 1. Draw on paper what the system should do
//STEP 2. Describe CONTRACTS between parts of the system (interfaces)
//STEP 3. Write tests against these contracts
//STEP 4. Implement stubs so that tests compile
//STEP 5. Write real implementations
//STEP 6. Replace stubs with real implementations
//
//This is called "Interface-First Design" or "Design by Contract".
// ─────────────────────────────────────────────────────────────────

// =================================================================
//SCENARIO: We are designing a notification system for an online store.
//
//Task:
//When the order is confirmed, you need to:
//1. Send an email to the client
//2. Notify the warehouse via SMS
//3. Write to analytics
//
//We don't know yet:
//- What email service (SendGrid? SES? SMTP?)
//- Do you already have an SMS provider?
//- Where do we write analytics (BigQuery? Kafka? Postgres?)
//
//But that doesn't matter! We describe WHAT is needed, not HOW.
// =================================================================

// ─────────────────────────────────────────────────────────────────
//STEP 1: DESCRIBING THE CONTRACTS - what each part should be able to do.
// ─────────────────────────────────────────────────────────────────

//Notifier - can send notifications to clients.
//WHO implements it does not matter. Email, SMS, Push - we don't care.
type Notifier interface {
	NotifyOrderConfirmed(customerEmail, orderID string, amount float64) error
}

//WarehouseNotifier - can notify the warehouse about a new order.
type WarehouseNotifier interface {
	NotifyNewOrder(orderID string, items []string) error
}

//AnalyticsTracker - can record events for analytics.
type AnalyticsTracker interface {
	TrackOrderConfirmed(orderID, customerID string, amount float64)
}

// ─────────────────────────────────────────────────────────────────
//STEP 2: DESIGN THE SERVICE through contracts.
//
//OrderConfirmationService was written BEFORE we decided
//what email provider, what SMS gateway, what analytics.
//Fields exported - creation via structure literal,
//which makes dependencies explicit and IDE-friendly.
// ─────────────────────────────────────────────────────────────────

//OrderConfirmationService - orchestrates order confirmation.
//Depends only on the interfaces.
type OrderConfirmationService struct {
	Orders    OrderRepository   //repository (from repository.go)
	Notifier  Notifier          //notifications to the client
	Warehouse WarehouseNotifier //warehouse notification
	Analytics AnalyticsTracker  //analytics
}

//ConfirmOrder is the main method. Complete business logic.
//Written BEFORE the implementation of email, sms and analytics!
func (s *OrderConfirmationService) ConfirmOrder(ctx context.Context, orderID, customerEmail string) error {
	order, err := s.Orders.FindByID(ctx, orderID)
	if err != nil {
		return err
	}
	if err := order.Confirm(); err != nil {
		return err
	}
	if err := s.Orders.Save(ctx, order); err != nil {
		return err
	}
	//Notify the client - WE DON’T KNOW how, we only know WHAT
	_ = s.Notifier.NotifyOrderConfirmed(customerEmail, order.ID, order.Amount)
	//Notify warehouse
	_ = s.Warehouse.NotifyNewOrder(order.ID, []string{"item-1"})
	//Analytics - fire-and-forget
	s.Analytics.TrackOrderConfirmed(order.ID, order.CustomerID, order.Amount)
	return nil
}

// ─────────────────────────────────────────────────────────────────
//STEP 3: STUBS - minimal implementations for development.
//They do nothing, but allow the system to work and be tested.
// ─────────────────────────────────────────────────────────────────

//StubNotifier - stub: does not send anything, always successful.
type StubNotifier struct{}

func (s *StubNotifier) NotifyOrderConfirmed(_, _ string, _ float64) error { return nil }

//StubWarehouseNotifier - stub for a warehouse.
type StubWarehouseNotifier struct{}

func (s *StubWarehouseNotifier) NotifyNewOrder(_ string, _ []string) error { return nil }

//StubAnalyticsTracker is a stub for analytics.
type StubAnalyticsTracker struct{}

func (s *StubAnalyticsTracker) TrackOrderConfirmed(_, _ string, _ float64) {}

// ─────────────────────────────────────────────────────────────────
//STEP 4: SPY (Spy) - remembers calls for verification in tests.
//"Did we call NotifyOrderConfirmed after confirmation?"
// ─────────────────────────────────────────────────────────────────

//SpyNotifier - remembers all calls.
type SpyNotifier struct {
	Calls []NotifyCall
}

//NotifyCall - a record of one call.
type NotifyCall struct {
	Email   string
	OrderID string
	Amount  float64
}

func (s *SpyNotifier) NotifyOrderConfirmed(email, orderID string, amount float64) error {
	s.Calls = append(s.Calls, NotifyCall{Email: email, OrderID: orderID, Amount: amount})
	return nil
}

func (s *SpyNotifier) Called() bool   { return len(s.Calls) > 0 }
func (s *SpyNotifier) CallCount() int { return len(s.Calls) }

//SpyAnalyticsTracker - spy for analytics.
type SpyAnalyticsTracker struct {
	TrackedEvents []string
}

func (s *SpyAnalyticsTracker) TrackOrderConfirmed(orderID, _ string, _ float64) {
	s.TrackedEvents = append(s.TrackedEvents, orderID)
}

// ─────────────────────────────────────────────────────────────────
//STEP 5: REAL IMPLEMENTATIONS - when providers have been chosen.
//We add them without changing the service, tests and other implementations.
// ─────────────────────────────────────────────────────────────────

//SendGridNotifier - implementation via SendGrid.
type SendGridNotifier struct {
	APIKey string
}

func (n *SendGridNotifier) NotifyOrderConfirmed(email, orderID string, amount float64) error {
	//In reality: HTTP request to SendGrid API
	_, _, _ = email, orderID, amount
	return nil
}

//SMSWarehouseNotifier - implementation via Twilio.
type SMSWarehouseNotifier struct {
	PhoneNumber string
}

func (n *SMSWarehouseNotifier) NotifyNewOrder(orderID string, items []string) error {
	_, _ = orderID, items
	return nil
}

// =================================================================
//SCENARIO 2: interface for different report export formats.
//
//Task: export orders to CSV, JSON, Excel, PDF.
//Without interfaces - a huge switch. With interfaces - each
//format is independent, add new = create new structure.
// =================================================================

//ReportExporter - can export data to a specific format.
type ReportExporter interface {
	Export(orders []*Order) ([]byte, error)
	ContentType() string
	FileExtension() string
}

//ReportService - generates reports. The format is connected externally.
type ReportService struct {
	Orders   OrderRepository
	Exporter ReportExporter
}

//GenerateReport - the logic is the same, the format is external.
func (s *ReportService) GenerateReport(ctx context.Context) (data []byte, contentType, filename string, err error) {
	orders, err := s.Orders.FindAll(ctx)
	if err != nil {
		return nil, "", "", err
	}
	data, err = s.Exporter.Export(orders)
	if err != nil {
		return nil, "", "", err
	}
	return data, s.Exporter.ContentType(), "report." + s.Exporter.FileExtension(), nil
}

//CSVExporter - export to CSV.
type CSVExporter struct{}

func (e *CSVExporter) Export(orders []*Order) ([]byte, error) {
	result := "id,customer_id,amount,status\n"
	for _, o := range orders {
		result += o.ID + "," + o.CustomerID + "," +
			fmt.Sprintf("%.2f", o.Amount) + "," + string(o.Status) + "\n"
	}
	return []byte(result), nil
}
func (e *CSVExporter) ContentType() string   { return "text/csv" }
func (e *CSVExporter) FileExtension() string { return "csv" }

//JSONExporter - export to JSON.
type JSONExporter struct{}

func (e *JSONExporter) Export(orders []*Order) ([]byte, error) {
	result := "[\n"
	for i, o := range orders {
		result += `  {"id":"` + o.ID + `","amount":` + fmt.Sprintf("%.2f", o.Amount) + "}"
		if i < len(orders)-1 {
			result += ","
		}
		result += "\n"
	}
	result += "]"
	return []byte(result), nil
}
func (e *JSONExporter) ContentType() string   { return "application/json" }
func (e *JSONExporter) FileExtension() string { return "json" }

// =================================================================
//SCENARIO 3: Hexagonal Architecture - ports and adapters.
//
//External world Adapter Port (interface) Core
//   ────────────     ───────────      ────────────────   ──────
//   PostgreSQL   →   PgOrderRepo  →   OrderRepository →  Service
//   Redis        →   RedisCache   →   CacheStore      →  Service
//   Kafka        →   KafkaAdapter →   EventPublisher  →  Service
// =================================================================

//CacheStore - cache port. Redis or Memcached - we don't care.
type CacheStore interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte)
	Delete(key string)
}

//EventPublisher - port for events. Kafka, RabbitMQ, in-memory.
type EventPublisher interface {
	Publish(topic string, payload []byte) error
}

//CachedOrderRepository - Decorator: Caches on top of any repository.
//Implements OrderRepository - the service does not know about the cache!
type CachedOrderRepository struct {
	Repo  OrderRepository
	Cache CacheStore
}

func (r *CachedOrderRepository) FindByID(ctx context.Context, id string) (*Order, error) {
	if data, ok := r.Cache.Get("order:" + id); ok {
		_ = data //in real life: json.Unmarshal → return order
	}
	order, err := r.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	r.Cache.Set("order:"+id, []byte(order.ID))
	return order, nil
}

func (r *CachedOrderRepository) Save(ctx context.Context, order *Order) error {
	r.Cache.Delete("order:" + order.ID) //invalidate the cache
	return r.Repo.Save(ctx, order)
}

func (r *CachedOrderRepository) FindAll(ctx context.Context) ([]*Order, error) {
	return r.Repo.FindAll(ctx)
}
func (r *CachedOrderRepository) Delete(ctx context.Context, id string) error {
	r.Cache.Delete("order:" + id)
	return r.Repo.Delete(ctx, id)
}

//InMemoryCacheStore - CacheStore implementation for tests.
type InMemoryCacheStore struct {
	data map[string][]byte
}

func NewInMemoryCacheStore() *InMemoryCacheStore {
	return &InMemoryCacheStore{data: make(map[string][]byte)}
}
func (c *InMemoryCacheStore) Get(key string) ([]byte, bool) { v, ok := c.data[key]; return v, ok }
func (c *InMemoryCacheStore) Set(key string, value []byte)  { c.data[key] = value }
func (c *InMemoryCacheStore) Delete(key string)             { delete(c.data, key) }

// ─────────────────────────────────────────────────────────────────
//Compile-time checks: all types implement their own interfaces.
// ─────────────────────────────────────────────────────────────────

var (
	_ Notifier          = (*StubNotifier)(nil)
	_ Notifier          = (*SpyNotifier)(nil)
	_ Notifier          = (*SendGridNotifier)(nil)
	_ WarehouseNotifier = (*StubWarehouseNotifier)(nil)
	_ WarehouseNotifier = (*SMSWarehouseNotifier)(nil)
	_ AnalyticsTracker  = (*StubAnalyticsTracker)(nil)
	_ AnalyticsTracker  = (*SpyAnalyticsTracker)(nil)
	_ ReportExporter    = (*CSVExporter)(nil)
	_ ReportExporter    = (*JSONExporter)(nil)
	_ OrderRepository   = (*CachedOrderRepository)(nil)
	_ CacheStore        = (*InMemoryCacheStore)(nil)
)

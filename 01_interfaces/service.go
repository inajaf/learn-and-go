package interfaces

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ─────────────────────────────────────────────────────────────────
// SERVICE LAYER
//
//The service contains BUSINESS LOGIC.
//He doesn’t know about HTTP, gRPC, database - only about business rules.
//All dependencies are transferred through interfaces (Dependency Injection).
// ─────────────────────────────────────────────────────────────────

//OrderService is a business logic layer.
//👉 Depends on the OrderRepository interface, not on the specific implementation.
//
//This allows you to change the repository in tests without changing the service.
//
//Configurable via Functional Options - see options.go.
type OrderService struct {
	repo   OrderRepository //👈 interface, not a specific type!
	logger *slog.Logger    //optional - configured via WithLogger
	pub    EventPublisher  //optional - configured via WithEventPublisher
}

//NewOrderService is defined in options.go via the Functional Options pattern.

//CreateOrder creates a new order.
//Business rules: amount must be > 0, customerID not empty.
//
//👉 context.Context is the first parameter. This is standard Go convention.
//
//Transmitted via ctx: deadline (timeout), cancellation (cancellation),
//trace_id (trace), request_id and other values.
func (s *OrderService) CreateOrder(ctx context.Context, customerID string, amount float64) (*Order, error) {
	//Business Validation
	if customerID == "" {
		return nil, fmt.Errorf("CreateOrder: customerID is required")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("CreateOrder: amount must be positive, got %.2f", amount)
	}

	order := &Order{
		ID:         generateID(),
		CustomerID: customerID,
		Amount:     amount,
		Status:     OrderStatusPending,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	//We save through the interface - the service does not know where exactly
	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("CreateOrder: failed to save: %w", err)
	}

	return order, nil
}

//GetOrder returns an order by ID.
func (s *OrderService) GetOrder(ctx context.Context, id string) (*Order, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("GetOrder: %w", err)
	}
	return order, nil
}

//ConfirmOrder confirms the order.
func (s *OrderService) ConfirmOrder(ctx context.Context, id string) (*Order, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ConfirmOrder: %w", err)
	}

	//Business method on a domain model - logic inside Order, not in the service
	if err := order.Confirm(); err != nil {
		return nil, fmt.Errorf("ConfirmOrder: %w", err)
	}

	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("ConfirmOrder: failed to save: %w", err)
	}

	return order, nil
}

//CancelOrder cancels the order.
func (s *OrderService) CancelOrder(ctx context.Context, id string) (*Order, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("CancelOrder: %w", err)
	}

	if err := order.Cancel(); err != nil {
		return nil, fmt.Errorf("CancelOrder: %w", err)
	}

	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("CancelOrder: failed to save: %w", err)
	}

	return order, nil
}

//ListOrders returns all orders.
func (s *OrderService) ListOrders(ctx context.Context) ([]*Order, error) {
	orders, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListOrders: %w", err)
	}
	return orders, nil
}

//generateID generates a unique ID based on time.
//👉 In a real project, use uuid.New().String() from github.com/google/uuid
func generateID() string {
	return fmt.Sprintf("order-%d", time.Now().UnixNano())
}

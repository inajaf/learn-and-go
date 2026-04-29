// Package unittest demonstrates unit testing with mocks.
//
// This service has TWO dependencies:
//   - OrderRepository: the order store
//   - EventPublisher: for publishing events
//
// Both dependencies are interfaces. In tests we swap them out.
package unittest

import (
	"errors"
	"fmt"
	"time"
)

// OrderStatus — an order status.
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusCancelled OrderStatus = "cancelled"
)

// ErrNotFound — order not found.
var ErrNotFound = errors.New("order not found")

// Order — domain model.
type Order struct {
	ID         string
	CustomerID string
	Amount     float64
	Status     OrderStatus
	CreatedAt  time.Time
}

// ─────────────────────────────────────────────────────────────────
// DEPENDENCY INTERFACES
// ─────────────────────────────────────────────────────────────────

// OrderRepository — interface for working with orders.
type OrderRepository interface {
	Save(order *Order) error
	FindByID(id string) (*Order, error)
}

// EventPublisher — interface for publishing events.
type EventPublisher interface {
	Publish(eventType string, payload any) error
}

// ─────────────────────────────────────────────────────────────────
// SERVICE
// ─────────────────────────────────────────────────────────────────

// OrderService — a service with two dependencies behind interfaces.
type OrderService struct {
	repo      OrderRepository // 👉 interface — not a concrete type
	publisher EventPublisher  // 👉 interface — easy to mock
}

// NewOrderService — constructor.
func NewOrderService(repo OrderRepository, publisher EventPublisher) *OrderService {
	return &OrderService{repo: repo, publisher: publisher}
}

// PlaceOrder creates an order and publishes an event.
func (s *OrderService) PlaceOrder(customerID string, amount float64) (*Order, error) {
	if customerID == "" {
		return nil, fmt.Errorf("PlaceOrder: customerID is required")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("PlaceOrder: amount must be positive")
	}

	order := &Order{
		ID:         fmt.Sprintf("ord-%d", time.Now().UnixNano()),
		CustomerID: customerID,
		Amount:     amount,
		Status:     StatusPending,
		CreatedAt:  time.Now(),
	}

	// Step 1: Save to the repository
	if err := s.repo.Save(order); err != nil {
		return nil, fmt.Errorf("PlaceOrder: failed to save: %w", err)
	}

	// Step 2: Publish the event
	// 👉 If the publisher fails — the order is already saved, we just log the error.
	//    This is intentional: the event is a secondary operation.
	if err := s.publisher.Publish("order.placed", map[string]any{
		"order_id":    order.ID,
		"customer_id": customerID,
		"amount":      amount,
	}); err != nil {
		// In production: log it, but don't return an error
		fmt.Printf("warning: failed to publish event: %v\n", err)
	}

	return order, nil
}

// GetOrder returns an order by ID.
func (s *OrderService) GetOrder(id string) (*Order, error) {
	order, err := s.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("GetOrder: %w", err)
	}
	return order, nil
}

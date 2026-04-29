//Package database demonstrates working with a real PostgreSQL database.
//
//Stack:
//- PostgreSQL 15 (via Docker Compose)
//- sqlx - a thin wrapper over database/sql with convenient row scanning
//- lib/pq - PostgreSQL driver
//- golang-migrate - rollback/rollback of SQL migrations
//
//Package architecture:
//
//domain.go - domain models (without database tags)
//repository/ — repository implementation (SQL queries)
//service/ - business logic (via the repository interface)
package database

import (
	"errors"
	"time"
)

// ═══════════════════════════════════════════════════════════════
//DOMAIN - pure business models. No db/json tags.
//👉Module 1: Domain Objects with Business Methods
// ═══════════════════════════════════════════════════════════════

var (
	ErrOrderNotFound    = errors.New("order not found")
	ErrCustomerNotFound = errors.New("customer not found")
	ErrInvalidInput     = errors.New("invalid input")
	ErrVersionConflict  = errors.New("optimistic lock: version conflict")
)

//OrderStatus - custom status type (Value Object).
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusCancelled OrderStatus = "cancelled"
)

func (s OrderStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusConfirmed, StatusCancelled:
		return true
	}
	return false
}

//Customer—customer domain model.
type Customer struct {
	ID        string
	Name      string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

//Order - domain order model.
type Order struct {
	ID          string
	CustomerID  string
	Status      OrderStatus
	TotalAmount float64
	Notes       string
	Items       []OrderItem
	Version     int // optimistic locking
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

//OrderItem - order item.
type OrderItem struct {
	ID        string
	OrderID   string
	ProductID string
	Name      string
	Quantity  int
	UnitPrice float64
}

//Total recalculates the order amount from the positions.
func (o *Order) Total() float64 {
	var sum float64
	for _, item := range o.Items {
		sum += float64(item.Quantity) * item.UnitPrice
	}
	return sum
}

//Confirm confirms the order.
func (o *Order) Confirm() error {
	if o.Status != StatusPending {
		return errors.New("only pending orders can be confirmed")
	}
	o.Status = StatusConfirmed
	o.UpdatedAt = time.Now()
	return nil
}

//Cancel cancels the order.
func (o *Order) Cancel() error {
	if o.Status == StatusCancelled {
		return errors.New("order already cancelled")
	}
	o.Status = StatusCancelled
	o.UpdatedAt = time.Now()
	return nil
}

// ═══════════════════════════════════════════════════════════════
//INTERFACES - repository contracts
//
//👉 Module 1: the interface is defined by the consumer (service),
//not a producer (postgres implementation).
// ═══════════════════════════════════════════════════════════════

//CustomerRepository - customer repository contract.
type CustomerRepository interface {
	Create(customer *Customer) error
	FindByID(id string) (*Customer, error)
	FindByEmail(email string) (*Customer, error)
	List() ([]*Customer, error)
}

//OrderRepository - order storage contract.
type OrderRepository interface {
	//Create saves the new order along with the items (in a transaction).
	Create(order *Order) error

	//FindByID returns the order with items.
	FindByID(id string) (*Order, error)

	//FindByCustomerID returns all customer orders.
	FindByCustomerID(customerID string) ([]*Order, error)

	//FindByStatus returns orders with the given status.
	FindByStatus(status OrderStatus) ([]*Order, error)

	//Update updates the order status/notes.
	//Uses optimistic locking - updates only if the version matches.
	Update(order *Order) error

	//SoftDelete marks the order as deleted (deleted_at = NOW()).
	//Physically, the row remains in the database.
	SoftDelete(id string) error

	//ListWithItems returns all orders with their items (JOIN).
	ListWithItems() ([]*Order, error)
}

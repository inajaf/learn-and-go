//Package interfaces demonstrates the use of domain models.
//The domain model is a “pure” business entity.
//She does NOT know about the database, HTTP, gRPC or any other transport.
package interfaces

import (
	"fmt"
	"time"
)

//OrderStatus - type for order status.
//👉 We use a custom type instead of string - the compiler will protect against errors.
//👉 This is the “Value Object” pattern - a primitive with business meaning.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusCancelled OrderStatus = "cancelled"
)

//IsValid checks that the status is a valid value.
//👉 Method on Value Object - business logic next to the data.
func (s OrderStatus) IsValid() bool {
	switch s {
	case OrderStatusPending, OrderStatusConfirmed, OrderStatusCancelled:
		return true
	}
	return false
}

//Money — Value Object for monetary values.
//👉 Value Object = type with business meaning + validation.
//
//You cannot create invalid money - the designer will protect it.
type Money float64

//NewMoney creates a monetary value with validation.
func NewMoney(amount float64) (Money, error) {
	if amount < 0 {
		return 0, fmt.Errorf("money cannot be negative: %.2f", amount)
	}
	return Money(amount), nil
}

//Add adds two sums.
func (m Money) Add(other Money) Money { return m + other }

//Float64 returns float64 - for storing in the database or sending in a response.
func (m Money) Float64() float64 { return float64(m) }

//Order - domain order model.
//👉 This is the “truth” about ordering from a business point of view.
//
//It does not contain `json`, `db` or anything similar tags.
//Clean data structure + business behavior.
type Order struct {
	ID         string
	CustomerID string
	Amount     float64
	Status     OrderStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

//IsActive checks that the order has not been cancelled.
//👉 Business logic lives in the domain model, and is not scattered across services.
func (o *Order) IsActive() bool {
	return o.Status != OrderStatusCancelled
}

//IsPending checks that the order is pending processing.
func (o *Order) IsPending() bool {
	return o.Status == OrderStatusPending
}

//Cancel cancels the order if possible.
//Returns an error if already cancelled.
func (o *Order) Cancel() error {
	if o.Status == OrderStatusCancelled {
		return ErrOrderAlreadyCancelled
	}
	o.Status = OrderStatusCancelled
	o.UpdatedAt = time.Now()
	return nil
}

//Confirm confirms the order in Pending status.
func (o *Order) Confirm() error {
	if o.Status != OrderStatusPending {
		return ErrOrderCannotBeConfirmed
	}
	o.Status = OrderStatusConfirmed
	o.UpdatedAt = time.Now()
	return nil
}

//CanTransitionTo checks the validity of a status transition.
//👉 State Machine - a pattern for state management.
//
//All transitions are described in one place - easy to read and change.
//
//Valid transitions:
//		pending → confirmed
//		pending → cancelled
//		confirmed → cancelled
func (o *Order) CanTransitionTo(next OrderStatus) bool {
	transitions := map[OrderStatus][]OrderStatus{
		OrderStatusPending:   {OrderStatusConfirmed, OrderStatusCancelled},
		OrderStatusConfirmed: {OrderStatusCancelled},
		OrderStatusCancelled: {}, //final status
	}
	for _, allowed := range transitions[o.Status] {
		if allowed == next {
			return true
		}
	}
	return false
}

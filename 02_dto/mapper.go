package dto

import (
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────
// MAPPER — functions for converting between data layers.
//
// A mapper is a thin layer that translates data from one
// format to another. There's no business logic here — only
// field translation.
//
// Pattern: each function is named To<Target>From<Source>
// ─────────────────────────────────────────────────────────────────

// RequestToDomain converts an HTTP DTO into a domain model.
// 👉 Called at the service boundary — the service only works with domain models.
func RequestToDomain(req CreateOrderRequest) OrderDomain {
	return OrderDomain{
		// ID — not set here, the service will generate it
		CustomerID: req.CustomerID,
		Amount:     req.Amount,
		Status:     OrderStatusPending, // 👉 Initial status — a business rule
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// DomainToResponse converts a domain model into an HTTP response.
// 👉 Called on the way out — right before sending to the client.
func DomainToResponse(order OrderDomain) OrderResponse {
	return OrderResponse{
		ID:         order.ID,
		CustomerID: order.CustomerID,
		Amount:     order.Amount,
		Status:     string(order.Status),
		// 👉 Format time as a string — easier for the client to read "2024-01-15 10:30:00"
		CreatedAt: order.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// DomainToDB converts a domain model into a DB row.
// 👉 Called in the repository before INSERT/UPDATE.
func DomainToDB(order OrderDomain) OrderRow {
	return OrderRow{
		ID:         order.ID,
		CustomerID: order.CustomerID,
		Amount:     order.Amount,
		Status:     string(order.Status), // 👉 OrderStatus → string for the DB
		CreatedAt:  order.CreatedAt,
		UpdatedAt:  order.UpdatedAt,
		DeletedAt:  nil,
		Version:    1,
	}
}

// DBToDomain converts a DB row into a domain model.
// 👉 Called in the repository after SELECT.
//
//	May return an error if the data in the DB is invalid.
func DBToDomain(row OrderRow) (OrderDomain, error) {
	status := OrderStatus(row.Status)

	// Validate that the status is one of the allowed values
	switch status {
	case OrderStatusPending, OrderStatusConfirmed, OrderStatusCancelled:
		// OK
	default:
		return OrderDomain{}, fmt.Errorf("unknown order status in DB: %q", row.Status)
	}

	return OrderDomain{
		ID:         row.ID,
		CustomerID: row.CustomerID,
		Amount:     row.Amount,
		Status:     status,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}, nil
}

// ApplyUpdateRequest applies the changes from the request to the domain model.
// 👉 Partial update pattern: only update the fields that were passed.
//
//	If a field is nil — leave it alone (the client didn't send it).
func ApplyUpdateRequest(order *OrderDomain, req UpdateOrderRequest) {
	if req.Amount != nil {
		order.Amount = *req.Amount // 👉 dereference the pointer
	}
	order.UpdatedAt = time.Now()
}

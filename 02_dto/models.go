// Package dto demonstrates splitting data into three layers:
// Transport (DTO), Domain (business model), Persistence (DB model).
package dto

import "time"

// ═══════════════════════════════════════════════════════════════
// LAYER 1: TRANSPORT (DTO)
// Structs that come from the client and go back to the client.
// json: tags define the field names in JSON.
// ═══════════════════════════════════════════════════════════════

// CreateOrderRequest — what the client sends when creating an order.
// 👉 Only the fields the client needs. ID, CreatedAt etc. — the server assigns them itself.
type CreateOrderRequest struct {
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	// 👉 No Status field — the client must not set it
}

// UpdateOrderRequest — what the client sends when updating.
type UpdateOrderRequest struct {
	Amount *float64 `json:"amount,omitempty"` // 👉 pointer + omitempty: field is optional
}

// OrderResponse — what the server returns to the client.
// 👉 No internal fields (InternalNotes, CostPrice, etc.)
//
//	The client should see only what they need.
type OrderResponse struct {
	ID         string  `json:"id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"` // 👉 String, not time.Time — more convenient for API
}

// ═══════════════════════════════════════════════════════════════
// LAYER 2: DOMAIN (business model)
// A clean business entity with no tags. Lives in the service layer.
// ═══════════════════════════════════════════════════════════════

// OrderStatus — custom type for the status.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// OrderDomain — the domain model.
// 👉 NO json or db tags. This is the "clean truth" about an order.
type OrderDomain struct {
	ID         string
	CustomerID string
	Amount     float64
	Status     OrderStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ═══════════════════════════════════════════════════════════════
// LAYER 3: PERSISTENCE (DB model)
// Struct that matches a row in a database table.
// db: tags — for libraries like sqlx.
// ═══════════════════════════════════════════════════════════════

// OrderRow — DB model, corresponds to a row in the `orders` table.
// 👉 Fields may differ from the domain model:
//   - snake_case instead of CamelCase
//   - May contain fields for soft delete (DeletedAt)
//   - May contain technical fields (Version for optimistic locking)
type OrderRow struct {
	ID         string     `db:"id"`
	CustomerID string     `db:"customer_id"`
	Amount     float64    `db:"amount"`
	Status     string     `db:"status"` // 👉 string in the DB, OrderStatus in the domain
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"`
	DeletedAt  *time.Time `db:"deleted_at"` // 👉 pointer: NULL in DB = nil in Go
	Version    int        `db:"version"`    // for optimistic locking
}

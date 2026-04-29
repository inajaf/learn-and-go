package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "learning_path/02_dto"
)

// TestRequestToDomain — verify that a DTO converts into a domain model.
func TestRequestToDomain(t *testing.T) {
	req := dto.CreateOrderRequest{
		CustomerID: "cust-42",
		Amount:     199.99,
	}

	domain := dto.RequestToDomain(req)

	assert.Equal(t, "cust-42", domain.CustomerID)
	assert.Equal(t, 199.99, domain.Amount)
	// 👉 The initial status is always Pending — a business rule baked into the mapper
	assert.Equal(t, dto.OrderStatusPending, domain.Status)
	// ID is not yet assigned — the service will do it later
	assert.Empty(t, domain.ID)
}

// TestDomainToResponse — verify conversion to an HTTP response.
func TestDomainToResponse(t *testing.T) {
	domain := dto.OrderDomain{
		ID:         "order-123",
		CustomerID: "cust-42",
		Amount:     199.99,
		Status:     dto.OrderStatusConfirmed,
		CreatedAt:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	response := dto.DomainToResponse(domain)

	assert.Equal(t, "order-123", response.ID)
	assert.Equal(t, "cust-42", response.CustomerID)
	assert.Equal(t, 199.99, response.Amount)
	assert.Equal(t, "confirmed", response.Status)
	// 👉 Time is formatted as a string
	assert.Equal(t, "2024-01-15 10:30:00", response.CreatedAt)
}

// TestRoundTrip — verify that Domain → DB → Domain preserves the data.
func TestRoundTrip(t *testing.T) {
	original := dto.OrderDomain{
		ID:         "order-456",
		CustomerID: "cust-99",
		Amount:     500.00,
		Status:     dto.OrderStatusPending,
		CreatedAt:  time.Now().Truncate(time.Second),
		UpdatedAt:  time.Now().Truncate(time.Second),
	}

	// Domain → DB
	row := dto.DomainToDB(original)
	assert.Equal(t, "pending", row.Status) // string in the DB

	// DB → Domain
	restored, err := dto.DBToDomain(row)
	require.NoError(t, err)

	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.CustomerID, restored.CustomerID)
	assert.Equal(t, original.Amount, restored.Amount)
	assert.Equal(t, original.Status, restored.Status) // OrderStatus again
}

// TestDBToDomain_InvalidStatus — an unknown status in the DB returns an error.
func TestDBToDomain_InvalidStatus(t *testing.T) {
	row := dto.OrderRow{
		ID:     "order-999",
		Status: "unknown_status", // 👉 invalid value coming from the DB
	}

	_, err := dto.DBToDomain(row)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_status")
}

// TestApplyUpdateRequest — verify partial update.
func TestApplyUpdateRequest(t *testing.T) {
	order := dto.OrderDomain{Amount: 100.0}

	newAmount := 250.0
	req := dto.UpdateOrderRequest{Amount: &newAmount}

	dto.ApplyUpdateRequest(&order, req)

	assert.Equal(t, 250.0, order.Amount)
}

// TestApplyUpdateRequest_NilAmount — if amount wasn't sent, we don't change it.
func TestApplyUpdateRequest_NilAmount(t *testing.T) {
	order := dto.OrderDomain{Amount: 100.0}
	req := dto.UpdateOrderRequest{Amount: nil} // field not sent

	dto.ApplyUpdateRequest(&order, req)

	assert.Equal(t, 100.0, order.Amount) // unchanged
}

package interfaces_test

//👉 Please note: the package is called interfaces_TEST (with the suffix _test).
//This is a "black-box" test - we only test the public API of the package.
//This approach is better: it checks what the user of the package sees.

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	interfaces "learning_path/01_interfaces"
)

// ─────────────────────────────────────────────────────────────────
//OrderService tests
//
//In each test we create a REAL InMemoryRepository.
//This is NOT a mock - this is a real implementation in memory.
//We will study mocks in module 05_unit_testing.
// ─────────────────────────────────────────────────────────────────

//TestOrderService_CreateOrder - test for successful order creation.
func TestOrderService_CreateOrder(t *testing.T) {
	//Arrange - preparation
	ctx := context.Background()
	repo := interfaces.NewInMemoryOrderRepository()
	svc := interfaces.NewOrderService(repo)

	//Act - action
	order, err := svc.CreateOrder(ctx, "customer-1", 100.50)

	//Assert - check
	//👉 require.NoError stops the test if there is an error, there is no point in continuing
	require.NoError(t, err)
	//👉 assert.* continues the test even if there is an error - you can collect all the problems
	assert.NotEmpty(t, order.ID)
	assert.Equal(t, "customer-1", order.CustomerID)
	assert.Equal(t, 100.50, order.Amount)
	assert.Equal(t, interfaces.OrderStatusPending, order.Status)
}

//TestOrderService_CreateOrder_Validation - validation test during creation.
func TestOrderService_CreateOrder_Validation(t *testing.T) {
	ctx := context.Background()
	repo := interfaces.NewInMemoryOrderRepository()
	svc := interfaces.NewOrderService(repo)

	//👉 Table-driven tests - Go idiom.
	//One logic, many inputs. Convenient and readable.
	tests := []struct {
		name       string
		customerID string
		amount     float64
		wantErr    bool
	}{
		{
			name:       "empty customerID",
			customerID: "",
			amount:     100,
			wantErr:    true,
		},
		{
			name:       "zero amount",
			customerID: "cust-1",
			amount:     0,
			wantErr:    true,
		},
		{
			name:       "negative amount",
			customerID: "cust-1",
			amount:     -50,
			wantErr:    true,
		},
		{
			name:       "valid order",
			customerID: "cust-1",
			amount:     1,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateOrder(ctx, tt.customerID, tt.amount)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//TestOrderService_ConfirmOrder - order confirmation test.
func TestOrderService_ConfirmOrder(t *testing.T) {
	ctx := context.Background()
	repo := interfaces.NewInMemoryOrderRepository()
	svc := interfaces.NewOrderService(repo)

	//Create an order
	order, err := svc.CreateOrder(ctx, "cust-1", 50)
	require.NoError(t, err)
	require.Equal(t, interfaces.OrderStatusPending, order.Status)

	//Confirm
	confirmed, err := svc.ConfirmOrder(ctx, order.ID)
	require.NoError(t, err)
	assert.Equal(t, interfaces.OrderStatusConfirmed, confirmed.Status)
}

//TestOrderService_CancelOrder - test for canceling an already canceled order.
func TestOrderService_CancelOrder_AlreadyCancelled(t *testing.T) {
	ctx := context.Background()
	repo := interfaces.NewInMemoryOrderRepository()
	svc := interfaces.NewOrderService(repo)

	order, err := svc.CreateOrder(ctx, "cust-1", 75)
	require.NoError(t, err)

	//First cancellation - OK
	_, err = svc.CancelOrder(ctx, order.ID)
	require.NoError(t, err)

	//Second cancel - should return an error
	_, err = svc.CancelOrder(ctx, order.ID)
	require.Error(t, err)

	//👉 errors.Is checking the sentinel error in the error chain.
	//This works even if the error is wrapped with fmt.Errorf("...: %w", err)
	assert.True(t, errors.Is(err, interfaces.ErrOrderAlreadyCancelled))
}

//TestOrderService_GetOrder_NotFound - test for receiving a non-existent order.
func TestOrderService_GetOrder_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := interfaces.NewInMemoryOrderRepository()
	svc := interfaces.NewOrderService(repo)

	_, err := svc.GetOrder(ctx, "non-existent-id")

	require.Error(t, err)
	assert.True(t, errors.Is(err, interfaces.ErrOrderNotFound))
}

//TestDecoratorPattern - demonstration of the Decorator pattern.
func TestDecoratorPattern(t *testing.T) {
	ctx := context.Background()
	//👉 Create a chain: LoggingRepo → InMemoryRepo
	//The service does not know that there is a logger between it and the storage.
	inner := interfaces.NewInMemoryOrderRepository()

	//👉 We use io.Discard as a writer - the logs go to nowhere (ideal for tests)
	logger := log.New(io.Discard, "[repo] ", log.LstdFlags)
	decorated := interfaces.NewLoggingRepository(inner, logger)

	//LoggingRepository also implements OrderRepository - so we pass it to the service
	svc := interfaces.NewOrderService(decorated)

	order, err := svc.CreateOrder(ctx, "cust-1", 200)
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)
}

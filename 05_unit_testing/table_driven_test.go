package unittest_test

// =============================================================================
// Modern Testing Patterns — patterns used in production projects
// =============================================================================

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "learning_path/05_unit_testing"
)

// =============================================================================
// Table-Driven Tests — THE core testing pattern in Go
// =============================================================================
//
// Advantages:
//   - All cases in one place — easy to see what's being tested
//   - Easy to add a new case — one line
//   - t.Run creates a named sub-test — the report shows what failed
//   - DRY — the test logic is written once

func TestPlaceOrder_TableDriven(t *testing.T) {
	tests := []struct {
		name       string // Case name (shown in t.Run)
		customerID string
		amount     float64
		wantErr    bool   // Expect an error?
		errMsg     string // Substring in the error message (if wantErr=true)
	}{
		{
			name:       "successful creation",
			customerID: "cust-1",
			amount:     100.0,
			wantErr:    false,
		},
		{
			name:       "empty customerID",
			customerID: "",
			amount:     100.0,
			wantErr:    true,
			errMsg:     "customer",
		},
		{
			name:       "zero amount",
			customerID: "cust-1",
			amount:     0,
			wantErr:    true,
			errMsg:     "amount",
		},
		{
			name:       "negative amount",
			customerID: "cust-1",
			amount:     -50.0,
			wantErr:    true,
			errMsg:     "amount",
		},
		{
			name:       "large amount",
			customerID: "cust-1",
			amount:     999_999.99,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 👉 t.Parallel() — runs sub-tests in parallel
			// Safe when each test uses its own data (no shared state)
			t.Parallel()

			repo := &ManualMockRepository{}
			pub := &ManualMockPublisher{}
			svc := NewOrderService(repo, pub)

			order, err := svc.PlaceOrder(tt.customerID, tt.amount)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, order.ID)
			assert.Equal(t, tt.customerID, order.CustomerID)
			assert.Equal(t, tt.amount, order.Amount)
		})
	}
}

// =============================================================================
// t.Helper() — clean assertion helpers
// =============================================================================
//
// 👉 t.Helper() marks the function as a helper.
//    On failure it points to the CALL site rather than the line inside the helper.
//
// Without t.Helper():
//   service_test.go:15: failed     ← line in the helper (useless)
//
// With t.Helper():
//   service_test.go:42: failed     ← line in the test (useful!)

// assertOrderValid — helper that checks an order.
func assertOrderValid(t *testing.T, order *Order) {
	t.Helper() // 👉 Required!
	require.NotNil(t, order)
	assert.NotEmpty(t, order.ID, "order ID must not be empty")
	assert.NotEmpty(t, order.CustomerID, "customer ID must not be empty")
	assert.Greater(t, order.Amount, 0.0, "amount must be > 0")
	assert.Equal(t, StatusPending, order.Status, "status must be pending")
}

func TestPlaceOrder_WithHelper(t *testing.T) {
	repo := &ManualMockRepository{}
	pub := &ManualMockPublisher{}
	svc := NewOrderService(repo, pub)

	order, err := svc.PlaceOrder("cust-1", 200.0)
	require.NoError(t, err)

	assertOrderValid(t, order)
}

// =============================================================================
// t.Cleanup() — deferred cleanup
// =============================================================================
//
// 👉 t.Cleanup registers a function that runs AFTER the test.
//    Unlike defer — it runs even when the test is aborted.
//    Handy for: closing the DB, stopping servers, removing files.

func TestWithCleanup(t *testing.T) {
	// Simulated resource (in reality: a docker container, a temp file)
	var resourceClosed bool

	t.Cleanup(func() {
		// 👉 Runs after the test (even on panic)
		resourceClosed = true
	})

	repo := &ManualMockRepository{}
	pub := &ManualMockPublisher{}
	svc := NewOrderService(repo, pub)

	order, err := svc.PlaceOrder("cust-1", 100.0)
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)

	// At the moment the test runs, cleanup has not fired yet
	assert.False(t, resourceClosed)
}

// =============================================================================
// Subtests for organizing tests
// =============================================================================
//
// t.Run creates a named sub-test. That lets you:
//   - Logically group tests
//   - Run one case: go test -run TestOrderService/create/success
//   - See the hierarchy in reports: TestOrderService/create/success ✓

func TestOrderService_Grouped(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			repo := &ManualMockRepository{}
			pub := &ManualMockPublisher{}
			svc := NewOrderService(repo, pub)

			order, err := svc.PlaceOrder("cust-1", 100.0)
			require.NoError(t, err)
			assert.NotEmpty(t, order.ID)
		})

		t.Run("validation error", func(t *testing.T) {
			repo := &ManualMockRepository{}
			pub := &ManualMockPublisher{}
			svc := NewOrderService(repo, pub)

			_, err := svc.PlaceOrder("", 100.0)
			require.Error(t, err)
		})
	})

	t.Run("get", func(t *testing.T) {
		t.Run("existing order", func(t *testing.T) {
			repo := &ManualMockRepository{
				FindByIDFunc: func(id string) (*Order, error) {
					return &Order{ID: id, CustomerID: "cust-1", Amount: 50.0}, nil
				},
			}
			pub := &ManualMockPublisher{}
			svc := NewOrderService(repo, pub)

			order, err := svc.GetOrder("ord-1")
			require.NoError(t, err)
			assert.Equal(t, "ord-1", order.ID)
		})
	})
}

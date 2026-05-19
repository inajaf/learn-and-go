package unittest_test

// =============================================================================
// Golden File Testing — testing complex output against baseline files
// =============================================================================
//
// Idea: instead of assert.Equal(t, "long string...", result)
//       compare result with the contents of testdata/test_name.golden
//
// First run (creating golden files):
//   go test -run TestGolden -update
//
// Subsequent runs (verification):
//   go test -run TestGolden
//
// When to use:
//   - JSON API responses (large, complex)
//   - Serialized protobuf/DTO
//   - Generated code/config
//   - Any output > 3-4 lines
//
// 🏭 In production: golden files are committed to git. In a PR review
//    you see EXACTLY what changed in the output.

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "learning_path/05_unit_testing"
)

// -update flag for updating golden files
var update = flag.Bool("update", false, "update golden files")

// goldenFile reads or updates a golden file.
func goldenFile(t *testing.T, name string, actual []byte) []byte {
	t.Helper()

	path := filepath.Join("testdata", name+".golden")

	if *update {
		// Update the golden file
		err := os.WriteFile(path, actual, 0644)
		require.NoError(t, err, "failed to update golden file")
		return actual
	}

	// Read the existing golden file
	expected, err := os.ReadFile(path)
	require.NoError(t, err,
		"golden file not found: %s\nRun with -update to create it: go test -run %s -update",
		path, t.Name())

	return []byte(strings.ReplaceAll(string(expected), "\r\n", "\n"))
}

// =============================================================================
// Golden-file tests
// =============================================================================

// OrderJSON — struct for the golden test (predictable values).
type OrderJSON struct {
	ID         string  `json:"id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
}

func TestGolden_OrderJSON(t *testing.T) {
	// 👉 Use fixed values — the golden file must not change between runs
	order := OrderJSON{
		ID:         "ord-12345",
		CustomerID: "cust-alice",
		Amount:     149.99,
		Status:     "pending",
		CreatedAt:  "2024-01-15T10:30:00Z",
	}

	actual, err := json.MarshalIndent(order, "", "  ")
	require.NoError(t, err)

	expected := goldenFile(t, "order_response", actual)
	assert.Equal(t, string(expected), string(actual))
}

func TestGolden_OrderListJSON(t *testing.T) {
	orders := []OrderJSON{
		{
			ID:         "ord-001",
			CustomerID: "cust-alice",
			Amount:     99.99,
			Status:     "confirmed",
			CreatedAt:  "2024-01-15T10:00:00Z",
		},
		{
			ID:         "ord-002",
			CustomerID: "cust-bob",
			Amount:     249.50,
			Status:     "pending",
			CreatedAt:  "2024-01-15T11:00:00Z",
		},
	}

	actual, err := json.MarshalIndent(orders, "", "  ")
	require.NoError(t, err)

	expected := goldenFile(t, "order_list_response", actual)
	assert.Equal(t, string(expected), string(actual))
}

func TestGolden_ValidationError(t *testing.T) {
	// A typical JSON response with a validation error
	errResp := map[string]any{
		"error": "validation failed",
		"details": []map[string]string{
			{"field": "customer_id", "message": "required field"},
			{"field": "amount", "message": "must be greater than 0"},
		},
	}

	actual, err := json.MarshalIndent(errResp, "", "  ")
	require.NoError(t, err)

	expected := goldenFile(t, "validation_error", actual)
	assert.Equal(t, string(expected), string(actual))
}

// =============================================================================
// Example: golden test via the real service
// =============================================================================

func TestGolden_PlaceOrder_Integration(t *testing.T) {
	// 👉 Test through the real service but with a predictable result
	repo := &ManualMockRepository{}
	pub := &ManualMockPublisher{}
	svc := NewOrderService(repo, pub)

	order, err := svc.PlaceOrder("cust-golden", 199.99)
	require.NoError(t, err)

	// Normalize unstable fields for the golden file
	// 👉 Any field that depends on time.Now() is replaced with a fixed value.
	//    Otherwise the golden file changes every run!
	response := OrderJSON{
		ID:         "ord-NORMALIZED", // ID depends on time.Now() — normalize
		CustomerID: order.CustomerID,
		Amount:     order.Amount,
		Status:     string(order.Status),
		CreatedAt:  "2024-01-15T10:00:00Z", // Fixed date
	}

	actual, err := json.MarshalIndent(response, "", "  ")
	require.NoError(t, err)

	expected := goldenFile(t, "place_order_normalized", actual)
	assert.Equal(t, string(expected), string(actual))
}

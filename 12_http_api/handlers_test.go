package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
//Mock Service - fake for tests
// =============================================================================

type mockOrderService struct {
	orders map[string]*Order
}

func newMockOrderService() *mockOrderService {
	return &mockOrderService{orders: make(map[string]*Order)}
}

func (m *mockOrderService) CreateOrder(ctx context.Context, req CreateOrderRequest) (*Order, error) {
	order := &Order{
		ID:         fmt.Sprintf("ord-%d", time.Now().UnixNano()),
		CustomerID: req.CustomerID,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	var total float64
	for _, item := range req.Items {
		order.Items = append(order.Items, OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
		})
		total += item.Price * float64(item.Quantity)
	}
	order.Total = total

	m.orders[order.ID] = order
	return order, nil
}

func (m *mockOrderService) GetOrder(ctx context.Context, id string) (*Order, error) {
	order, ok := m.orders[id]
	if !ok {
		return nil, ErrOrderNotFound
	}
	return order, nil
}

func (m *mockOrderService) ListOrders(ctx context.Context) ([]*Order, error) {
	var orders []*Order
	for _, o := range m.orders {
		orders = append(orders, o)
	}
	return orders, nil
}

func (m *mockOrderService) CancelOrder(ctx context.Context, id string) error {
	order, ok := m.orders[id]
	if !ok {
		return ErrOrderNotFound
	}
	order.Status = "cancelled"
	return nil
}

// =============================================================================
//Helper: creating a test environment
// =============================================================================

func setupTest() (http.Handler, *mockOrderService) {
	svc := newMockOrderService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	router := NewRouter(svc, logger)
	return router, svc
}

// =============================================================================
//CreateOrder tests
// =============================================================================

func TestCreateOrder_Success(t *testing.T) {
	router, _ := setupTest()

	body := `{"customer_id": "cust-1", "items": [{"product_id": "prod-1", "quantity": 2, "price": 10.50}]}`
	req := httptest.NewRequest("POST", "/api/v1/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// 👉 201 Created
	assert.Equal(t, http.StatusCreated, rec.Code)

	//Checking the answer
	var order Order
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&order))
	assert.Equal(t, "cust-1", order.CustomerID)
	assert.Equal(t, "pending", order.Status)
	assert.Equal(t, 21.0, order.Total)

	// 👉 Response headers
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestCreateOrder_InvalidJSON(t *testing.T) {
	router, _ := setupTest()

	req := httptest.NewRequest("POST", "/api/v1/orders", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]APIError
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp["error"].Message, "invalid JSON")
}

func TestCreateOrder_ValidationErrors(t *testing.T) {
	router, _ := setupTest()

	//👉 Empty customer_id and items - should receive validation errors
	body := `{"customer_id": "", "items": []}`
	req := httptest.NewRequest("POST", "/api/v1/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]APIError
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp["error"].Message, "validation")
}

// =============================================================================
//GetOrder tests
// =============================================================================

func TestGetOrder_Success(t *testing.T) {
	router, svc := setupTest()

	//We create an order in advance
	svc.orders["ord-test-1"] = &Order{
		ID:         "ord-test-1",
		CustomerID: "cust-1",
		Status:     "pending",
		Total:      42.0,
	}

	req := httptest.NewRequest("GET", "/api/v1/orders/ord-test-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var order Order
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&order))
	assert.Equal(t, "ord-test-1", order.ID)
	assert.Equal(t, 42.0, order.Total)
}

func TestGetOrder_NotFound(t *testing.T) {
	router, _ := setupTest()

	req := httptest.NewRequest("GET", "/api/v1/orders/nonexistent", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	//👉 404 with JSON error, not text "Not Found"
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var resp map[string]APIError
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp["error"].Message, "not found")
}

// =============================================================================
//ListOrders tests
// =============================================================================

func TestListOrders_Empty(t *testing.T) {
	router, _ := setupTest()

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// =============================================================================
//CancelOrder tests
// =============================================================================

func TestCancelOrder_Success(t *testing.T) {
	router, svc := setupTest()

	svc.orders["ord-1"] = &Order{ID: "ord-1", Status: "pending"}

	req := httptest.NewRequest("POST", "/api/v1/orders/ord-1/cancel", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "cancelled", svc.orders["ord-1"].Status)
}

func TestCancelOrder_NotFound(t *testing.T) {
	router, _ := setupTest()

	req := httptest.NewRequest("POST", "/api/v1/orders/nonexistent/cancel", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// =============================================================================
//Health Check Tests
// =============================================================================

func TestHealthCheck(t *testing.T) {
	router, _ := setupTest()

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["status"])
}

// =============================================================================
//Middleware tests
// =============================================================================

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	router, _ := setupTest()

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	//👉 Request ID in response header
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestRequestIDMiddleware_PreservesExistingID(t *testing.T) {
	router, _ := setupTest()

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "my-custom-id")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	//👉 Saves the transferred ID
	assert.Equal(t, "my-custom-id", rec.Header().Get("X-Request-ID"))
}

func TestRecoveryMiddleware_CatchesPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	//Handler who panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic!")
	})

	handler := RecoveryMiddleware(logger)(panicHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	//👉 Don’t panic, but return 500
	assert.NotPanics(t, func() {
		handler.ServeHTTP(rec, req)
	})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	handler := CORSMiddleware("*")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	//👉 OPTIONS request (preflight)
	req := httptest.NewRequest("OPTIONS", "/api/v1/orders", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

// =============================================================================
//Integration test: full flow via httptest.NewServer
// =============================================================================

func TestIntegration_FullOrderLifecycle(t *testing.T) {
	router, _ := setupTest()
	//👉 httptest.NewServer starts a real HTTP server on a random port
	server := httptest.NewServer(router)
	defer server.Close()

	client := server.Client()

	//1. Create an order
	body := `{"customer_id":"cust-1","items":[{"product_id":"prod-1","quantity":3,"price":15.00}]}`
	resp, err := client.Post(server.URL+"/api/v1/orders", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdOrder Order
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createdOrder))
	resp.Body.Close()

	assert.NotEmpty(t, createdOrder.ID)
	assert.Equal(t, 45.0, createdOrder.Total)

	//2. We receive the order by ID
	resp, err = client.Get(server.URL + "/api/v1/orders/" + createdOrder.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	//3. Cancel the order
	cancelReq, _ := http.NewRequest("POST", server.URL+"/api/v1/orders/"+createdOrder.ID+"/cancel", nil)
	resp, err = client.Do(cancelReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	//4. Check your health
	resp, err = client.Get(server.URL + "/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

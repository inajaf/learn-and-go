package productionpatterns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
//Tests context.WithTimeout
// =============================================================================

func TestSlowOperation_CompletesBeforeTimeout(t *testing.T) {
	//👉 Operation for 50ms, timeout 1s - should be in time
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := SlowOperation(ctx, 50*time.Millisecond)

	require.NoError(t, err)
	assert.Equal(t, "the result is ready", result)
}

func TestSlowOperation_TimesOut(t *testing.T) {
	//👉 Operation for 1s, timeout 50ms - won’t have time
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := SlowOperation(ctx, 1*time.Second)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSlowOperation_CancelledByParent(t *testing.T) {
	//👉 Manual cancellation - simulates "client closed connection"
	ctx, cancel := context.WithCancel(context.Background())

	//Cancel after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := SlowOperation(ctx, 1*time.Second)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCallWithTimeout_Success(t *testing.T) {
	//👉 CallWithTimeout sets a timeout of 2s inside, the operation is 1s - it will be in time
	result, err := CallWithTimeout(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "the result is ready", result)
}

// =============================================================================
//Tests context.WithCancel (fan-out with cancellation)
// =============================================================================

func TestRunWorkersWithCancel(t *testing.T) {
	results := RunWorkersWithCancel(context.Background(), 5)

	//👉 Worker 2 crashes and cancels everyone - some workers will also receive an error
	assert.Len(t, results, 5)

	var errors int
	for _, r := range results {
		if r.Err != nil {
			errors++
		}
	}
	//Minimum 1 error (worker 2), possibly more (cancelled)
	assert.GreaterOrEqual(t, errors, 1)
}

// =============================================================================
//Tests context.Value
// =============================================================================

func TestRequestIDFromContext(t *testing.T) {
	t.Run("there is a meaning", func(t *testing.T) {
		ctx := WithRequestID(context.Background(), "req-123")
		assert.Equal(t, "req-123", RequestIDFromContext(ctx))
	})

	t.Run("no value - returns default", func(t *testing.T) {
		//👉 Don't panic, return "unknown"
		assert.Equal(t, "unknown", RequestIDFromContext(context.Background()))
	})
}

func TestUserIDFromContext(t *testing.T) {
	ctx := WithUserID(context.Background(), "user-42")
	assert.Equal(t, "user-42", UserIDFromContext(ctx))
}

func TestContextValuePropagation(t *testing.T) {
	//👉 Values ​​are INHERITED by child contexts
	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-abc")
	ctx = WithUserID(ctx, "user-1")

	//Create a child context with a timeout
	childCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	//Values ​​are also available in child contexts
	assert.Equal(t, "req-abc", RequestIDFromContext(childCtx))
	assert.Equal(t, "user-1", UserIDFromContext(childCtx))
}

// =============================================================================
//Full request flow test
// =============================================================================

func TestProcessRequest(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	err := ProcessRequest(context.Background(), logger)
	require.NoError(t, err)
}

func TestProcessRequest_WithCancelledContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	//👉 Already canceled context - the operation should complete immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ProcessRequest(ctx, logger)
	require.Error(t, err)
}

// =============================================================================
//Test OrderService with context
// =============================================================================

//--- Mock implementations for tests ---

type mockRepo struct {
	orders map[string]*Order
}

func newMockRepo() *mockRepo {
	return &mockRepo{orders: make(map[string]*Order)}
}

func (m *mockRepo) Save(ctx context.Context, order *Order) error {
	//👉 We respect the context - if it’s cancelled, we don’t save it
	if err := ctx.Err(); err != nil {
		return err
	}
	m.orders[order.ID] = order
	return nil
}

func (m *mockRepo) FindByID(ctx context.Context, id string) (*Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	order, ok := m.orders[id]
	if !ok {
		return nil, fmt.Errorf("order %s not found", id)
	}
	return order, nil
}

type mockStock struct {
	available bool
}

func (m *mockStock) CheckStock(ctx context.Context, itemID string, quantity int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return m.available, nil
}

func TestOrderService_CreateOrder_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	repo := newMockRepo()
	stock := &mockStock{available: true}
	svc := NewOrderService(repo, stock, logger)

	ctx := context.Background()
	items := []OrderItem{{ProductID: "prod-1", Quantity: 2, Price: 10.0}}

	order, err := svc.CreateOrder(ctx, "customer-1", items)

	require.NoError(t, err)
	assert.Equal(t, "customer-1", order.CustomerID)
	assert.Equal(t, "pending", order.Status)
}

func TestOrderService_CreateOrder_CancelledContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	repo := newMockRepo()
	stock := &mockStock{available: true}
	svc := NewOrderService(repo, stock, logger)

	//👉 We cancel the context BEFORE the call - the service should immediately return an error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.CreateOrder(ctx, "customer-1", []OrderItem{{ProductID: "prod-1", Quantity: 1, Price: 5.0}})

	require.Error(t, err)
}

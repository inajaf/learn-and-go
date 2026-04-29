package interfaces_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iface "learning_path/01_interfaces"
)

// =============================================================================
//Functional Options tests
// =============================================================================

func TestNewOrderService_DefaultConfig(t *testing.T) {
	//👉 No options - default values ​​are used
	repo := iface.NewInMemoryOrderRepository()
	svc := iface.NewOrderService(repo)

	ctx := context.Background()
	order, err := svc.CreateOrder(ctx, "cust-1", 100.0)
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)
}

func TestNewOrderService_WithLogger(t *testing.T) {
	//👉 We transfer a custom logger via WithLogger
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	repo := iface.NewInMemoryOrderRepository()
	svc := iface.NewOrderService(repo, iface.WithLogger(logger))

	ctx := context.Background()
	_, err := svc.CreateOrder(ctx, "cust-1", 50.0)
	require.NoError(t, err)
	//The service was created with a logger - it works without errors
}

func TestNewOrderService_WithEventPublisher(t *testing.T) {
	//👉 We pass publisher via WithEventPublisher
	repo := iface.NewInMemoryOrderRepository()
	pub := &stubPublisher{}
	svc := iface.NewOrderService(repo, iface.WithEventPublisher(pub))

	ctx := context.Background()
	_, err := svc.CreateOrder(ctx, "cust-1", 75.0)
	require.NoError(t, err)
}

func TestNewOrderService_MultipleOptions(t *testing.T) {
	//👉 Several options at the same time - each one is applied sequentially
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	pub := &stubPublisher{}

	repo := iface.NewInMemoryOrderRepository()
	svc := iface.NewOrderService(repo,
		iface.WithLogger(logger),
		iface.WithEventPublisher(pub),
	)

	ctx := context.Background()
	order, err := svc.CreateOrder(ctx, "cust-1", 200.0)
	require.NoError(t, err)
	assert.Equal(t, "cust-1", order.CustomerID)
}

//stubPublisher - stub for EventPublisher tests.
type stubPublisher struct {
	published [][]byte
}

func (p *stubPublisher) Publish(_ string, payload []byte) error {
	p.published = append(p.published, payload)
	return nil
}

package communication

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
//Circuit Breaker tests
// =============================================================================

func TestCircuitBreaker_ClosedPassesThrough(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)

	err := cb.Call(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CBClosed, cb.State())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)

	for i := 0; i < 3; i++ {
		cb.Call(func() error { return fmt.Errorf("error") })
	}

	assert.Equal(t, CBOpen, cb.State())

	//👉 Next challenge - instant refusal
	err := cb.Call(func() error { return nil })
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

func TestCircuitBreaker_ResetsAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	//Opening
	for i := 0; i < 2; i++ {
		cb.Call(func() error { return fmt.Errorf("error") })
	}
	assert.Equal(t, CBOpen, cb.State())

	//Waiting for reset timeout
	time.Sleep(100 * time.Millisecond)

	//👉 Trial request successful → Closed
	err := cb.Call(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CBClosed, cb.State())
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	for i := 0; i < 2; i++ {
		cb.Call(func() error { return fmt.Errorf("error") })
	}

	time.Sleep(100 * time.Millisecond)

	//👉 Trial request failed → back to Open
	cb.Call(func() error { return fmt.Errorf("still broken") })
	assert.Equal(t, CBOpen, cb.State())
}

// =============================================================================
//ResilientInventoryChecker tests
// =============================================================================

func TestResilientInventoryChecker_NormalOperation(t *testing.T) {
	inventory := NewInMemoryInventory(map[string]int{})
	inventory.AddStock("item-1", 100)

	resilient := NewResilientInventoryChecker(inventory, 3, 1*time.Second)

	available, err := resilient.CheckStock("item-1", 5)
	require.NoError(t, err)
	assert.True(t, available)
}

func TestResilientInventoryChecker_CircuitOpensOnFailures(t *testing.T) {
	//👉 Inventory without stock - ReserveStock will return an error
	inventory := NewInMemoryInventory(map[string]int{})

	resilient := NewResilientInventoryChecker(inventory, 2, 1*time.Second)

	//Two calls to ReserveStock with error → CB opens
	for i := 0; i < 2; i++ {
		resilient.ReserveStock("nonexistent", 1)
	}

	assert.Equal(t, CBOpen, resilient.CircuitState())

	//👉 The next call to CheckStock is fail fast (inventory is not even called)
	_, err := resilient.CheckStock("item-1", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")
}

// =============================================================================
//Test: Saga + Circuit Breaker (integration)
// =============================================================================

func TestSagaWithCircuitBreaker(t *testing.T) {
	inventory := NewInMemoryInventory(map[string]int{})
	inventory.AddStock("item-1", 50)

	//👉 CB with threshold 2 and fast reset
	resilient := NewResilientInventoryChecker(inventory, 2, 50*time.Millisecond)

	payment := NewInMemoryPayment()
	bus := NewInMemoryEventBus()

	saga := NewOrderSagaService(resilient, payment, bus)

	//Regular work
	order, err := saga.CreateOrder("cust-1", "item-1", 5, 100.0)
	require.NoError(t, err)
	assert.Equal(t, StatusConfirmed, order.Status)

	//👉 Check that the CB is in good condition
	assert.Equal(t, CBClosed, resilient.CircuitState())
}

func TestCircuitBreakerIntegration_FailsGracefully(t *testing.T) {
	//Empty inventory → ReserveStock will return errors
	inventory := NewInMemoryInventory(map[string]int{})
	resilient := NewResilientInventoryChecker(inventory, 2, 50*time.Millisecond)

	//Two ReserveStock with error → CB open
	resilient.ReserveStock("missing", 1)
	resilient.ReserveStock("missing", 1)
	assert.Equal(t, CBOpen, resilient.CircuitState())

	//👉 Instant failure - doesn’t even call inventory
	_, err := resilient.CheckStock("any", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")

	//Add stock and wait for reset timeout
	inventory.AddStock("item-1", 10)
	time.Sleep(100 * time.Millisecond)

	//👉 CB tries → success → closes
	available, err := resilient.CheckStock("item-1", 5)
	require.NoError(t, err)
	assert.True(t, available)
	assert.Equal(t, CBClosed, resilient.CircuitState())
}

// InMemoryInventory helper
func (inv *InMemoryInventory) AddStock(itemID string, qty int) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.stock[itemID] = qty
}

//Checking compatibility with the interface
var _ InventoryChecker = (*ResilientInventoryChecker)(nil)

func init() {
	//Make sure that ErrCircuitOpen is different from ErrInsufficientStock
	if errors.Is(ErrCircuitOpen, ErrInsufficientStock) {
		panic("ErrCircuitOpen must be different from ErrInsufficientStock")
	}
}

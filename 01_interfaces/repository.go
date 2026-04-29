package interfaces

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// ─────────────────────────────────────────────────────────────────
//The INTERFACE is a contract. Any type that has these methods
//automatically implements the interface. There are no obvious "implements".
// ─────────────────────────────────────────────────────────────────

//OrderRepository - a contract for working with the order repository.
//👉 The service depends ONLY on this interface, and not on Postgres/Redis/etc.
//
//This allows you to:
//1. Change the implementation without changing the service
//2. Use stubs (mocks) in tests
//
//👉 context.Context is the REQUIRED first parameter of all methods.
//
//This is the standard Go convention for:
//- Timeouts: ctx with deadline → database request does not hang forever
//- Cancellations: ctx.Done() → free resources during shutdown
//- Tracing: trace_id in ctx → trace the request through all services
type OrderRepository interface {
	Save(ctx context.Context, order *Order) error
	FindByID(ctx context.Context, id string) (*Order, error)
	FindAll(ctx context.Context) ([]*Order, error)
	Delete(ctx context.Context, id string) error
}

// ─────────────────────────────────────────────────────────────────
//COMPILE-TIME CHECKS - if you remove the method from the implementation,
//the program will not compile. This is Go best practice.
//👉 Add such lines to the beginning of the implementation file.
// ─────────────────────────────────────────────────────────────────
var _ OrderRepository = (*InMemoryOrderRepository)(nil)
var _ OrderRepository = (*LoggingRepository)(nil)

// ─────────────────────────────────────────────────────────────────
//IMPLEMENTATION 1: InMemoryOrderRepository
//Stores data in memory (map). Great for tests.
// ─────────────────────────────────────────────────────────────────

//InMemoryOrderRepository - implementation of the interface in memory.
//👉 Uses sync.RWMutex to operate securely across multiple goroutines.
type InMemoryOrderRepository struct {
	mu     sync.RWMutex      //protect data from race conditions
	orders map[string]*Order //"database" in memory
}

//NewInMemoryOrderRepository creates a new in-memory repository.
//👉 A constructor is a function that initializes a structure.
//
//We return a specific type, but it will be accepted through the interface.
func NewInMemoryOrderRepository() *InMemoryOrderRepository {
	return &InMemoryOrderRepository{
		orders: make(map[string]*Order),
	}
}

//Save saves an order (creates or updates).
func (r *InMemoryOrderRepository) Save(_ context.Context, order *Order) error {
	r.mu.Lock()         //block for writing
	defer r.mu.Unlock() //unlock when the function is completed

	//We create a copy so that external code cannot change the internal state
	//👉 This is called defensive copy - protection against random mutations
	copied := *order
	r.orders[order.ID] = &copied
	return nil
}

//FindByID searches for an order by ID.
func (r *InMemoryOrderRepository) FindByID(_ context.Context, id string) (*Order, error) {
	r.mu.RLock() //block for read only (several readers at the same time)
	defer r.mu.RUnlock()

	order, ok := r.orders[id]
	if !ok {
		//👉 Return sentinel error - the caller will check via errors.Is()
		return nil, fmt.Errorf("FindByID %q: %w", id, ErrOrderNotFound)
	}

	//We return a copy - we do not allow the internal state to be changed
	copied := *order
	return &copied, nil
}

//FindAll returns all orders.
func (r *InMemoryOrderRepository) FindAll(_ context.Context) ([]*Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Order, 0, len(r.orders))
	for _, o := range r.orders {
		copied := *o
		result = append(result, &copied)
	}
	return result, nil
}

//Delete deletes an order by ID.
func (r *InMemoryOrderRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.orders[id]; !ok {
		return fmt.Errorf("Delete %q: %w", id, ErrOrderNotFound)
	}
	delete(r.orders, id)
	return nil
}

// ─────────────────────────────────────────────────────────────────
//IMPLEMENTATION 2: LoggingRepository - Decorator pattern
//
//Decorator wraps another interface implementation and adds
//new behavior (logging) without changing the original code.
//
//The service receives LoggingRepository but thinks it is running
//with a regular OrderRepository - because the interface is the same!
// ─────────────────────────────────────────────────────────────────

//LoggingRepository wraps any OrderRepository and logs calls.
type LoggingRepository struct {
	inner  OrderRepository //👉 dependent on the interface, not on a specific type
	logger *log.Logger
}

//NewLoggingRepository - decorator constructor.
func NewLoggingRepository(inner OrderRepository, logger *log.Logger) *LoggingRepository {
	return &LoggingRepository{inner: inner, logger: logger}
}

func (l *LoggingRepository) Save(ctx context.Context, order *Order) error {
	l.logger.Printf("[repo] Save order id=%s", order.ID)
	err := l.inner.Save(ctx, order)
	if err != nil {
		l.logger.Printf("[repo] Save error: %v", err)
	}
	return err
}

func (l *LoggingRepository) FindByID(ctx context.Context, id string) (*Order, error) {
	l.logger.Printf("[repo] FindByID id=%s", id)
	order, err := l.inner.FindByID(ctx, id)
	if err != nil {
		l.logger.Printf("[repo] FindByID error: %v", err)
	}
	return order, err
}

func (l *LoggingRepository) FindAll(ctx context.Context) ([]*Order, error) {
	l.logger.Printf("[repo] FindAll")
	return l.inner.FindAll(ctx)
}

func (l *LoggingRepository) Delete(ctx context.Context, id string) error {
	l.logger.Printf("[repo] Delete id=%s", id)
	return l.inner.Delete(ctx, id)
}

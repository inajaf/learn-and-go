package integration_test

// 👉 Integration test — we test the REAL service against a REAL repository.
//    No mocks. We're checking how components work together.
//
// testify/suite lets you group tests with shared setup/teardown.
// Handy when you want to build the "environment" once for a group of tests.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	unittest "learning_path/05_unit_testing" // reuse the service from module 5
)

// ─────────────────────────────────────────────────────────────────
// NoopPublisher — a publisher stub without mocks.
// 👉 In the integration test we care about the service+repo interaction,
//    not about event publishing. So the publisher is a simple stub.
// ─────────────────────────────────────────────────────────────────

// NoopPublisher does nothing — it just accepts events.
type NoopPublisher struct{}

func (n *NoopPublisher) Publish(_ string, _ any) error { return nil }

// ─────────────────────────────────────────────────────────────────
// InMemoryRepository — a real repository for integration tests.
// ─────────────────────────────────────────────────────────────────

type InMemoryRepository struct {
	orders map[string]*unittest.Order
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{orders: make(map[string]*unittest.Order)}
}

func (r *InMemoryRepository) Save(order *unittest.Order) error {
	r.orders[order.ID] = order
	return nil
}

func (r *InMemoryRepository) FindByID(id string) (*unittest.Order, error) {
	o, ok := r.orders[id]
	if !ok {
		return nil, unittest.ErrNotFound
	}
	return o, nil
}

// ─────────────────────────────────────────────────────────────────
// OrderServiceSuite — Test Suite for the integration tests
//
// The Suite struct embeds suite.Suite and holds the shared dependencies.
// ─────────────────────────────────────────────────────────────────

type OrderServiceSuite struct {
	suite.Suite // 👉 embed — we get every Suite method

	// Dependencies for tests — initialized once or before each test
	repo *InMemoryRepository
	svc  *unittest.OrderService
}

// SetupSuite — runs ONCE before all tests in the Suite.
// 👉 Fit for expensive operations: opening a DB connection,
//
//	starting docker-compose, loading fixtures.
func (s *OrderServiceSuite) SetupSuite() {
	s.Suite.T().Log("=== SetupSuite: initializing environment ===")
	// In a real project: connect to the test DB, run migrations, etc.
}

// TearDownSuite — runs ONCE after all tests.
// 👉 Close connections, remove test data.
func (s *OrderServiceSuite) TearDownSuite() {
	s.Suite.T().Log("=== TearDownSuite: cleaning up resources ===")
}

// SetupTest — runs BEFORE EVERY test.
// 👉 Create a fresh repository and service — every test starts from a blank slate.
//
//	This prevents state leaking between tests.
func (s *OrderServiceSuite) SetupTest() {
	s.repo = NewInMemoryRepository()
	s.svc = unittest.NewOrderService(s.repo, &NoopPublisher{})
}

// TearDownTest — runs AFTER each test.
// 👉 Extra cleanup if needed.
func (s *OrderServiceSuite) TearDownTest() {
	// For example, you could roll back a DB transaction here
}

// ─────────────────────────────────────────────────────────────────
// TESTS
// Methods prefixed with Test* run automatically as tests.
// ─────────────────────────────────────────────────────────────────

// TestPlaceAndRetrieveOrder — create an order and read it right back.
// 👉 Integration: verify Save + FindByID work together.
func (s *OrderServiceSuite) TestPlaceAndRetrieveOrder() {
	// Arrange + Act
	order, err := s.svc.PlaceOrder("customer-integration", 1500.0)
	s.Require().NoError(err) // 👉 s.Require() — the Suite equivalent of require.X

	// Read the order — this is a second repository call
	retrieved, err := s.svc.GetOrder(order.ID)
	s.Require().NoError(err)

	// Assert
	s.Equal(order.ID, retrieved.ID)
	s.Equal("customer-integration", retrieved.CustomerID)
	s.Equal(1500.0, retrieved.Amount)
}

// TestPlaceOrder_MultipleOrders — several orders don't interfere with each other.
func (s *OrderServiceSuite) TestPlaceOrder_MultipleOrders() {
	orderA, err := s.svc.PlaceOrder("customer-A", 100)
	s.Require().NoError(err)

	orderB, err := s.svc.PlaceOrder("customer-B", 200)
	s.Require().NoError(err)

	// The IDs must differ
	s.NotEqual(orderA.ID, orderB.ID)

	// Each order is read independently
	gotA, err := s.svc.GetOrder(orderA.ID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "customer-A", gotA.CustomerID)

	gotB, err := s.svc.GetOrder(orderB.ID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "customer-B", gotB.CustomerID)
}

// TestGetOrder_NotFound — fetching a non-existent order.
func (s *OrderServiceSuite) TestGetOrder_NotFound() {
	_, err := s.svc.GetOrder("totally-nonexistent-id")
	s.Require().Error(err)
	s.ErrorIs(err, unittest.ErrNotFound) // 👉 s.ErrorIs — a convenient wrapper over errors.Is
}

// TestIsolation — demonstrating isolation between tests.
// Every test starts with an empty repository (SetupTest creates a new one).
func (s *OrderServiceSuite) TestIsolation() {
	// If tests shared a repository, there would be an order from the previous test here.
	// But SetupTest created a new repo — so it's empty.
	_, err := s.svc.GetOrder("any-id")
	s.Error(err) // empty! isolation works.
}

// ─────────────────────────────────────────────────────────────────
// TestOrderServiceSuite — entry point for running the Suite.
// 👉 This is a regular Go test function that runs the whole Suite.
//
//	go test invokes it, and it calls suite.Run.
//
// ─────────────────────────────────────────────────────────────────
func TestOrderServiceSuite(t *testing.T) {
	suite.Run(t, new(OrderServiceSuite))
}

package unittest_test

// 👉 Here we test OrderService with TWO approaches to mocks:
//    1. Manual Mock — hand-written mock with function fields
//    2. testify/mock — declarative mock with call verification

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "learning_path/05_unit_testing" // 👉 dot import so we don't have to write unittest.OrderService
)

// ══════════════════════════════════════════════════════════════════
// APPROACH 1: Manual mocks
// ══════════════════════════════════════════════════════════════════

// TestPlaceOrder_ManualMock_Success — happy path with a manual mock.
func TestPlaceOrder_ManualMock_Success(t *testing.T) {
	// Arrange
	repo := &ManualMockRepository{} // by default Save returns nil
	pub := &ManualMockPublisher{}   // by default Publish returns nil

	svc := NewOrderService(repo, pub)

	// Act
	order, err := svc.PlaceOrder("cust-123", 500.0)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)
	assert.Equal(t, "cust-123", order.CustomerID)
	assert.Equal(t, 500.0, order.Amount)
	assert.Equal(t, StatusPending, order.Status)

	// 👉 Check Save was called exactly once
	assert.Len(t, repo.SaveCalls, 1)
	assert.Equal(t, order.ID, repo.SaveCalls[0].ID)

	// 👉 Check the event was published
	assert.Len(t, pub.PublishCalls, 1)
	assert.Equal(t, "order.placed", pub.PublishCalls[0].EventType)
}

// TestPlaceOrder_ManualMock_RepoError — repository returned an error.
func TestPlaceOrder_ManualMock_RepoError(t *testing.T) {
	repoErr := errors.New("database connection lost")

	repo := &ManualMockRepository{
		// 👉 Swap the behavior: Save always errors
		SaveFunc: func(order *Order) error {
			return repoErr
		},
	}
	pub := &ManualMockPublisher{}

	svc := NewOrderService(repo, pub)

	_, err := svc.PlaceOrder("cust-1", 100)

	// The service must return an error
	require.Error(t, err)
	// 👉 errors.Is checks the original error in the chain (through %w)
	assert.True(t, errors.Is(err, repoErr))

	// The event must NOT be published — Save failed before Publish
	assert.Empty(t, pub.PublishCalls)
}

// TestPlaceOrder_ManualMock_PublisherError — publisher failed, but the order was still created.
func TestPlaceOrder_ManualMock_PublisherError(t *testing.T) {
	repo := &ManualMockRepository{}
	pub := &ManualMockPublisher{
		// 👉 Publisher fails — but that must not break order creation
		PublishFunc: func(eventType string, payload any) error {
			return errors.New("message broker is down")
		},
	}

	svc := NewOrderService(repo, pub)

	// Act
	order, err := svc.PlaceOrder("cust-1", 100)

	// 👉 The order must be created successfully even if the publisher fails
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)
	// Save was invoked
	assert.Len(t, repo.SaveCalls, 1)
}

// ══════════════════════════════════════════════════════════════════
// APPROACH 2: testify/mock
//
// testify/mock — a library for building mocks with a declarative API.
// Pros:
// - Checks that a method was called with the expected arguments
// - Works with mock.Anything for "I don't care what was passed"
// - AssertExpectations checks every expectation at once
// ══════════════════════════════════════════════════════════════════

// TestifyMockRepository — a testify/mock-based mock.
// 👉 We embed mock.Mock and implement the interface.
type TestifyMockRepository struct {
	mock.Mock // 👉 testify magic — records calls and expectations
}

func (m *TestifyMockRepository) Save(order *Order) error {
	// m.Called() — tells testify the method was called with argument `order`
	args := m.Called(order)
	return args.Error(0) // 👉 return the error from the expectations
}

func (m *TestifyMockRepository) FindByID(id string) (*Order, error) {
	args := m.Called(id)
	return args.Get(0).(*Order), args.Error(1)
}

// TestifyMockPublisher — mock for the publisher.
type TestifyMockPublisher struct {
	mock.Mock
}

func (m *TestifyMockPublisher) Publish(eventType string, payload any) error {
	args := m.Called(eventType, payload)
	return args.Error(0)
}

// TestPlaceOrder_TestifyMock_Success — test with testify/mock.
func TestPlaceOrder_TestifyMock_Success(t *testing.T) {
	// Arrange: create the mocks
	repo := new(TestifyMockRepository)
	pub := new(TestifyMockPublisher)

	// 👉 .On() declares an expectation:
	//    "When Save is called with any argument — return nil"
	repo.On("Save", mock.AnythingOfType("*unittest.Order")).Return(nil)

	// 👉 mock.Anything — don't check the concrete payload
	pub.On("Publish", "order.placed", mock.Anything).Return(nil)

	svc := NewOrderService(repo, pub)

	// Act
	order, err := svc.PlaceOrder("cust-testify", 750.0)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)

	// 👉 AssertExpectations checks that EVERY declared .On() was invoked.
	//    If Save wasn't called — the test fails with a clear message.
	repo.AssertExpectations(t)
	pub.AssertExpectations(t)
}

// TestGetOrder_TestifyMock_CallsRepository — verify GetOrder calls FindByID.
func TestGetOrder_TestifyMock_CallsRepository(t *testing.T) {
	repo := new(TestifyMockRepository)
	pub := new(TestifyMockPublisher)

	expectedOrder := &Order{ID: "ord-42", CustomerID: "cust-1", Amount: 100}

	// 👉 Configure: when FindByID is called with "ord-42" — return expectedOrder
	repo.On("FindByID", "ord-42").Return(expectedOrder, nil)

	svc := NewOrderService(repo, pub)

	result, err := svc.GetOrder("ord-42")

	require.NoError(t, err)
	assert.Equal(t, expectedOrder, result)

	// 👉 AssertCalled checks FindByID was called with exactly "ord-42"
	repo.AssertCalled(t, "FindByID", "ord-42")
	// 👉 AssertNotCalled — Save must not be called during GetOrder
	repo.AssertNotCalled(t, "Save")
}

// TestPlaceOrder_Validation — validation must short-circuit before the repository.
func TestPlaceOrder_Validation(t *testing.T) {
	repo := new(TestifyMockRepository)
	pub := new(TestifyMockPublisher)
	// 👉 No On() declared — if repo.Save is called the test will fail

	svc := NewOrderService(repo, pub)

	_, err := svc.PlaceOrder("", 100) // empty customerID
	require.Error(t, err)

	// The repository wasn't called — validation failed earlier
	repo.AssertNotCalled(t, "Save")
}

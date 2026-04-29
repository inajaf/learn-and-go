package capstone_test

//👉 Capstone test uses testify/suite (Module 6) + mocks (Module 5).
//This is a demonstration of how everything works together.

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	capstone "learning_path/07_capstone"
)

// ─────────────────────────────────────────────────────────────────
//Publisher's mock for tests
// ─────────────────────────────────────────────────────────────────

type SpyPublisher struct {
	events []struct {
		EventType string
		Payload   any
	}
	failOnEvent string //if specified, return an error for this type
}

func (p *SpyPublisher) Publish(eventType string, payload any) error {
	p.events = append(p.events, struct {
		EventType string
		Payload   any
	}{eventType, payload})
	if p.failOnEvent == eventType {
		return errors.New("publisher error")
	}
	return nil
}

func (p *SpyPublisher) EventCount() int { return len(p.events) }
func (p *SpyPublisher) LastEventType() string {
	if len(p.events) == 0 {
		return ""
	}
	return p.events[len(p.events)-1].EventType
}

// ─────────────────────────────────────────────────────────────────
// Suite
// ─────────────────────────────────────────────────────────────────

type CapstoneOrderSuite struct {
	suite.Suite
	ctx       context.Context
	repo      *capstone.InMemoryOrderRepository
	publisher *SpyPublisher
	svc       *capstone.OrderService
}

func (s *CapstoneOrderSuite) SetupTest() {
	s.ctx = context.Background()
	s.repo = capstone.NewInMemoryOrderRepository()
	s.publisher = &SpyPublisher{}
	s.svc = capstone.NewOrderService(s.repo, s.publisher)
}

//TestCreateOrder_Success - full cycle of order creation.
func (s *CapstoneOrderSuite) TestCreateOrder_Success() {
	req := capstone.CreateOrderRequest{
		CustomerID: "cap-customer",
		Items: []capstone.CreateItemRequest{
			{ProductID: "prod-1", Name: "Widget", Quantity: 2, Price: 25.0},
			{ProductID: "prod-2", Name: "Gadget", Quantity: 1, Price: 50.0},
		},
	}

	resp, err := s.svc.CreateOrder(s.ctx, req)
	s.Require().NoError(err)

	//Checking the DTO of the response
	s.NotEmpty(resp.ID)
	s.Equal("cap-customer", resp.CustomerID)
	s.Equal(100.0, resp.TotalAmount) // 2*25 + 1*50 = 100
	s.Equal("pending", resp.Status)
	s.Equal(2, resp.ItemCount)

	//Checking that the event was published
	s.Equal(1, s.publisher.EventCount())
	s.Equal("order.created", s.publisher.LastEventType())
}

//TestCreateOrder_InvalidInput - validation is working.
func (s *CapstoneOrderSuite) TestCreateOrder_InvalidInput() {
	tests := []struct {
		name string
		req  capstone.CreateOrderRequest
	}{
		{
			name: "empty customer_id",
			req:  capstone.CreateOrderRequest{CustomerID: "", Items: []capstone.CreateItemRequest{{Quantity: 1, Price: 10}}},
		},
		{
			name: "no items",
			req:  capstone.CreateOrderRequest{CustomerID: "cust-1", Items: []capstone.CreateItemRequest{}},
		},
		{
			name: "zero quantity",
			req:  capstone.CreateOrderRequest{CustomerID: "cust-1", Items: []capstone.CreateItemRequest{{ProductID: "p1", Quantity: 0, Price: 10}}},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, err := s.svc.CreateOrder(s.ctx, tt.req)
			s.Error(err)
			s.ErrorIs(err, capstone.ErrInvalidInput)
			//If there is a validation error, the event is not published
			s.Equal(0, s.publisher.EventCount())
		})
	}
}

//TestGetOrder_NotFound - request for a non-existent order.
func (s *CapstoneOrderSuite) TestGetOrder_NotFound() {
	_, err := s.svc.GetOrder(s.ctx, "ghost-order-id")
	s.Require().Error(err)
	s.ErrorIs(err, capstone.ErrOrderNotFound)
}

//TestListOrders - list of all orders.
func (s *CapstoneOrderSuite) TestListOrders() {
	//We create two orders
	for i := 0; i < 2; i++ {
		_, err := s.svc.CreateOrder(s.ctx, capstone.CreateOrderRequest{
			CustomerID: "cust-list",
			Items:      []capstone.CreateItemRequest{{ProductID: "p1", Name: "Item", Quantity: 1, Price: 10}},
		})
		s.Require().NoError(err)
	}

	orders, err := s.svc.ListOrders(s.ctx)
	s.Require().NoError(err)
	s.Len(orders, 2)
}

//TestPublisherFailure_DoesNotBreakOrder - publisher fails, the order is still created.
func (s *CapstoneOrderSuite) TestPublisherFailure_DoesNotBreakOrder() {
	//Setting up publisher to fail
	s.publisher.failOnEvent = "order.created"

	resp, err := s.svc.CreateOrder(s.ctx, capstone.CreateOrderRequest{
		CustomerID: "resilient-customer",
		Items:      []capstone.CreateItemRequest{{ProductID: "p1", Name: "Item", Quantity: 1, Price: 99}},
	})

	//👉 The order was created despite the publisher’s failure - this is a deliberate decision
	s.Require().NoError(err)
	s.NotEmpty(resp.ID)

	//We check that the order has actually been saved (integration aspect)
	retrieved, err := s.svc.GetOrder(s.ctx, resp.ID)
	s.Require().NoError(err)
	s.Equal(resp.ID, retrieved.ID)
}

//Entry point to launch Suite
func TestCapstoneOrderSuite(t *testing.T) {
	suite.Run(t, new(CapstoneOrderSuite))
}

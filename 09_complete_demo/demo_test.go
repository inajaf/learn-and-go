package demo_test

//Integration test of the complete system.
//We check the entire path from order creation to event publication.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	demo "learning_path/09_complete_demo"
)

// ─────────────────────────────────────────────────────────────────
//SystemSuite - Test Suite for the entire system.
//👉Module 6: testify/suite
// ─────────────────────────────────────────────────────────────────

type SystemSuite struct {
	suite.Suite
	ctx    context.Context
	system *demo.System
}

//SetupTest - before each test we create a clean system.
func (s *SystemSuite) SetupTest() {
	s.ctx = context.Background()
	s.system = demo.NewSystem(map[string]int{
		"iphone-15":   50,
		"macbook-air": 20,
		"airpods":     100,
	})
}

// ─────────────────────────────────────────────────────────────────
//Test 1: Happy Path - creating an order
// ─────────────────────────────────────────────────────────────────

func (s *SystemSuite) TestCreateOrder_HappyPath() {
	// Arrange
	req := demo.CreateOrderRequest{
		CustomerID: "customer-alice",
		Items: []demo.CreateItemRequest{
			{ProductID: "iphone-15", Name: "iPhone 15", Quantity: 1, UnitPrice: 99900},
			{ProductID: "airpods", Name: "AirPods Pro", Quantity: 2, UnitPrice: 24900},
		},
	}

	// Act
	resp, err := s.system.OrderSvc.CreateOrder(s.ctx, req)

	//Assert: the order was created correctly
	s.Require().NoError(err)
	s.NotEmpty(resp.ID)
	s.Equal("customer-alice", resp.CustomerID)
	s.Equal("confirmed", resp.Status)   //status confirmed after a successful saga
	s.Equal(149700.0, resp.TotalAmount) // 99900 + 2*24900
	s.Len(resp.Items, 2)
	s.NotEmpty(resp.CreatedAt) //mapping time to string (Module 2)

	//Assert: subtotal is calculated correctly in DTO (Module 2: mapping)
	s.Equal(99900.0, resp.Items[0].Subtotal)
	s.Equal(49800.0, resp.Items[1].Subtotal)

	//Assert: warehouse has decreased (synchronous reservation via interface)
	s.Equal(49, s.system.Inventory.StockLevel("iphone-15")) //there were 50, reserved 1
	s.Equal(98, s.system.Inventory.StockLevel("airpods"))   //there were 100, reserved 2

	//Assert: event published to the bus (asynchronously)
	eventLog := s.system.EventBus.EventLog()
	s.Require().Len(eventLog, 1, "there should be one order.created event")
	s.Equal("order.created", eventLog[0].Type)
	s.Equal(resp.ID, eventLog[0].Payload["order_id"])

	//Assert: NotificationService received a notification (asynchronous subscriber)
	emails := s.system.Notification.SentEmails()
	s.Require().Len(emails, 1)
	s.Contains(emails[0], resp.ID)
	s.Contains(emails[0], "customer-alice")

	//Assert: AnalyticsService wrote event (asynchronous subscriber)
	stats := s.system.Analytics.Stats()
	s.Equal(1, stats["order.created"])
}

// ─────────────────────────────────────────────────────────────────
//Test 2: Validation - invalid request
// ─────────────────────────────────────────────────────────────────

func (s *SystemSuite) TestCreateOrder_ValidationErrors() {
	tests := []struct {
		name    string
		req     demo.CreateOrderRequest
		wantErr string
	}{
		{
			name: "empty customer_id",
			req: demo.CreateOrderRequest{
				CustomerID: "",
				Items:      []demo.CreateItemRequest{{ProductID: "p1", Quantity: 1, UnitPrice: 100}},
			},
			wantErr: "customer_id is required",
		},
		{
			name: "no products",
			req: demo.CreateOrderRequest{
				CustomerID: "cust-1",
				Items:      []demo.CreateItemRequest{},
			},
			wantErr: "at least one item",
		},
		{
			name: "zero quantity",
			req: demo.CreateOrderRequest{
				CustomerID: "cust-1",
				Items:      []demo.CreateItemRequest{{ProductID: "p1", Quantity: 0, UnitPrice: 100}},
			},
			wantErr: "quantity must be positive",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, err := s.system.OrderSvc.CreateOrder(s.ctx, tt.req)
			s.Require().Error(err)
			s.Contains(err.Error(), tt.wantErr)
			s.True(errors.Is(err, demo.ErrInvalidInput))

			//There should be no events when there is a validation error
			s.Len(s.system.EventBus.EventLog(), 0)
		})
	}
}

// ─────────────────────────────────────────────────────────────────
//Test 3: Not enough product - Saga with refusal
// ─────────────────────────────────────────────────────────────────

func (s *SystemSuite) TestCreateOrder_InsufficientStock() {
	req := demo.CreateOrderRequest{
		CustomerID: "cust-greedy",
		Items: []demo.CreateItemRequest{
			{ProductID: "macbook-air", Name: "MacBook", Quantity: 100, UnitPrice: 150000}, //only 20 in stock!
		},
	}

	_, err := s.system.OrderSvc.CreateOrder(s.ctx, req)

	s.Require().Error(err)
	s.True(errors.Is(err, demo.ErrInsufficientStock))

	//The warehouse has not changed - no compensation is needed (reservation failed)
	s.Equal(20, s.system.Inventory.StockLevel("macbook-air"))

	//No events - no order created
	s.Len(s.system.EventBus.EventLog(), 0)
	s.Len(s.system.Notification.SentEmails(), 0)
}

// ─────────────────────────────────────────────────────────────────
//Test 4: Complete order life cycle
//Create → Get → Cancel → check events
// ─────────────────────────────────────────────────────────────────

func (s *SystemSuite) TestOrderLifecycle() {
	// Create
	createReq := demo.CreateOrderRequest{
		CustomerID: "cust-lifecycle",
		Items: []demo.CreateItemRequest{
			{ProductID: "airpods", Name: "AirPods", Quantity: 1, UnitPrice: 24900},
		},
	}
	created, err := s.system.OrderSvc.CreateOrder(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal("confirmed", created.Status)

	// Get
	fetched, err := s.system.OrderSvc.GetOrder(s.ctx, created.ID)
	s.Require().NoError(err)
	s.Equal(created.ID, fetched.ID)
	s.Equal("confirmed", fetched.Status)

	// Cancel
	cancelled, err := s.system.OrderSvc.CancelOrder(s.ctx, created.ID)
	s.Require().NoError(err)
	s.Equal("cancelled", cancelled.Status)

	// Get after cancel
	afterCancel, err := s.system.OrderSvc.GetOrder(s.ctx, created.ID)
	s.Require().NoError(err)
	s.Equal("cancelled", afterCancel.Status)

	//Assert events: order.created + order.cancelled
	eventLog := s.system.EventBus.EventLog()
	s.Require().Len(eventLog, 2)
	s.Equal("order.created", eventLog[0].Type)
	s.Equal("order.cancelled", eventLog[1].Type)

	//Assert notifications: two letters
	emails := s.system.Notification.SentEmails()
	s.Require().Len(emails, 2)
	s.Contains(emails[0], "created")
	s.Contains(emails[1], "cancelled")

	//Assert analytics
	stats := s.system.Analytics.Stats()
	s.Equal(1, stats["order.created"])
	s.Equal(1, stats["order.cancelled"])
}

// ─────────────────────────────────────────────────────────────────
//Test 5: Multiple orders - data isolation
// ─────────────────────────────────────────────────────────────────

func (s *SystemSuite) TestMultipleOrders_Independence() {
	req1 := demo.CreateOrderRequest{
		CustomerID: "alice",
		Items:      []demo.CreateItemRequest{{ProductID: "airpods", Quantity: 1, UnitPrice: 24900}},
	}
	req2 := demo.CreateOrderRequest{
		CustomerID: "bob",
		Items:      []demo.CreateItemRequest{{ProductID: "airpods", Quantity: 3, UnitPrice: 24900}},
	}

	order1, err := s.system.OrderSvc.CreateOrder(s.ctx, req1)
	s.Require().NoError(err)

	order2, err := s.system.OrderSvc.CreateOrder(s.ctx, req2)
	s.Require().NoError(err)

	//Different IDs
	s.NotEqual(order1.ID, order2.ID)

	//Warehouse decreased by 4 (1+3)
	s.Equal(96, s.system.Inventory.StockLevel("airpods"))

	//All orders are available
	orders, err := s.system.OrderSvc.ListOrders(s.ctx)
	s.Require().NoError(err)
	s.Len(orders, 2)

	//Two events, two notifications
	s.Len(s.system.EventBus.EventLog(), 2)
	s.Len(s.system.Notification.SentEmails(), 2)
}

// ─────────────────────────────────────────────────────────────────
//Test 6: GetOrder - non-existent order
// ─────────────────────────────────────────────────────────────────

func (s *SystemSuite) TestGetOrder_NotFound() {
	_, err := s.system.OrderSvc.GetOrder(s.ctx, "totally-fake-id")
	s.Require().Error(err)
	s.True(errors.Is(err, demo.ErrOrderNotFound))
}

// ─────────────────────────────────────────────────────────────────
//TestSystemSuite - entry point to launch Suite
// ─────────────────────────────────────────────────────────────────

func TestSystemSuite(t *testing.T) {
	suite.Run(t, new(SystemSuite))
}

// ─────────────────────────────────────────────────────────────────
//Single Test: Demonstrating Loose Coupling
//
//OrderService does not know about NotificationService and AnalyticsService.
//You can add a new subscriber - OrderService does not change!
// ─────────────────────────────────────────────────────────────────

func TestLooseCoupling_AddNewSubscriber(t *testing.T) {
	ctx := context.Background()
	system := demo.NewSystem(map[string]int{"prod-1": 10})

	//Add a new subscriber AFTER creating the system
	//OrderService doesn't know about this subscriber!
	var auditLog []string
	system.EventBus.Subscribe("order.created", func(eventType string, payload map[string]any) {
		auditLog = append(auditLog, fmt.Sprintf("AUDIT: order %s created", payload["order_id"]))
	})

	//Create an order
	_, err := system.OrderSvc.CreateOrder(ctx, demo.CreateOrderRequest{
		CustomerID: "audited-customer",
		Items: []demo.CreateItemRequest{
			{ProductID: "prod-1", Quantity: 1, UnitPrice: 100},
		},
	})
	require.NoError(t, err)

	//The new subscriber received the event - without changing the OrderService!
	assert.Len(t, auditLog, 1)
	assert.Contains(t, auditLog[0], "AUDIT: order")
}

package communication_test

//Tests for module 8 - microservice communication patterns.
//
//Each test demonstrates a specific pattern:
//TestSyncCommunication - synchronous call (analogous to gRPC)
//TestAsyncCommunication - asynchronous events (similar to Kafka)
//TestSagaPattern_Success - successful saga
//TestSagaPattern_Compensation - saga with error compensation
//TestOutboxPattern - reliable publishing via outbox

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	comm "learning_path/08_communication_patterns"
)

// ─────────────────────────────────────────────────────────────────
//Test 1: Synchronous communication (similar to gRPC)
//
//We check that CheckStock and ReserveStock are working correctly.
//In a real project, this would be a gRPC client for the InventoryService.
// ─────────────────────────────────────────────────────────────────

func TestSyncCommunication_CheckStock(t *testing.T) {
	//Arrange: create a warehouse with 10 units of product "prod-1"
	inventory := comm.NewInMemoryInventory(map[string]int{
		"prod-1": 10,
		"prod-2": 0, //out of stock
	})

	t.Run("the product is there", func(t *testing.T) {
		available, err := inventory.CheckStock("prod-1", 5)
		require.NoError(t, err)
		assert.True(t, available)
	})

	t.Run("no product", func(t *testing.T) {
		available, err := inventory.CheckStock("prod-2", 1)
		require.NoError(t, err)
		assert.False(t, available)
	})

	t.Run("we want more than we have", func(t *testing.T) {
		available, err := inventory.CheckStock("prod-1", 100)
		require.NoError(t, err)
		assert.False(t, available)
	})
}

// ─────────────────────────────────────────────────────────────────
//Test 2: Asynchronous communication (similar to Kafka pub/sub)
//
//Several subscribers are subscribed to one event.
//Publisher doesn't know who's listening.
// ─────────────────────────────────────────────────────────────────

func TestAsyncCommunication_FanOut(t *testing.T) {
	bus := comm.NewInMemoryEventBus()

	//Arrange: 3 different subscribers are subscribed to "order.created"
	//This is an analogue of three consumer groups in Kafka
	notificationReceived := false
	analyticsReceived := false
	warehouseReceived := false

	bus.Subscribe("order.created", func(e comm.Event) error {
		notificationReceived = true // NotificationService
		return nil
	})
	bus.Subscribe("order.created", func(e comm.Event) error {
		analyticsReceived = true // AnalyticsService
		return nil
	})
	bus.Subscribe("order.created", func(e comm.Event) error {
		warehouseReceived = true // WarehouseService
		return nil
	})

	//Act: publish the event
	err := bus.Publish(comm.Event{
		ID:   "evt-1",
		Type: "order.created",
		Payload: map[string]any{
			"order_id": "order-123",
		},
	})

	//Assert: all three subscribers received the event
	require.NoError(t, err)
	assert.True(t, notificationReceived, "notification service should receive event")
	assert.True(t, analyticsReceived, "analytics service should receive event")
	assert.True(t, warehouseReceived, "warehouse service should receive event")
}

func TestAsyncCommunication_EventLog(t *testing.T) {
	//👉 Kafka stores an event log - you can replay it.
	//Our InMemoryEventBus also stores a log.
	bus := comm.NewInMemoryEventBus()
	bus.Subscribe("order.created", func(e comm.Event) error { return nil })

	bus.Publish(comm.Event{ID: "evt-1", Type: "order.created"})
	bus.Publish(comm.Event{ID: "evt-2", Type: "order.created"})
	bus.Publish(comm.Event{ID: "evt-3", Type: "payment.completed"})

	log := bus.EventLog()
	assert.Len(t, log, 3, "event log should contain all published events")
	assert.Equal(t, "evt-1", log[0].ID)
	assert.Equal(t, "evt-3", log[2].ID)
}

// ─────────────────────────────────────────────────────────────────
//Test 3: Saga Pattern - successful scenario
//
//Full happy path:
//1. Check the warehouse (synchronously - gRPC)
//2. Backup (synchronously - gRPC)
//3. Payment (synchronously - gRPC)
//4. Publish the event (asynchronously - Kafka)
// ─────────────────────────────────────────────────────────────────

func TestSagaPattern_Success(t *testing.T) {
	// Arrange
	inventory := comm.NewInMemoryInventory(map[string]int{"iphone": 10})
	payment := comm.NewInMemoryPayment()
	bus := comm.NewInMemoryEventBus()

	//Subscribe to the event - let's check that it comes
	var receivedEvent *comm.Event
	bus.Subscribe("order.created", func(e comm.Event) error {
		receivedEvent = &e
		return nil
	})

	svc := comm.NewOrderSagaService(inventory, payment, bus)

	//Act: create an order through Saga
	order, err := svc.CreateOrder("customer-1", "iphone", 2, 999.99)

	//Assert: order created
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)
	assert.Equal(t, comm.StatusConfirmed, order.Status)
	assert.Equal(t, 1999.98, order.TotalAmount)

	//Assert: event published (asynchronous fan-out)
	require.NotNil(t, receivedEvent, "order.created event should be published")
	assert.Equal(t, "order.created", receivedEvent.Type)
	assert.Equal(t, order.ID, receivedEvent.CorrelationID)

	//Assert: warehouse decreased from 10 to 8 (2 reserved)
	//We check if we can take 8 more - yes
	available8, _ := inventory.CheckStock("iphone", 8)
	assert.True(t, available8, "8 iphones should still be available after reserving 2 from 10")
	//We check if we can take 9 - no, there are only 8 left
	available9, _ := inventory.CheckStock("iphone", 9)
	assert.False(t, available9, "9 iphones should NOT be available (only 8 left)")
}

// ─────────────────────────────────────────────────────────────────
//Test 4: Saga Pattern - compensation in case of payment error
//
//If the payment has fallen → the goods must be released (compensation).
//This is the key point of Saga Pattern!
// ─────────────────────────────────────────────────────────────────

func TestSagaPattern_PaymentFails_StockReleased(t *testing.T) {
	// Arrange
	inventory := comm.NewInMemoryInventory(map[string]int{"laptop": 5})
	payment := comm.NewInMemoryPayment()
	payment.SetFailFor("bad-customer") //this client always gets an error

	bus := comm.NewInMemoryEventBus()
	eventPublished := false
	bus.Subscribe("order.created", func(e comm.Event) error {
		eventPublished = true
		return nil
	})

	svc := comm.NewOrderSagaService(inventory, payment, bus)

	//Act: create an order - payment must fail
	order, err := svc.CreateOrder("bad-customer", "laptop", 2, 500.0)

	//Assert: order NOT created
	require.Error(t, err)
	assert.Nil(t, order)
	assert.True(t, errors.Is(err, comm.ErrPaymentFailed),
		"payment error should be returned")

	//Assert: COMPENSATION - the warehouse must be vacated!
	//This is critical - without compensation, the product will forever be “stuck” as reserved
	available, _ := inventory.CheckStock("laptop", 5)
	assert.True(t, available,
		"the warehouse must be fully restored after compensation")

	//Assert: event NOT published (order not created)
	assert.False(t, eventPublished,
		"the event should not be published when the saga fails")
}

// ─────────────────────────────────────────────────────────────────
//Test 5: Saga - not enough product in stock
// ─────────────────────────────────────────────────────────────────

func TestSagaPattern_InsufficientStock(t *testing.T) {
	inventory := comm.NewInMemoryInventory(map[string]int{"item": 1})
	payment := comm.NewInMemoryPayment()
	bus := comm.NewInMemoryEventBus()
	svc := comm.NewOrderSagaService(inventory, payment, bus)

	//Trying to buy 5 pieces when there is only 1
	_, err := svc.CreateOrder("cust-1", "item", 5, 100.0)

	require.Error(t, err)
	assert.True(t, errors.Is(err, comm.ErrInsufficientStock))

	//The warehouse has not changed
	available, _ := inventory.CheckStock("item", 1)
	assert.True(t, available)
}

// ─────────────────────────────────────────────────────────────────
//Test 6: Outbox Pattern
//
//Demo: the event is saved in the outbox along with the order.
//OutboxWorker publishes it later.
//This guarantees "at-least-once" delivery.
// ─────────────────────────────────────────────────────────────────

func TestOutboxPattern_EventPublishedByWorker(t *testing.T) {
	// Arrange
	outboxStore := comm.NewInMemoryOutboxStore()
	bus := comm.NewInMemoryEventBus()

	var publishedEvents []comm.Event
	bus.Subscribe("order.created", func(e comm.Event) error {
		publishedEvents = append(publishedEvents, e)
		return nil
	})

	svc := comm.NewOrderWithOutboxService(outboxStore)
	worker := comm.NewOutboxWorker(outboxStore, bus)

	//Act: create an order
	order, err := svc.CreateOrder("customer-1", 299.99)
	require.NoError(t, err)
	assert.NotEmpty(t, order.ID)

	//Assert: event has NOT been published yet (saved in outbox)
	assert.Len(t, publishedEvents, 0,
		"the event is not published when an order is created - it is only saved in the outbox")

	//Act: launch OutboxWorker
	err = worker.ProcessPending()
	require.NoError(t, err)

	//Assert: the event is now published
	assert.Len(t, publishedEvents, 1,
		"OutboxWorker must publish event from outbox")
	assert.Equal(t, "order.created", publishedEvents[0].Type)

	//Act: launch Worker again
	err = worker.ProcessPending()
	require.NoError(t, err)

	//Assert: event is NOT duplicated (already marked as processed)
	assert.Len(t, publishedEvents, 1,
		"restarting the worker should not duplicate events")
}

// ─────────────────────────────────────────────────────────────────
//Test 7: Comparing Approaches - Final Demonstration
//
//We show the difference: when gRPC (synchronous) and when Kafka (asynchronous).
// ─────────────────────────────────────────────────────────────────

func TestCommunicationPatterns_Overview(t *testing.T) {
	t.Run("synchronous call - immediate response required", func(t *testing.T) {
		//👉 Analogous to gRPC: we need to know IMMEDIATELY whether the product is available
		inventory := comm.NewInMemoryInventory(map[string]int{"prod": 3})

		available, err := inventory.CheckStock("prod", 2)
		require.NoError(t, err)

		//The client receives a response IMMEDIATELY - we continue only if there is one
		if !available {
			t.Skip("There is no product - we cannot create an order")
		}
		assert.True(t, available)
	})

	t.Run("asynchronous event - do not wait for processing", func(t *testing.T) {
		//👉 Analogous to Kafka: we publish a fact and DO NOT wait for someone to process it
		bus := comm.NewInMemoryEventBus()

		//3 services are subscribed - OrderService does not know about them
		results := make([]string, 0)
		bus.Subscribe("order.confirmed", func(e comm.Event) error {
			results = append(results, "notification_sent")
			return nil
		})
		bus.Subscribe("order.confirmed", func(e comm.Event) error {
			results = append(results, "analytics_tracked")
			return nil
		})
		bus.Subscribe("order.confirmed", func(e comm.Event) error {
			results = append(results, "warehouse_notified")
			return nil
		})

		//OrderService just publishes - doesn't know about three subscribers
		err := bus.Publish(comm.Event{
			ID:   "evt-confirmed-1",
			Type: "order.confirmed",
		})
		require.NoError(t, err)

		//All three services responded
		assert.Len(t, results, 3)
		assert.Contains(t, results, "notification_sent")
		assert.Contains(t, results, "analytics_tracked")
		assert.Contains(t, results, "warehouse_notified")
	})
}

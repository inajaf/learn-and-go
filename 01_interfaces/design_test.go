package interfaces_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iface "learning_path/01_interfaces"
)

//newConfirmSvc - local helper: assembles OrderConfirmationService from parts.
//Dependency Injection clearly shows: we pass any interface implementation.
func newConfirmSvc(
	repo iface.OrderRepository,
	notifier iface.Notifier,
	warehouse iface.WarehouseNotifier,
	analytics iface.AnalyticsTracker,
) *iface.OrderConfirmationService {
	return &iface.OrderConfirmationService{
		Orders:    repo,
		Notifier:  notifier,
		Warehouse: warehouse,
		Analytics: analytics,
	}
}

func TestOrderConfirmationService_ConfirmOrder_NotifiesCustomer(t *testing.T) {
	ctx := context.Background()
	repo := iface.NewInMemoryOrderRepository()
	spy := &iface.SpyNotifier{}
	svc := newConfirmSvc(repo, spy, &iface.StubWarehouseNotifier{}, &iface.SpyAnalyticsTracker{})

	baseSvc := iface.NewOrderService(repo)
	order, err := baseSvc.CreateOrder(ctx, "cust-1", 299.99)
	require.NoError(t, err)

	err = svc.ConfirmOrder(ctx, order.ID, "alice@example.com")
	require.NoError(t, err)

	assert.True(t, spy.Called(), "NotifyOrderConfirmed must be called")
	assert.Equal(t, 1, spy.CallCount())
	assert.Equal(t, "alice@example.com", spy.Calls[0].Email)
	assert.Equal(t, order.ID, spy.Calls[0].OrderID)
	assert.Equal(t, 299.99, spy.Calls[0].Amount)
}

func TestOrderConfirmationService_ConfirmOrder_TracksAnalytics(t *testing.T) {
	ctx := context.Background()
	repo := iface.NewInMemoryOrderRepository()
	analytics := &iface.SpyAnalyticsTracker{}
	svc := newConfirmSvc(repo, &iface.StubNotifier{}, &iface.StubWarehouseNotifier{}, analytics)

	baseSvc := iface.NewOrderService(repo)
	order, _ := baseSvc.CreateOrder(ctx, "cust-2", 100.0)

	require.NoError(t, svc.ConfirmOrder(ctx, order.ID, "bob@example.com"))
	assert.Contains(t, analytics.TrackedEvents, order.ID)
}

func TestOrderConfirmationService_ConfirmOrder_UpdatesStatus(t *testing.T) {
	ctx := context.Background()
	repo := iface.NewInMemoryOrderRepository()
	svc := newConfirmSvc(repo, &iface.StubNotifier{}, &iface.StubWarehouseNotifier{}, &iface.StubAnalyticsTracker{})

	baseSvc := iface.NewOrderService(repo)
	order, _ := baseSvc.CreateOrder(ctx, "cust-3", 50.0)

	require.NoError(t, svc.ConfirmOrder(ctx, order.ID, "charlie@example.com"))

	updated, err := repo.FindByID(ctx, order.ID)
	require.NoError(t, err)
	assert.Equal(t, iface.OrderStatusConfirmed, updated.Status)
}

func TestOrderConfirmationService_ConfirmAlreadyConfirmed_ReturnsError(t *testing.T) {
	ctx := context.Background()
	repo := iface.NewInMemoryOrderRepository()
	spy := &iface.SpyNotifier{}
	svc := newConfirmSvc(repo, spy, &iface.StubWarehouseNotifier{}, &iface.StubAnalyticsTracker{})

	baseSvc := iface.NewOrderService(repo)
	order, _ := baseSvc.CreateOrder(ctx, "cust-4", 75.0)

	_ = svc.ConfirmOrder(ctx, order.ID, "dave@example.com")

	err := svc.ConfirmOrder(ctx, order.ID, "dave@example.com")
	assert.Error(t, err, "reconfirmation - error")
	assert.Equal(t, 1, spy.CallCount(), "notification only once")
}

func TestReportService_ExportOrders_CSV(t *testing.T) {
	ctx := context.Background()
	repo := iface.NewInMemoryOrderRepository()
	baseSvc := iface.NewOrderService(repo)
	_, _ = baseSvc.CreateOrder(ctx, "cust-1", 100.0)
	_, _ = baseSvc.CreateOrder(ctx, "cust-2", 200.0)

	svc := &iface.ReportService{Orders: repo, Exporter: &iface.CSVExporter{}}
	data, contentType, filename, err := svc.GenerateReport(ctx)

	require.NoError(t, err)
	assert.Equal(t, "text/csv", contentType)
	assert.Equal(t, "report.csv", filename)
	assert.Contains(t, string(data), "id,customer_id,amount,status")
}

func TestReportService_ExportOrders_JSON(t *testing.T) {
	ctx := context.Background()
	repo := iface.NewInMemoryOrderRepository()
	baseSvc := iface.NewOrderService(repo)
	_, _ = baseSvc.CreateOrder(ctx, "cust-1", 100.0)

	//Same logic - different exporter. The service has not changed!
	svc := &iface.ReportService{Orders: repo, Exporter: &iface.JSONExporter{}}
	data, contentType, filename, err := svc.GenerateReport(ctx)

	require.NoError(t, err)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "report.json", filename)
	assert.Contains(t, string(data), "[")
}

func TestCachedOrderRepository_CachesOnRead(t *testing.T) {
	ctx := context.Background()
	baseRepo := iface.NewInMemoryOrderRepository()
	cache := iface.NewInMemoryCacheStore()
	cachedRepo := &iface.CachedOrderRepository{Repo: baseRepo, Cache: cache}

	baseSvc := iface.NewOrderService(cachedRepo)
	order, err := baseSvc.CreateOrder(ctx, "cust-cache", 500.0)
	require.NoError(t, err)

	found, err := cachedRepo.FindByID(ctx, order.ID)
	require.NoError(t, err)
	assert.Equal(t, order.ID, found.ID)

	_, cached := cache.Get("order:" + order.ID)
	assert.True(t, cached, "after reading, the write should be in the cache")
}

func TestCachedOrderRepository_InvalidatesOnSave(t *testing.T) {
	ctx := context.Background()
	baseRepo := iface.NewInMemoryOrderRepository()
	cache := iface.NewInMemoryCacheStore()
	cachedRepo := &iface.CachedOrderRepository{Repo: baseRepo, Cache: cache}

	baseSvc := iface.NewOrderService(cachedRepo)
	order, _ := baseSvc.CreateOrder(ctx, "cust-inv", 300.0)

	_, _ = cachedRepo.FindByID(ctx, order.ID)
	_, wasCached := cache.Get("order:" + order.ID)
	assert.True(t, wasCached)

	order.Amount = 999.0
	_ = cachedRepo.Save(ctx, order)

	_, stillCached := cache.Get("order:" + order.ID)
	assert.False(t, stillCached, "After the update the cache should be cleared")
}

func TestNotifier_AllImplementationsAreInterchangeable(t *testing.T) {
	//All three implement Notifier - we use it the same way
	notifiers := []iface.Notifier{
		&iface.StubNotifier{},
		&iface.SpyNotifier{},
		&iface.SendGridNotifier{APIKey: "test-key"},
	}
	for _, n := range notifiers {
		err := n.NotifyOrderConfirmed("test@example.com", "order-123", 99.99)
		assert.NoError(t, err, "all Notifier implementations work without error")
	}
}

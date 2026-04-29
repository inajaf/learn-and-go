package concurrencypatterns

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// =============================================================================
//Fan-Out / Fan-In - parallel queries with aggregation
// =============================================================================
//
//Fan-Out: one request → N parallel operations
//Fan-In: N results → one answer
//
//Real life example:
//- Check stock of 10 products: fan-out 10 requests to InventoryService
//- Collect data from 3 services: User + Orders + Payments
//- Send notifications to 1000 users
//
//Key tool: golang.org/x/sync/errgroup

// =============================================================================
//Pattern 1: errgroup - parallel operations with shared cancellation
// =============================================================================

//StockChecker - stock check (simulation of an external call).
type StockChecker struct {
	stock map[string]int
	delay time.Duration //Simulate Network Latency
}

func NewStockChecker(stock map[string]int, delay time.Duration) *StockChecker {
	return &StockChecker{stock: stock, delay: delay}
}

func (sc *StockChecker) CheckItem(ctx context.Context, itemID string, required int) error {
	//Simulating a network call
	select {
	case <-time.After(sc.delay):
	case <-ctx.Done():
		return ctx.Err()
	}

	available, ok := sc.stock[itemID]
	if !ok {
		return fmt.Errorf("product %s not found", itemID)
	}
	if available < required {
		return fmt.Errorf("product %s: needed %d, available %d", itemID, required, available)
	}
	return nil
}

//CheckAllItems checks the stock of ALL items in parallel.
//
// 👉 errgroup.WithContext:
//- Launches N goroutines
//- If ANY returns an error, cancels all others
//- Returns the FIRST error
//
//This is MUCH better than sync.WaitGroup + manual error collection.
func CheckAllItems(ctx context.Context, checker *StockChecker, items map[string]int) error {
	g, ctx := errgroup.WithContext(ctx)

	for itemID, quantity := range items {
		//👉 Capture variables in a closure
		g.Go(func() error {
			return checker.CheckItem(ctx, itemID, quantity)
		})
	}

	//👉 We are waiting for ALL goroutines. Returns the first error or nil.
	return g.Wait()
}

// =============================================================================
//Pattern 2: Fan-Out with Concurrency Limit
// =============================================================================

//CheckAllItemsLimited - the same, but with a limit on simultaneous requests.
//
//👉 Why: an external service may not withstand 1000 simultaneous requests.
//We limit to maxConcurrent.
func CheckAllItemsLimited(ctx context.Context, checker *StockChecker, items map[string]int, maxConcurrent int) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent) //👉 Go 1.20+: built-in limitation

	for itemID, quantity := range items {
		g.Go(func() error {
			return checker.CheckItem(ctx, itemID, quantity)
		})
	}

	return g.Wait()
}

// =============================================================================
//Pattern 3: Fan-Out with results
// =============================================================================

//StockResult — the result of checking one product.
type StockResult struct {
	ItemID    string
	Available bool
	Err       error
}

//CheckAllItemsWithResults returns the result for EVERY item.
//
//👉 Unlike errgroup.Wait(), we don’t stop at the first error.
//The client receives the full picture: “product A is ok, product B is out of stock.”
func CheckAllItemsWithResults(ctx context.Context, checker *StockChecker, items map[string]int) []StockResult {
	var (
		mu      sync.Mutex
		results []StockResult
		wg      sync.WaitGroup
	)

	for itemID, quantity := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := checker.CheckItem(ctx, itemID, quantity)

			mu.Lock()
			results = append(results, StockResult{
				ItemID:    itemID,
				Available: err == nil,
				Err:       err,
			})
			mu.Unlock()
		}()
	}

	wg.Wait()
	return results
}

// =============================================================================
//Pattern 4: Aggregator - collecting data from several services
// =============================================================================
//
//Real Scenario: User Profile Page
//   → UserService.GetUser(id)       (50ms)
//   → OrderService.GetOrders(id)    (100ms)
//   → PaymentService.GetBalance(id) (80ms)
//
//Serial: 230ms
//Parallel: ~100ms (maximum of all)

//UserProfile - aggregated data from several services.
type UserProfile struct {
	Name     string
	Orders   int
	Balance  float64
}

//UserService, OrderCounter, BalanceService - imitation of external services.
type UserService struct{}
type OrderCounter struct{}
type BalanceService struct{}

func (s *UserService) GetName(ctx context.Context, userID string) (string, error) {
	time.Sleep(50 * time.Millisecond)
	return fmt.Sprintf("User_%s", userID), nil
}

func (s *OrderCounter) CountOrders(ctx context.Context, userID string) (int, error) {
	time.Sleep(80 * time.Millisecond)
	return 42, nil
}

func (s *BalanceService) GetBalance(ctx context.Context, userID string) (float64, error) {
	time.Sleep(60 * time.Millisecond)
	return 1500.50, nil
}

//GetUserProfile collects a profile from 3 services in parallel.
func GetUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	g, ctx := errgroup.WithContext(ctx)

	var profile UserProfile
	var mu sync.Mutex

	userSvc := &UserService{}
	orderSvc := &OrderCounter{}
	balanceSvc := &BalanceService{}

	//👉 All three requests in parallel
	g.Go(func() error {
		name, err := userSvc.GetName(ctx, userID)
		if err != nil {
			return fmt.Errorf("user service: %w", err)
		}
		mu.Lock()
		profile.Name = name
		mu.Unlock()
		return nil
	})

	g.Go(func() error {
		count, err := orderSvc.CountOrders(ctx, userID)
		if err != nil {
			return fmt.Errorf("order service: %w", err)
		}
		mu.Lock()
		profile.Orders = count
		mu.Unlock()
		return nil
	})

	g.Go(func() error {
		balance, err := balanceSvc.GetBalance(ctx, userID)
		if err != nil {
			return fmt.Errorf("balance service: %w", err)
		}
		mu.Lock()
		profile.Balance = balance
		mu.Unlock()
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &profile, nil
}

package concurrencypatterns

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
//Worker Pool tests
// =============================================================================

func TestWorkerPool_ProcessAll(t *testing.T) {
	//👉 3 workers process 10 tasks
	pool := NewWorkerPool(3, func(ctx context.Context, input int) (string, error) {
		return fmt.Sprintf("result_%d", input*2), nil
	})

	inputs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	results := pool.Process(context.Background(), inputs)

	assert.Len(t, results, 10)

	//We check that all tasks have been processed (the order may vary!)
	for _, r := range results {
		require.NoError(t, r.Err)
		assert.Contains(t, r.Output, "result_")
	}
}

func TestWorkerPool_HandlesErrors(t *testing.T) {
	pool := NewWorkerPool(2, func(ctx context.Context, input int) (string, error) {
		if input%3 == 0 {
			return "", fmt.Errorf("error for %d", input)
		}
		return "ok", nil
	})

	results := pool.Process(context.Background(), []int{1, 2, 3, 4, 5, 6})

	var errors int
	for _, r := range results {
		if r.Err != nil {
			errors++
		}
	}
	assert.Equal(t, 2, errors) //3 and 6 will return errors
}

func TestWorkerPool_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	pool := NewWorkerPool(2, func(ctx context.Context, input int) (string, error) {
		time.Sleep(200 * time.Millisecond) //Slow operation
		return "done", nil
	})

	results := pool.Process(ctx, []int{1, 2, 3, 4, 5})

	//👉 Most tasks will receive a cancellation error
	var cancelled int
	for _, r := range results {
		if r.Err != nil {
			cancelled++
		}
	}
	assert.Greater(t, cancelled, 0)
}

func TestSimpleWorkerPool(t *testing.T) {
	pool := NewSimpleWorkerPool(3)

	items := []string{"hello", "world", "go", "concurrency"}
	results := pool.ProcessStrings(context.Background(), items, func(ctx context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	})

	assert.Len(t, results, 4)
	for _, r := range results {
		require.NoError(t, r.Err)
		assert.Equal(t, strings.ToUpper(r.Input), r.Output)
	}
}

// =============================================================================
//Pipeline tests
// =============================================================================

func TestPipeline(t *testing.T) {
	numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	results := Pipeline(context.Background(), numbers)

	//👉 Even numbers only (2, 4, 6, 8, 10)
	assert.Len(t, results, 5)
	for _, r := range results {
		assert.True(t, strings.HasPrefix(r, "number_"))
	}
}

func TestPipeline_EmptyInput(t *testing.T) {
	results := Pipeline(context.Background(), nil)
	assert.Empty(t, results)
}

// =============================================================================
//Fan-Out/errgroup tests
// =============================================================================

func TestCheckAllItems_Success(t *testing.T) {
	checker := NewStockChecker(map[string]int{
		"item-1": 100,
		"item-2": 50,
		"item-3": 200,
	}, 10*time.Millisecond)

	items := map[string]int{
		"item-1": 5,
		"item-2": 10,
		"item-3": 1,
	}

	err := CheckAllItems(context.Background(), checker, items)
	require.NoError(t, err)
}

func TestCheckAllItems_InsufficientStock(t *testing.T) {
	checker := NewStockChecker(map[string]int{
		"item-1": 3,
	}, 10*time.Millisecond)

	items := map[string]int{
		"item-1": 100, //👉 Need 100, available 3
	}

	err := CheckAllItems(context.Background(), checker, items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "item-1")
}

func TestCheckAllItems_ParallelExecution(t *testing.T) {
	//👉 Each check takes 100ms
	checker := NewStockChecker(map[string]int{
		"item-1": 100,
		"item-2": 100,
		"item-3": 100,
	}, 100*time.Millisecond)

	items := map[string]int{"item-1": 1, "item-2": 1, "item-3": 1}

	start := time.Now()
	err := CheckAllItems(context.Background(), checker, items)
	duration := time.Since(start)

	require.NoError(t, err)
	//👉 Parallel: ~100ms, sequential would be ~300ms
	assert.Less(t, duration, 200*time.Millisecond)
}

func TestCheckAllItemsWithResults(t *testing.T) {
	checker := NewStockChecker(map[string]int{
		"item-1": 100,
		"item-2": 2, //👉 Low drainage
	}, 10*time.Millisecond)

	items := map[string]int{
		"item-1": 5,
		"item-2": 50, //Need 50, available 2
	}

	results := CheckAllItemsWithResults(context.Background(), checker, items)

	assert.Len(t, results, 2)

	//👉 We get results for EVERY product, and not just the first error
	resultMap := make(map[string]StockResult)
	for _, r := range results {
		resultMap[r.ItemID] = r
	}

	assert.True(t, resultMap["item-1"].Available)
	assert.False(t, resultMap["item-2"].Available)
}

// =============================================================================
//Aggregator tests (fan-out from several services)
// =============================================================================

func TestGetUserProfile(t *testing.T) {
	start := time.Now()
	profile, err := GetUserProfile(context.Background(), "user-42")
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, "User_user-42", profile.Name)
	assert.Equal(t, 42, profile.Orders)
	assert.Equal(t, 1500.50, profile.Balance)

	//👉 Parallel: ~80ms (max out of 50, 80, 60), not 190ms
	assert.Less(t, duration, 150*time.Millisecond)
}

// =============================================================================
//Rate Limiter tests
// =============================================================================

func TestTokenBucketLimiter_AllowsUpToMax(t *testing.T) {
	limiter := NewTokenBucketLimiter(3, 1*time.Second)
	defer limiter.Stop()

	//👉 3 tokens available immediately
	assert.True(t, limiter.Allow())
	assert.True(t, limiter.Allow())
	assert.True(t, limiter.Allow())

	//4th - no tokens
	assert.False(t, limiter.Allow())
}

func TestTokenBucketLimiter_Refills(t *testing.T) {
	limiter := NewTokenBucketLimiter(2, 50*time.Millisecond)
	defer limiter.Stop()

	//We take all tokens
	limiter.Allow()
	limiter.Allow()
	assert.False(t, limiter.Allow())

	//We are waiting for replenishment
	time.Sleep(120 * time.Millisecond)

	//👉 Tokens have been replenished
	assert.True(t, limiter.Allow())
}

func TestTokenBucketLimiter_Wait(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 50*time.Millisecond)
	defer limiter.Stop()

	//We take the only token
	limiter.Allow()

	//Wait blocks until refilled
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx)
	require.NoError(t, err) //👉 Waited for the token
}

func TestTokenBucketLimiter_WaitTimeout(t *testing.T) {
	limiter := NewTokenBucketLimiter(1, 5*time.Second) //Very slow replenishment
	defer limiter.Stop()

	limiter.Allow() //They took the only one

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx)
	require.Error(t, err) //👉 Didn’t wait - timeout
}

// =============================================================================
//Semaphore tests
// =============================================================================

func TestSemaphore_LimitsConcurrency(t *testing.T) {
	sem := NewSemaphore(2)

	//👉 Two grips - ok
	require.NoError(t, sem.Acquire(context.Background()))
	require.NoError(t, sem.Acquire(context.Background()))

	//The third one must block. We check using TryAcquire.
	assert.False(t, sem.TryAcquire())

	//Freeing up one slot
	sem.Release()

	//Now you can
	assert.True(t, sem.TryAcquire())
}

func TestSemaphore_AcquireRespectsContext(t *testing.T) {
	sem := NewSemaphore(1)
	sem.Acquire(context.Background()) //Occupied the only slot

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := sem.Acquire(ctx)
	require.Error(t, err) //👉 Timeout
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

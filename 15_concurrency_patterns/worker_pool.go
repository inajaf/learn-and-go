package concurrencypatterns

import (
	"context"
	"fmt"
	"sync"
)

// =============================================================================
//Worker Pool - a pool of workers for parallel processing of tasks
// =============================================================================
//
//For what:
//- 10_000 tasks, but only 10 CPU → no need for 10_000 goroutines
//- Limiting the load on external services (rate limiting)
//- Controlled parallelism
//
//Structure:
//
//   ┌──────────┐     ┌─────────┐     ┌──────────┐
//   │ Producer │────▶│  Jobs   │────▶│ Worker 1 │──┐
//   │          │     │ Channel │     │ Worker 2 │  │    ┌─────────┐
//   │          │     │         │     │ Worker 3 │  ├───▶│ Results │
//   │          │     │         │     │   ...    │  │    │ Channel │
//   │          │     │         │     │ Worker N │──┘    └─────────┘
//   └──────────┘     └─────────┘     └──────────┘

//Task - task to be processed.
type Task[T any, R any] struct {
	ID    int
	Input T
}

//Result — the result of processing the task.
type Result[R any] struct {
	TaskID int
	Output R
	Err    error
}

//WorkerPool - a pool of workers with a fixed number of goroutines.
type WorkerPool[T any, R any] struct {
	workerCount int
	processor   func(ctx context.Context, input T) (R, error)
}

//NewWorkerPool creates a pool of workers.
//
//Example:
//
//	pool := NewWorkerPool(5, func(ctx context.Context, url string) (int, error) {
//	    resp, err := http.Get(url)
//	    if err != nil { return 0, err }
//	    return resp.StatusCode, nil
//	})
//
//	results := pool.Process(ctx, urls)
func NewWorkerPool[T any, R any](workerCount int, processor func(ctx context.Context, input T) (R, error)) *WorkerPool[T, R] {
	return &WorkerPool[T, R]{
		workerCount: workerCount,
		processor:   processor,
	}
}

//Process processes tasks through a pool of workers.
//Returns a slice of results (the order may differ from the input!).
func (wp *WorkerPool[T, R]) Process(ctx context.Context, inputs []T) []Result[R] {
	jobs := make(chan Task[T, R], len(inputs))
	results := make(chan Result[R], len(inputs))

	//👉 Launching workers
	var wg sync.WaitGroup
	for i := 0; i < wp.workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range jobs {
				//Checking the cancellation
				if ctx.Err() != nil {
					results <- Result[R]{TaskID: task.ID, Err: ctx.Err()}
					continue
				}

				output, err := wp.processor(ctx, task.Input)
				results <- Result[R]{TaskID: task.ID, Output: output, Err: err}
			}
		}()
	}

	//👉 Sending tasks
	for i, input := range inputs {
		jobs <- Task[T, R]{ID: i, Input: input}
	}
	close(jobs) //We signal the workers: there are no more tasks

	//👉 We are waiting for all workers to complete and closing the results
	go func() {
		wg.Wait()
		close(results)
	}()

	//Collecting results
	var collected []Result[R]
	for r := range results {
		collected = append(collected, r)
	}

	return collected
}

// =============================================================================
//Simple Worker Pool without generics (to understand the basics)
// =============================================================================

//SimpleWorkerPool - a pool for processing string tasks.
//👉 Easier to understand if generics are still unfamiliar.
type SimpleWorkerPool struct {
	workerCount int
}

func NewSimpleWorkerPool(workerCount int) *SimpleWorkerPool {
	return &SimpleWorkerPool{workerCount: workerCount}
}

//ProcessStrings processes strings in parallel.
func (p *SimpleWorkerPool) ProcessStrings(ctx context.Context, items []string, fn func(context.Context, string) (string, error)) []SimpleResult {
	jobs := make(chan indexedItem, len(items))
	results := make(chan SimpleResult, len(items))

	var wg sync.WaitGroup
	for i := 0; i < p.workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				if ctx.Err() != nil {
					results <- SimpleResult{Index: job.index, Err: ctx.Err()}
					continue
				}
				output, err := fn(ctx, job.item)
				results <- SimpleResult{
					Index:  job.index,
					Input:  job.item,
					Output: output,
					Err:    err,
				}
			}
		}(i)
	}

	for i, item := range items {
		jobs <- indexedItem{index: i, item: item}
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var collected []SimpleResult
	for r := range results {
		collected = append(collected, r)
	}
	return collected
}

type indexedItem struct {
	index int
	item  string
}

type SimpleResult struct {
	Index  int
	Input  string
	Output string
	Err    error
}

// =============================================================================
//Pipeline - data processing pipeline
// =============================================================================
//
//Pattern: each stage is a goroutine, the stages are connected by channels.
//
//   Generate → Filter → Transform → Collect
//      │          │          │          │
//   []int      chan int   chan int   chan string
//
//👉 In production: ETL processes, log processing, data pipeline.

//Pipeline demonstrates pipeline processing.
func Pipeline(ctx context.Context, numbers []int) []string {
	//Stage 1: Generation - putting data into the channel
	source := generate(ctx, numbers)

	//Stage 2: Filtering - we pass only even numbers
	filtered := filter(ctx, source, func(n int) bool { return n%2 == 0 })

	//Stage 3: Transformation - numbers to strings
	transformed := transform(ctx, filtered, func(n int) string {
		return fmt.Sprintf("number_%d", n)
	})

	//Step 4: Collect Results
	var results []string
	for s := range transformed {
		results = append(results, s)
	}
	return results
}

func generate(ctx context.Context, numbers []int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range numbers {
			select {
			case out <- n:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func filter(ctx context.Context, in <-chan int, predicate func(int) bool) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			if predicate(n) {
				select {
				case out <- n:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func transform(ctx context.Context, in <-chan int, fn func(int) string) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		for n := range in {
			select {
			case out <- fn(n):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

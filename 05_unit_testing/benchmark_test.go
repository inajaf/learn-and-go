package unittest_test

// =============================================================================
// Benchmark tests — measuring performance
// =============================================================================
//
// Run:
//   go test -bench=. -benchmem ./05_unit_testing/...
//
// Output:
//   BenchmarkPlaceOrder-8    1000000    1050 ns/op    512 B/op    8 allocs/op
//                    │           │         │            │             │
//                    │           │         │            │             └─ allocations per op
//                    │           │         │            └─ bytes of memory per op
//                    │           │         └─ nanoseconds per op
//                    │           └─ iteration count
//                    └─ GOMAXPROCS
//
// When to use:
//   - Hot-path optimization (serialization, mapping, validation)
//   - Comparing two implementations (A vs B)
//   - Checking that a refactor didn't regress performance
//
// 🏭 In production: benchstat for comparing results
//   go test -bench=. -count=10 > old.txt
//   # ... make changes ...
//   go test -bench=. -count=10 > new.txt
//   benchstat old.txt new.txt

import (
	"testing"

	. "learning_path/05_unit_testing"
)

// BenchmarkPlaceOrder — benchmark for creating an order.
// 👉 b.N is chosen automatically for a statistically significant result.
func BenchmarkPlaceOrder(b *testing.B) {
	repo := &ManualMockRepository{}
	pub := &ManualMockPublisher{}
	svc := NewOrderService(repo, pub)

	// b.ResetTimer() — resets the timer after setup (if setup is expensive)
	b.ResetTimer()

	for b.Loop() {
		_, _ = svc.PlaceOrder("cust-bench", 99.99)
	}
}

// BenchmarkPlaceOrder_Parallel — parallel benchmark.
// 👉 Shows throughput under concurrent access.
func BenchmarkPlaceOrder_Parallel(b *testing.B) {
	repo := &ManualMockRepository{}
	pub := &ManualMockPublisher{}
	svc := NewOrderService(repo, pub)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = svc.PlaceOrder("cust-parallel", 50.0)
		}
	})
}

// BenchmarkGetOrder — benchmark for reading an order.
func BenchmarkGetOrder(b *testing.B) {
	repo := &ManualMockRepository{
		FindByIDFunc: func(id string) (*Order, error) {
			return &Order{ID: id, CustomerID: "cust-1", Amount: 100}, nil
		},
	}
	pub := &ManualMockPublisher{}
	svc := NewOrderService(repo, pub)

	b.ResetTimer()

	for b.Loop() {
		_, _ = svc.GetOrder("ord-1")
	}
}

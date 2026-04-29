package productionpatterns

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// =============================================================================
//context.Context - REQUIRED first parameter in production code
// =============================================================================
//
//In Go context.Context solves 3 problems:
//1. Canceling operations (cancellation) - the user has left, there is no need to continue
//2. Deadlines/timeouts - don’t wait forever
//3. Request-scoped values ​​- request ID, user ID (NOT business data!)
//
//Rule: ctx is ALWAYS the first parameter, NEVER put in the structure.

//--- Interfaces with context.Context (this is what any production code looks like) ----------

//OrderRepository - a repository with context in each method.
//👉 Without ctx, the database request will hang forever if the database is unavailable.
type OrderRepository interface {
	Save(ctx context.Context, order *Order) error
	FindByID(ctx context.Context, id string) (*Order, error)
}

//StockChecker is an external service (gRPC). Without ctx there is no timeout.
type StockChecker interface {
	CheckStock(ctx context.Context, itemID string, quantity int) (bool, error)
}

//--- Domain model (for examples) -----------------------------------------

type Order struct {
	ID         string
	CustomerID string
	Items      []OrderItem
	Status     string
	CreatedAt  time.Time
}

type OrderItem struct {
	ProductID string
	Quantity  int
	Price     float64
}

//--- OrderService with correct use of context -----------------------

type OrderService struct {
	repo   OrderRepository
	stock  StockChecker
	logger *slog.Logger
}

func NewOrderService(repo OrderRepository, stock StockChecker, logger *slog.Logger) *OrderService {
	return &OrderService{repo: repo, stock: stock, logger: logger}
}

//CreateOrder demonstrates propagation: ctx is passed down the entire chain.
//
//	HTTP Handler → Service → Repository
//	                      → StockChecker (gRPC)
//
//If the client cancels the request, all operations are stopped.
func (s *OrderService) CreateOrder(ctx context.Context, customerID string, items []OrderItem) (*Order, error) {
	//👉 Check ctx BEFORE the hard work begins
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("order creation canceled before start: %w", err)
	}

	s.logger.InfoContext(ctx, "creating an order",
		slog.String("customer_id", customerID),
		slog.Int("item_count", len(items)),
	)

	//👉 We check the stock of each product with the same ctx
	for _, item := range items {
		available, err := s.stock.CheckStock(ctx, item.ProductID, item.Quantity)
		if err != nil {
			return nil, fmt.Errorf("%s drain check error: %w", item.ProductID, err)
		}
		if !available {
			return nil, fmt.Errorf("product %s: not enough in stock", item.ProductID)
		}
	}

	order := &Order{
		ID:         fmt.Sprintf("ord_%d", time.Now().UnixNano()),
		CustomerID: customerID,
		Items:      items,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	//👉 ctx is forwarded to the repository - if the database is slow, the timeout will work
	if err := s.repo.Save(ctx, order); err != nil {
		return nil, fmt.Errorf("error saving order: %w", err)
	}

	s.logger.InfoContext(ctx, "order created", slog.String("order_id", order.ID))
	return order, nil
}

//GetOrder is a simple example with ctx.
func (s *OrderService) GetOrder(ctx context.Context, id string) (*Order, error) {
	return s.repo.FindByID(ctx, id)
}

// =============================================================================
//Pattern 1: context.WithTimeout - operation time limit
// =============================================================================

//SlowOperation simulates a slow operation (a request to an external API).
func SlowOperation(ctx context.Context, duration time.Duration) (string, error) {
	select {
	case <-time.After(duration):
		//👉 The operation completed before the timeout
		return "the result is ready", nil
	case <-ctx.Done():
		//👉 Context cancelled—the client has left or timed out
		return "", fmt.Errorf("operation aborted: %w", ctx.Err())
	}
}

//CallWithTimeout shows how the calling code sets the timeout.
//
//Important: the child ctx INHERITS the deadline from the parent.
//If the parent ctx has a timeout of 5s, the child cannot be longer.
func CallWithTimeout(parentCtx context.Context) (string, error) {
	//👉 2 seconds maximum for this operation
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel() //👉 ALWAYS defer cancel() - goroutines leak without it!

	return SlowOperation(ctx, 1*time.Second)
}

// =============================================================================
//Pattern 2: context.WithCancel - manual cancellation
// =============================================================================

//WorkerResult is the result of the work of one worker.
type WorkerResult struct {
	WorkerID int
	Data     string
	Err      error
}

//RunWorkersWithCancel starts N workers and cancels everything at the first error.
//
//👉 In production, this is the “fan-out with cancellation” pattern:
//- Checking N microservices in parallel
//- If one falls, cancel all the others
func RunWorkersWithCancel(ctx context.Context, workerCount int) []WorkerResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan WorkerResult, workerCount)

	for i := 0; i < workerCount; i++ {
		go func(id int) {
			//👉 Each worker checks ctx
			select {
			case <-ctx.Done():
				results <- WorkerResult{WorkerID: id, Err: ctx.Err()}
				return
			case <-time.After(time.Duration(id*100) * time.Millisecond):
				if id == 2 {
					//👉 Worker 2 “falls” - cancel everyone
					cancel()
					results <- WorkerResult{WorkerID: id, Err: fmt.Errorf("worker %d: critical error", id)}
					return
				}
				results <- WorkerResult{WorkerID: id, Data: fmt.Sprintf("result from worker %d", id)}
			}
		}(i)
	}

	collected := make([]WorkerResult, 0, workerCount)
	for i := 0; i < workerCount; i++ {
		collected = append(collected, <-results)
	}
	return collected
}

// =============================================================================
//Pattern 3: context.WithValue - request-scoped metadata
// =============================================================================
//
//⚠️ IMPORTANT: context.Value is NOT for business data!
//
//✅ Correct: request ID, trace ID, user ID (for logging/metrics)
//❌ Incorrect: config, DB connection, business parameters
//
//Reason: Value is not type safe, there is no compile-time check,
//It's easy to forget to put a value and end up with nil.

//contextKey - private type for context keys.
//👉 The private type GUARANTEES that another package will not overwrite our value.
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userIDKey    contextKey = "user_id"
)

//WithRequestID puts the request ID into the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

//RequestIDFromContext retrieves the request ID from the context.
//👉 We always return the default if there is no value - we never panic.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return "unknown"
}

//WithUserID puts the user ID into the context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

//UserIDFromContext retrieves the user ID from the context.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}

// =============================================================================
//Pattern 4: Propagation via middleware → service → repository
// =============================================================================
//
//Typical production flow:
//
//   HTTP Request
//       │
//       ▼
//Middleware: ctx = WithRequestID(ctx, uuid) ← put request ID
//       │
//       ▼
//Handler: ctx, cancel = WithTimeout(ctx, 30s) ← set the timeout
//       │
//       ▼
//Service: s.repo.Save(ctx, order) ← forward down
//       │
//       ▼
//Repository: db.ExecContext(ctx, query) ← The database uses ctx to cancel
//       │
//       ▼
//PostgreSQL: Cancels query if ctx.Done()

//ProcessRequest simulates a complete request flow with propagation.
func ProcessRequest(ctx context.Context, logger *slog.Logger) error {
	//1. Middleware adds request ID
	ctx = WithRequestID(ctx, "req-abc-123")
	ctx = WithUserID(ctx, "user-42")

	//2. Handler sets a timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	//3. Logging uses values ​​from the context
	reqID := RequestIDFromContext(ctx)
	userID := UserIDFromContext(ctx)

	logger.InfoContext(ctx, "request processing",
		slog.String("request_id", reqID),
		slog.String("user_id", userID),
	)

	//4. Call the service with the same ctx
	result, err := SlowOperation(ctx, 100*time.Millisecond)
	if err != nil {
		logger.ErrorContext(ctx, "operation error",
			slog.String("request_id", reqID),
			slog.String("error", err.Error()),
		)
		return err
	}

	logger.InfoContext(ctx, "request processed",
		slog.String("request_id", reqID),
		slog.String("result", result),
	)
	return nil
}

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// =============================================================================
//HTTP API - a full-fledged REST server on net/http (Go 1.22+)
// =============================================================================
//
//Go 1.22 added pattern routing to the standard http.ServeMux:
//   - GET /api/v1/orders/{id}     ← path parameters!
//- POST /api/v1/orders ← method in the pattern
//   - DELETE /api/v1/orders/{id}
//
//Before Go 1.22, this required chi/gorilla/gin.
//Now stdlib is sufficient for most APIs.

// =============================================================================
//Domain - models and interfaces (simplified for example)
// =============================================================================

type Order struct {
	ID         string      `json:"id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Status     string      `json:"status"`
	Total      float64     `json:"total"`
	CreatedAt  time.Time   `json:"created_at"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

//OrderService - business logic interface.
//👉 Handler depends on the interface, not on the implementation.
type OrderService interface {
	CreateOrder(ctx context.Context, req CreateOrderRequest) (*Order, error)
	GetOrder(ctx context.Context, id string) (*Order, error)
	ListOrders(ctx context.Context) ([]*Order, error)
	CancelOrder(ctx context.Context, id string) error
}

// =============================================================================
//DTO - request/response objects
// =============================================================================

type CreateOrderRequest struct {
	CustomerID string             `json:"customer_id"`
	Items      []CreateItemRequest `json:"items"`
}

type CreateItemRequest struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

//Validate validates the DTO. Collects ALL errors, does not stop at the first one.
func (r *CreateOrderRequest) Validate() error {
	var errs []string

	if r.CustomerID == "" {
		errs = append(errs, "customer_id: required field")
	}
	if len(r.Items) == 0 {
		errs = append(errs, "items: there must be at least one product")
	}
	for i, item := range r.Items {
		if item.ProductID == "" {
			errs = append(errs, fmt.Sprintf("items[%d].product_id: required field", i))
		}
		if item.Quantity <= 0 {
			errs = append(errs, fmt.Sprintf("items[%d].quantity: must be > 0", i))
		}
		if item.Price <= 0 {
			errs = append(errs, fmt.Sprintf("items[%d].price: must be > 0", i))
		}
	}

	if len(errs) > 0 {
		return &APIError{
			Code:    http.StatusBadRequest,
			Message: "validation errors",
			Details: errs,
		}
	}
	return nil
}

// =============================================================================
//APIError - structured error for the client
// =============================================================================
//
//👉 The client always receives JSON, even if there is an error:
//   {
//     "error": {
//       "code": 404,
//"message": "order not found",
//"details": ["order with ID ord-123 does not exist"]
//     }
//   }

type APIError struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

//Sentinel errors for the domain layer
var (
	ErrOrderNotFound = errors.New("order not found")
	ErrInvalidInput  = errors.New("invalid data")
)

//MapDomainError converts a domain error to an APIError.
//👉 The Domain layer does not know about HTTP. Mapping - at the transport level.
func MapDomainError(err error) *APIError {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		return &APIError{Code: http.StatusNotFound, Message: "order not found"}
	case errors.Is(err, ErrInvalidInput):
		return &APIError{Code: http.StatusBadRequest, Message: "invalid data"}
	default:
		//👉 We do not disclose internal errors to the client!
		return &APIError{Code: http.StatusInternalServerError, Message: "Internal Server Error"}
	}
}

// =============================================================================
//NewRouter - creating and configuring a router
// =============================================================================

//NewRouter creates an HTTP router with handlers and middleware.
//
//A typical call to main():
//
//	svc := NewInMemoryOrderService()
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	router := NewRouter(svc, logger)
//	http.ListenAndServe(":8080", router)
func NewRouter(svc OrderService, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	h := &OrderHandler{service: svc, logger: logger}

	//👉 Go 1.22+: METHOD /path - method directly in the pattern
	mux.HandleFunc("POST /api/v1/orders", h.CreateOrder)
	mux.HandleFunc("GET /api/v1/orders", h.ListOrders)
	mux.HandleFunc("GET /api/v1/orders/{id}", h.GetOrder)
	mux.HandleFunc("POST /api/v1/orders/{id}/cancel", h.CancelOrder)

	//Health check (for Kubernetes liveness/readiness probes)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	//👉 We use middleware (order: outside → inside)
	var handler http.Handler = mux
	handler = RecoveryMiddleware(logger)(handler)       //3. Catching panic
	handler = RequestLoggingMiddleware(logger)(handler)  //2. Log the request
	handler = RequestIDMiddleware()(handler)              //1. Generate request ID

	return handler
}

// =============================================================================
//Helpers - utilities for handlers
// =============================================================================

//writeJSON sends a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

//writeError sends a JSON error.
func writeError(w http.ResponseWriter, apiErr *APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiErr.Code)
	json.NewEncoder(w).Encode(map[string]*APIError{"error": apiErr})
}

//decodeJSON decodes JSON from the request body.
//👉 We limit body size (protection from abuse).
func decodeJSON(r *http.Request, dst any) error {
	//1MB limit - protection from huge payloads
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() //👉 Strict parsing: unknown fields = error

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("JSON parsing error: %w", err)
	}
	return nil
}

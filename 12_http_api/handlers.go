package httpapi

import (
	"log/slog"
	"net/http"
)

// =============================================================================
//OrderHandler - HTTP handlers for orders
// =============================================================================
//
//Pattern: Handler as a method on a struct (not a closure).
//
//✅ struct with dependencies - easy to test using a mock service
//❌ closure - dependencies are closed, difficult to replace in tests
//
//Each handler follows the pattern:
//1. Decode - parse the request (path params, query params, body)
//2. Validate - check the data
//3. Call Service - call business logic
//4. Encode - generate a response

type OrderHandler struct {
	service OrderService
	logger  *slog.Logger
}

//CreateOrder handles POST /api/v1/orders
//
//	Request:  { "customer_id": "cust-1", "items": [...] }
//	Response: 201 Created + { "id": "ord-123", ... }
//Errors: 400 Bad Request (invalid JSON or data)
//	          500 Internal Server Error
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	// 1. Decode
	var req CreateOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, &APIError{
			Code:    http.StatusBadRequest,
			Message: "invalid JSON",
			Details: []string{err.Error()},
		})
		return
	}

	// 2. Validate
	if err := req.Validate(); err != nil {
		if apiErr, ok := err.(*APIError); ok {
			writeError(w, apiErr)
			return
		}
		writeError(w, &APIError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	//3. Call Service (with context from the request!)
	//👉 r.Context() contains request ID, timeouts, cancellation
	order, err := h.service.CreateOrder(r.Context(), req)
	if err != nil {
		apiErr := MapDomainError(err)
		h.logger.ErrorContext(r.Context(), "order creation error",
			slog.String("error", err.Error()),
		)
		writeError(w, apiErr)
		return
	}

	// 4. Encode (201 Created)
	h.logger.InfoContext(r.Context(), "order created",
		slog.String("order_id", order.ID),
	)
	writeJSON(w, http.StatusCreated, order)
}

//GetOrder handles GET /api/v1/orders/{id}
//
//	Response: 200 OK + { "id": "ord-123", ... }
//	Errors:   404 Not Found
func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	//👉 Go 1.22+: path parameter via r.PathValue()
	id := r.PathValue("id")
	if id == "" {
		writeError(w, &APIError{Code: http.StatusBadRequest, Message: "id is required"})
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusOK, order)
}

//ListOrders handles GET /api/v1/orders
//
//	Response: 200 OK + [{ ... }, { ... }]
func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.service.ListOrders(r.Context())
	if err != nil {
		writeError(w, MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusOK, orders)
}

//CancelOrder processes POST /api/v1/orders/{id}/cancel
//
//Response: 200 OK + { "message": "order cancelled" }
//	Errors:   404 Not Found
func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, &APIError{Code: http.StatusBadRequest, Message: "id is required"})
		return
	}

	if err := h.service.CancelOrder(r.Context(), id); err != nil {
		writeError(w, MapDomainError(err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "order canceled"})
}

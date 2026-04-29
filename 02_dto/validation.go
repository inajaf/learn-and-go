package dto

import (
	"fmt"
	"strings"
)

// =============================================================================
// DTO validation — manual implementation
// =============================================================================
//
// 👉 Why do validation manually when go-playground/validator exists?
//
//   In production validator/v10 is the standard. But it's important to understand WHAT it does:
//   - Collects ALL errors (doesn't stop at the first one)
//   - Returns structured errors (field + rule + value)
//   - Supports custom rules
//
//   Here we implement the same approach by hand — to understand the principle.
//
// 🏭 In production: use github.com/go-playground/validator/v10
//   with tags like `validate:"required,min=1,max=100"`.
//   Manual validation — for custom business rules.

// ValidationError — a structured validation error.
// Contains ALL the problems found, not only the first one.
//
//	This matters for APIs: the client gets every error in a single request
//	instead of fixing them one at a time.
type ValidationError struct {
	Fields []FieldError
}

// FieldError — an error for a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = fmt.Sprintf("%s: %s", f.Field, f.Message)
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

// HasErrors returns true if there are validation errors.
func (e *ValidationError) HasErrors() bool {
	return len(e.Fields) > 0
}

// Validate checks a CreateOrderRequest.
//
// 👉 Collects ALL errors — the client sees the full picture in a single request.
//
//	Returns nil if everything is OK, or *ValidationError with details.
//
// Rules:
//   - customer_id: required, not empty after trim, max 100 characters
//   - amount: required, > 0, max 1_000_000
func (req CreateOrderRequest) Validate() error {
	var ve ValidationError

	// customer_id
	trimmed := strings.TrimSpace(req.CustomerID)
	if trimmed == "" {
		ve.Fields = append(ve.Fields, FieldError{
			Field:   "customer_id",
			Message: "required field",
		})
	} else if len(trimmed) > 100 {
		ve.Fields = append(ve.Fields, FieldError{
			Field:   "customer_id",
			Message: fmt.Sprintf("max 100 characters, got %d", len(trimmed)),
		})
	}

	// amount
	if req.Amount <= 0 {
		ve.Fields = append(ve.Fields, FieldError{
			Field:   "amount",
			Message: "must be greater than 0",
		})
	} else if req.Amount > 1_000_000 {
		ve.Fields = append(ve.Fields, FieldError{
			Field:   "amount",
			Message: fmt.Sprintf("max 1000000, got %.2f", req.Amount),
		})
	}

	if ve.HasErrors() {
		return &ve
	}
	return nil // 👉 Return nil, not &ve{} — otherwise you'll hit the nil interface pitfall!
}

// Validate checks an UpdateOrderRequest.
func (req UpdateOrderRequest) Validate() error {
	var ve ValidationError

	if req.Amount != nil {
		if *req.Amount <= 0 {
			ve.Fields = append(ve.Fields, FieldError{
				Field:   "amount",
				Message: "must be greater than 0",
			})
		} else if *req.Amount > 1_000_000 {
			ve.Fields = append(ve.Fields, FieldError{
				Field:   "amount",
				Message: fmt.Sprintf("max 1000000, got %.2f", *req.Amount),
			})
		}
	}

	if ve.HasErrors() {
		return &ve
	}
	return nil
}

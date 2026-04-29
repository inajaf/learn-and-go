package errorhandling

import (
	"errors"
	"fmt"
	"time"
)

// =============================================================================
//Error patterns in Go - from junior to senior
// =============================================================================
//
//In Go, errors are values ​​(not exceptions!).
//This gives complete control, but requires discipline.
//
//Three levels of maturity:
//1. Junior: if err != nil { return err } ← loses context
//2. Middle: fmt.Errorf("save order: %w", err) ← adds context
//3. Senior: custom types + behavioral checking ← full control

// =============================================================================
//Level 1: Sentinel Errors - simple named errors
// =============================================================================
//
//Sentinel error is a packet level variable with a fixed message.
//Use it when the error does NOT contain data, only fact.
//
//Convention: name starts with Err.

var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyExists  = errors.New("already exists")
	ErrInvalidInput   = errors.New("invalid data")
	ErrUnauthorized   = errors.New("not authorized")
	ErrForbidden      = errors.New("access denied")
	ErrConflict       = errors.New("version conflict")
	ErrInternalServer = errors.New("Internal Server Error")
)

//Checking sentinel error:
//
//   if errors.Is(err, ErrNotFound) {
//// handle 404
//   }

// =============================================================================
//Level 2: Wrapping - adding context when forwarding
// =============================================================================
//
//❌ Bad: return err
//→ "sql: no rows in result set" - WHERE? In what request?
//
//✅ Good: return fmt.Errorf("search for order %s: %w", id, err)
//→ "order search ord-123: sql: no rows in result set" - CLEAR!
//
//%w - wraps the error (errors.Is/As will continue to work)
//%v - DOES NOT wrap (the chain is lost, do not use for errors)

//WrapExample shows proper wrapping through layers.
//
//	Repository: sql: no rows in result set
//	     ↓ wrap
//Service: order search ord-123: sql: no rows in result set
//	     ↓ wrap
//Handler: processing GET /orders/ord-123: order search ord-123: sql: no rows in result set
func WrapExample() error {
	//Simulating an error from the repository
	repoErr := fmt.Errorf("sql: no rows in result set")

	//Service wraps
	serviceErr := fmt.Errorf("order search ord-123: %w", repoErr)

	//Handler wraps
	handlerErr := fmt.Errorf("processing GET /orders/ord-123: %w", serviceErr)

	return handlerErr
}

// =============================================================================
//Level 3: Custom Error Types - data errors
// =============================================================================
//
//When sentinel error is not enough:
//- You need to transfer data (which ID was not found? which field is invalid?)
//- It is necessary to distinguish between subtypes (NotFound for an order vs NotFound for a client)
//- Need type safe checking via errors.As

//--- NotFoundError: resource not found, with details ---------------------------

//NotFoundError - resource not found.
//👉 Contains WHAT exactly was not found and what ID.
type NotFoundError struct {
	Resource string // "order", "customer", "product"
	ID       string //Resource ID
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s with ID %s not found", e.Resource, e.ID)
}

//Is allows comparison with sentinel error ErrNotFound.
//👉 errors.Is(customErr, ErrNotFound) will return true
func (e *NotFoundError) Is(target error) bool {
	return target == ErrNotFound
}

//NewNotFoundError creates a NotFoundError.
func NewNotFoundError(resource, id string) error {
	return &NotFoundError{Resource: resource, ID: id}
}

//--- ValidationError: validation errors, multiple -----------------------

//FieldError—one field error.
type FieldError struct {
	Field   string // "email", "price", "quantity"
	Message string // "cannot be empty", "must be > 0"
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

//ValidationError - a set of validation errors.
//👉 Collects ALL errors, rather than stopping at the first one.
//
//	{
//	  "errors": [
//{"field": "email", "message": "cannot be empty"},
//{"field": "price", "message": "must be > 0"}
//	  ]
//	}
type ValidationError struct {
	Errors []FieldError
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return fmt.Sprintf("validation error: %s", e.Errors[0].Error())
	}
	return fmt.Sprintf("validation errors (%d): %s ...", len(e.Errors), e.Errors[0].Error())
}

//Is allows you to check via errors.Is(err, ErrInvalidInput).
func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidInput
}

//HasErrors returns true if there is at least one error.
func (e *ValidationError) HasErrors() bool {
	return len(e.Errors) > 0
}

//Add adds a field error.
func (e *ValidationError) Add(field, message string) {
	e.Errors = append(e.Errors, FieldError{Field: field, Message: message})
}

//--- ConflictError: optimistic lock conflict -------------------

//ConflictError - version conflict (optimistic locking).
type ConflictError struct {
	Resource        string
	ID              string
	ExpectedVersion int
	ActualVersion   int
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf(
		"version conflict for %s %s: expected version %d, current version %d",
		e.Resource, e.ID, e.ExpectedVersion, e.ActualVersion,
	)
}

func (e *ConflictError) Is(target error) bool {
	return target == ErrConflict
}

//--- OperationError: business operation error with context --------------------

//OperationError - operation error with full context.
//👉 Convenient for logging and debugging.
type OperationError struct {
	Op       string    // "CreateOrder", "ProcessPayment"
	Resource string    // "order", "payment"
	ID       string    //Resource ID (if any)
	Err      error     //Original error
	Time     time.Time //When did it happen
}

func (e *OperationError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("%s %s(%s): %v", e.Op, e.Resource, e.ID, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Resource, e.Err)
}

//Unwrap allows errors.Is/As to flow through the chain.
func (e *OperationError) Unwrap() error {
	return e.Err
}

//NewOperationError creates an OperationError.
func NewOperationError(op, resource, id string, err error) error {
	return &OperationError{
		Op:       op,
		Resource: resource,
		ID:       id,
		Err:      err,
		Time:     time.Now(),
	}
}

// =============================================================================
//Level 4: Behavioral Errors - checking through interfaces
// =============================================================================
//
//Problem: errors.As binds the calling code to a specific type.
//- Package A defines *NotFoundError
//- Package B needs to import A to check
//- It creates addiction
//
//Solution: check BEHAVIOR, not type.
//- Error implementing interface{ NotFound() bool }
//- The calling code checks the interface without importing the package

//NotFoundChecker - interface for checking "not found".
type NotFoundChecker interface {
	NotFound() bool
}

//TemporaryChecker - interface for checking "temporary error" (can be retry).
type TemporaryChecker interface {
	Temporary() bool
}

//Implementing interfaces for our error types

func (e *NotFoundError) NotFound() bool { return true }

//IsNotFound checks for errors via the behavioral interface.
//👉 Does not depend on the specific type of error!
func IsNotFound(err error) bool {
	var nf NotFoundChecker
	if errors.As(err, &nf) {
		return nf.NotFound()
	}
	//Fallback on sentinel
	return errors.Is(err, ErrNotFound)
}

//IsTemporary checks whether retry is possible.
func IsTemporary(err error) bool {
	var t TemporaryChecker
	if errors.As(err, &t) {
		return t.Temporary()
	}
	return false
}

//TemporaryError - an error that can be repeated.
type TemporaryError struct {
	Err error
}

func (e *TemporaryError) Error() string    { return e.Err.Error() }
func (e *TemporaryError) Unwrap() error    { return e.Err }
func (e *TemporaryError) Temporary() bool  { return true }

//NewTemporaryError marks the error as temporary.
func NewTemporaryError(err error) error {
	return &TemporaryError{Err: err}
}

// =============================================================================
//errors.Join (Go 1.20+) - joining multiple errors
// =============================================================================
//
//When you need to return MULTIPLE errors:
//- Validation of multiple fields
//- Closing multiple resources
//- Parallel operations

//ValidateOrder demonstrates multiple error collection.
func ValidateOrder(customerID string, items []string, total float64) error {
	var errs []error

	if customerID == "" {
		errs = append(errs, fmt.Errorf("customer_id: required field"))
	}

	if len(items) == 0 {
		errs = append(errs, fmt.Errorf("items: there must be at least one product"))
	}

	if total <= 0 {
		errs = append(errs, fmt.Errorf("total: must be > 0, received: %.2f", total))
	}

	if len(errs) > 0 {
		return fmt.Errorf("order validation: %w", errors.Join(errs...))
	}

	return nil
}

//CloseAll closes multiple resources and collects errors.
//
//	defer func() {
//	    if err := CloseAll(db, kafka, redis); err != nil {
//logger.Error("errors when closing", "error", err)
//	    }
//	}()
func CloseAll(closers ...interface{ Close() error }) error {
	var errs []error
	for _, c := range closers {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// =============================================================================
//Pattern: Error Mapping (Domain → HTTP/gRPC)
// =============================================================================
//
//👉 Domain errors should not know about HTTP/gRPC.
//Mapping occurs at the handler/transport level.

//HTTPStatusCode converts a domain error into an HTTP status.
func HTTPStatusCode(err error) int {
	if err == nil {
		return 200
	}

	switch {
	case errors.Is(err, ErrNotFound):
		return 404
	case errors.Is(err, ErrInvalidInput):
		return 400
	case errors.Is(err, ErrUnauthorized):
		return 401
	case errors.Is(err, ErrForbidden):
		return 403
	case errors.Is(err, ErrConflict):
		return 409
	case errors.Is(err, ErrAlreadyExists):
		return 409
	default:
		return 500
	}
}

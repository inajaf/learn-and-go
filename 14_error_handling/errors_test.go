package errorhandling

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
//Sentinel Errors Tests
// =============================================================================

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
	}{
		{"NotFound", ErrNotFound, ErrNotFound},
		{"InvalidInput", ErrInvalidInput, ErrInvalidInput},
		{"Conflict", ErrConflict, ErrConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, tt.err, tt.target)
		})
	}
}

// =============================================================================
//Wrapping tests (chain of errors)
// =============================================================================

func TestWrapExample(t *testing.T) {
	err := WrapExample()

	//👉 Full context in the message
	assert.Contains(t, err.Error(), "processing GET /orders/ord-123")
	assert.Contains(t, err.Error(), "order search ord-123")
	assert.Contains(t, err.Error(), "sql: no rows in result set")
}

func TestWrappedErrorChain(t *testing.T) {
	//👉 errors.Is goes through the ENTIRE %w chain
	original := ErrNotFound
	wrapped1 := fmt.Errorf("service: %w", original)
	wrapped2 := fmt.Errorf("handler: %w", wrapped1)

	assert.ErrorIs(t, wrapped2, ErrNotFound)
}

// =============================================================================
//Tests Custom Error Types
// =============================================================================

func TestNotFoundError(t *testing.T) {
	err := NewNotFoundError("order", "ord-123")

	t.Run("message contains resource and ID", func(t *testing.T) {
		assert.Contains(t, err.Error(), "order")
		assert.Contains(t, err.Error(), "ord-123")
	})

	t.Run("errors.Is with ErrNotFound", func(t *testing.T) {
		//👉 Custom type is compatible with sentinel via the Is() method
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("errors.As for data access", func(t *testing.T) {
		var nfe *NotFoundError
		require.ErrorAs(t, err, &nfe)
		assert.Equal(t, "order", nfe.Resource)
		assert.Equal(t, "ord-123", nfe.ID)
	})

	t.Run("works via wrapping", func(t *testing.T) {
		wrapped := fmt.Errorf("GetOrder: %w", err)
		//errors.Is goes through %w
		assert.ErrorIs(t, wrapped, ErrNotFound)

		var nfe *NotFoundError
		require.ErrorAs(t, wrapped, &nfe)
		assert.Equal(t, "ord-123", nfe.ID)
	})
}

func TestValidationError(t *testing.T) {
	ve := &ValidationError{}
	ve.Add("email", "cannot be empty")
	ve.Add("price", "must be > 0")

	t.Run("HasErrors", func(t *testing.T) {
		assert.True(t, ve.HasErrors())
	})

	t.Run("contains all errors", func(t *testing.T) {
		assert.Len(t, ve.Errors, 2)
		assert.Equal(t, "email", ve.Errors[0].Field)
		assert.Equal(t, "price", ve.Errors[1].Field)
	})

	t.Run("errors.Is with ErrInvalidInput", func(t *testing.T) {
		assert.ErrorIs(t, ve, ErrInvalidInput)
	})

	t.Run("message for one error", func(t *testing.T) {
		single := &ValidationError{}
		single.Add("name", "required field")
		assert.Contains(t, single.Error(), "validation error")
		assert.Contains(t, single.Error(), "name")
	})

	t.Run("message for multiple errors", func(t *testing.T) {
		assert.Contains(t, ve.Error(), "validation errors (2)")
	})
}

func TestConflictError(t *testing.T) {
	err := &ConflictError{
		Resource:        "order",
		ID:              "ord-123",
		ExpectedVersion: 3,
		ActualVersion:   5,
	}

	assert.ErrorIs(t, err, ErrConflict)
	assert.Contains(t, err.Error(), "version 3")
	assert.Contains(t, err.Error(), "current version 5")
}

func TestOperationError(t *testing.T) {
	original := NewNotFoundError("order", "ord-456")
	opErr := NewOperationError("GetOrder", "order", "ord-456", original)

	t.Run("message contains context", func(t *testing.T) {
		assert.Contains(t, opErr.Error(), "GetOrder")
		assert.Contains(t, opErr.Error(), "order(ord-456)")
	})

	t.Run("Unwrap preserves the chain", func(t *testing.T) {
		//👉 errors.Is passed through Unwrap
		assert.ErrorIs(t, opErr, ErrNotFound)
	})

	t.Run("errors.As via Unwrap", func(t *testing.T) {
		var nfe *NotFoundError
		require.ErrorAs(t, opErr, &nfe)
		assert.Equal(t, "ord-456", nfe.ID)
	})
}

// =============================================================================
//Behavioral Errors Tests
// =============================================================================

func TestIsNotFound_CustomType(t *testing.T) {
	err := NewNotFoundError("product", "prod-1")
	assert.True(t, IsNotFound(err))
}

func TestIsNotFound_Sentinel(t *testing.T) {
	err := fmt.Errorf("something: %w", ErrNotFound)
	assert.True(t, IsNotFound(err))
}

func TestIsNotFound_RegularError(t *testing.T) {
	err := fmt.Errorf("some other error")
	assert.False(t, IsNotFound(err))
}

func TestIsTemporary(t *testing.T) {
	t.Run("temporary error", func(t *testing.T) {
		err := NewTemporaryError(fmt.Errorf("connection timeout"))
		assert.True(t, IsTemporary(err))
	})

	t.Run("regular error — not temporary", func(t *testing.T) {
		err := fmt.Errorf("invalid input")
		assert.False(t, IsTemporary(err))
	})

	t.Run("wrapped temporary", func(t *testing.T) {
		inner := NewTemporaryError(fmt.Errorf("timeout"))
		wrapped := fmt.Errorf("service: %w", inner)
		//👉 errors.As goes through wrapping
		assert.True(t, IsTemporary(wrapped))
	})
}

// =============================================================================
//errors.Join tests
// =============================================================================

func TestValidateOrder_MultipleErrors(t *testing.T) {
	err := ValidateOrder("", nil, -10)

	require.Error(t, err)
	//👉 All errors in one message
	assert.Contains(t, err.Error(), "customer_id")
	assert.Contains(t, err.Error(), "items")
	assert.Contains(t, err.Error(), "total")
}

func TestValidateOrder_Valid(t *testing.T) {
	err := ValidateOrder("cust-1", []string{"item-1"}, 99.99)
	assert.NoError(t, err)
}

func TestValidateOrder_PartialErrors(t *testing.T) {
	//👉 Only one error (items empty)
	err := ValidateOrder("cust-1", nil, 99.99)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "items")
	assert.NotContains(t, err.Error(), "customer_id")
}

// =============================================================================
//Error Mapping Tests
// =============================================================================

func TestHTTPStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"nil → 200", nil, 200},
		{"NotFound → 404", ErrNotFound, 404},
		{"InvalidInput → 400", ErrInvalidInput, 400},
		{"Unauthorized → 401", ErrUnauthorized, 401},
		{"Forbidden → 403", ErrForbidden, 403},
		{"Conflict → 409", ErrConflict, 409},
		{"AlreadyExists → 409", ErrAlreadyExists, 409},
		{"Unknown → 500", fmt.Errorf("unexpected"), 500},
		//👉 Custom types are also mapped via errors.Is
		{"Custom NotFound → 404", NewNotFoundError("order", "1"), 404},
		{"Wrapped NotFound → 404", fmt.Errorf("svc: %w", ErrNotFound), 404},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HTTPStatusCode(tt.err))
		})
	}
}

// =============================================================================
//Example: how everything works together in a real service
// =============================================================================

func TestRealWorldErrorFlow(t *testing.T) {
	//1. Repository returns custom error
	repoErr := NewNotFoundError("order", "ord-999")

	//2. Service wraps
	serviceErr := NewOperationError("GetOrder", "order", "ord-999", repoErr)

	//3. Handler checks - ALL methods work:

	//Via sentinel
	assert.ErrorIs(t, serviceErr, ErrNotFound)

	//Via custom type
	var nfe *NotFoundError
	require.ErrorAs(t, serviceErr, &nfe)
	assert.Equal(t, "ord-999", nfe.ID)

	//Via behavioral interface
	assert.True(t, IsNotFound(serviceErr))

	//Via HTTP mapping
	assert.Equal(t, 404, HTTPStatusCode(serviceErr))
}

// =============================================================================
//Antipatterns (what NOT to do)
// =============================================================================

func TestAntiPatterns(t *testing.T) {
	t.Run("❌ string comparison instead of errors.Is", func(t *testing.T) {
		err := fmt.Errorf("wrapper: %w", ErrNotFound)

		//❌ BAD - it will break if wrapped
		badCheck := err.Error() == "not found"
		assert.False(t, badCheck) //doesn't work!

		//✅ GOOD - goes through wrapping
		goodCheck := errors.Is(err, ErrNotFound)
		assert.True(t, goodCheck)
	})

	t.Run("❌ type assertion instead of errors.As", func(t *testing.T) {
		var err error = NewOperationError("Get", "order", "1", NewNotFoundError("order", "1"))

		//❌ BAD - does not see through wrapping
		_, badCheck := err.(*NotFoundError)
		assert.False(t, badCheck) //doesn't work! (OperationError, not NotFoundError)

		//✅ GOOD - goes through Unwrap
		var nfe *NotFoundError
		goodCheck := errors.As(err, &nfe)
		assert.True(t, goodCheck)
	})
}

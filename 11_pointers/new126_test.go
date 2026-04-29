//go:build go1.26

//Tests for new(expr) - a new feature in Go 1.26.
//This file compiles with Go 1.26+ ONLY.
//The IDE with the old analyzer will pass it through - there will be no errors.
package pointers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	p "learning_path/11_pointers"
)

// ─────────────────────────────────────────────────────────────────
//Go 1.26: new(expr) - pointer directly from expression
//
//Before 1.26: &42 - compiler error, a temporary variable is needed.
//After 1.26: new(42) - works directly.
// ─────────────────────────────────────────────────────────────────

func TestNew_Go126_FromLiteral(t *testing.T) {
	//Go 1.26: new(expr) - pointer straight from literal
	pi := new(42)
	pb := new(true)
	ps := new("hello")

	require.NotNil(t, pi)
	require.NotNil(t, pb)
	require.NotNil(t, ps)

	assert.Equal(t, 42, *pi)
	assert.Equal(t, true, *pb)
	assert.Equal(t, "hello", *ps)
}

func TestNew_Go126_FromExpression(t *testing.T) {
	//Go 1.26: new(expr) - pointer from function call
	add := func(a, b int) int { return a + b }

	result := new(add(3, 4))
	require.NotNil(t, result)
	assert.Equal(t, 7, *result)
}

func TestNew_Go126_BuildProfileAfter(t *testing.T) {
	//BuildProfileAfter uses new(expr) internally (see new126.go)
	after := p.BuildProfileAfter()

	require.NotNil(t, after.IsActive)
	require.NotNil(t, after.Age)
	require.NotNil(t, after.Bio)

	assert.True(t, *after.IsActive)
	assert.Greater(t, *after.Age, 0)
	assert.Equal(t, "Senior Go Developer", *after.Bio)
}

func TestNew_Go126_VsPtr_SameResult(t *testing.T) {
	//new(expr) and Ptr[T]() give the same result.
	//new(expr) is a built-in Go 1.26 syntax.
	//Ptr[T]() - compatible with Go 1.18+.

	//Method 1: Ptr (Go 1.18+)
	withPtr := p.Ptr(42)

	//Method 2: new(expr) (Go 1.26+)
	withNew := new(42)

	assert.Equal(t, *withPtr, *withNew, "the result is the same")
}

func TestNew_Go126_BuildProfile_AllThreeWays(t *testing.T) {
	//Three ways to create a UserProfile with pointer fields all give the same result:
	before := p.BuildProfileBefore()   //temporary variables (all versions of Go)
	withPtr := p.BuildProfileWithPtr() // Ptr[T]() (Go 1.18+)
	after := p.BuildProfileAfter()     // new(expr) (Go 1.26+)

	assert.Equal(t, *before.IsActive, *withPtr.IsActive)
	assert.Equal(t, *before.IsActive, *after.IsActive)

	assert.Equal(t, *before.Age, *withPtr.Age)
	assert.Equal(t, *before.Age, *after.Age)

	assert.Equal(t, *before.Bio, *withPtr.Bio)
	assert.Equal(t, *before.Bio, *after.Bio)
}

func TestNew_Go126_NewFromLiteralVsNewFromExpression(t *testing.T) {
	//Difference: new(T) - zero value, new(expr) - specific value
	zeroInt := new(int) // *int = 0 (zero value)
	valueInt := new(42) //*int = 42 (specific value)

	assert.Equal(t, 0, *zeroInt, "new(int) — zero value")
	assert.Equal(t, 42, *valueInt, "new(42) - specific value")

	//NewFromExpression via function from new126.go
	result := p.NewFromExpression(10, 5)
	require.NotNil(t, result)
	assert.Equal(t, 15, *result)
}

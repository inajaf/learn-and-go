package pointers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	p "learning_path/11_pointers"
)

// ─────────────────────────────────────────────────────────────────
//PART 1: Basic & and * operations
// ─────────────────────────────────────────────────────────────────

func TestBasicPointer_AddressAndDereference(t *testing.T) {
	//& - take address
	x := 42
	ptr := &x
	assert.Equal(t, 42, *ptr, "dereferencing should give the original meaning")

	//* — change the value by address
	*ptr = 100
	assert.Equal(t, 100, x, "x must change via pointer")
	assert.Equal(t, 100, *ptr, "ptr should also show the new value")
}

func TestBasicPointer_TwoPointersOneValue(t *testing.T) {
	//Two pointers to the same x - both see the change
	x := 10
	p1 := &x
	p2 := &x

	*p1 = 99
	assert.Equal(t, 99, *p2, "p2 sees the change through p1 - both look at x")
}

func TestBasicPointerDemo(t *testing.T) {
	result := p.BasicPointerDemo()
	assert.Contains(t, result, "x = 42")
	assert.Contains(t, result, "after *p = 100: x = 100")
}

// ─────────────────────────────────────────────────────────────────
//PART 2: Value vs Pointer when passed to a function
// ─────────────────────────────────────────────────────────────────

func TestValueVsPointer(t *testing.T) {
	byValue, byPointer := p.ValueVsPointerDemo()

	assert.Equal(t, 10, byValue,
		"pass by value: original unchanged")
	assert.Equal(t, 20, byPointer,
		"pointer transfer: original doubled")
}

// ─────────────────────────────────────────────────────────────────
//PART 3: Optional fields in the structure
// ─────────────────────────────────────────────────────────────────

func TestOptionalFields_NilMeansAbsent(t *testing.T) {
	profile := p.UserProfile{
		ID:       1,
		Username: "alice",
		//Age, IsActive, Bio - not specified (nil)
	}

	assert.Nil(t, profile.Age, "nil = age not specified")
	assert.Nil(t, profile.IsActive, "nil = status not specified")

	age := 30
	active := true
	profile.Age = &age
	profile.IsActive = &active

	assert.Equal(t, 30, *profile.Age)
	assert.Equal(t, true, *profile.IsActive)
}

func TestOptionalFields_ZeroIsNotNil(t *testing.T) {
	//Critically important: 0 and nil are DIFFERENT things
	zero := 0
	profile := p.UserProfile{Age: &zero}

	require.NotNil(t, profile.Age, "age is given (0 ≠ nil)")
	assert.Equal(t, 0, *profile.Age, "age is 0")
}

func TestApplyUpdate_PartialUpdate(t *testing.T) {
	age := 25
	active := true
	bio := "Go developer"

	profile := p.UserProfile{
		ID:       1,
		Username: "bob",
		Age:      &age,
		IsActive: &active,
		Bio:      &bio,
	}

	//We update only the age
	newAge := 26
	req := p.UpdateProfileRequest{
		Age: &newAge, //only age
		//IsActive and Bio = nil → do not touch
	}

	p.ApplyUpdate(&profile, req)

	assert.Equal(t, 26, *profile.Age, "age updated")
	assert.Equal(t, true, *profile.IsActive, "status has not changed")
	assert.Equal(t, "Go developer", *profile.Bio, "bio has not changed")
}

func TestApplyUpdate_NoChange(t *testing.T) {
	age := 30
	profile := p.UserProfile{Age: &age}

	//Empty request - we don’t change anything
	p.ApplyUpdate(&profile, p.UpdateProfileRequest{})
	assert.Equal(t, 30, *profile.Age, "no changes")
}

// ─────────────────────────────────────────────────────────────────
//PART 4: Ptr[T]() - a universal way to take the address of a literal
//
//&42, &true, &"hello" is a compiler error in all versions of Go.
//Ptr[T]() solves this elegantly and has worked since Go 1.18.
//new(expr) - Go 1.26 method, tests in new126_test.go.
// ─────────────────────────────────────────────────────────────────

func TestPtr_Generic_AllTypes(t *testing.T) {
	//Ptr[T]() - take the address of any value or expression
	pi := p.Ptr(42)
	pb := p.Ptr(true)
	ps := p.Ptr("hello")

	require.NotNil(t, pi)
	require.NotNil(t, pb)
	require.NotNil(t, ps)

	assert.Equal(t, 42, *pi)
	assert.Equal(t, true, *pb)
	assert.Equal(t, "hello", *ps)
}

func TestPtr_Generic_FromFunctionCall(t *testing.T) {
	//Ptr() accepts the result of a function call - analogous to new(f()) from Go 1.26
	add := func(a, b int) int { return a + b }

	result := p.Ptr(add(3, 4))
	require.NotNil(t, result)
	assert.Equal(t, 7, *result)
}

func TestNew_BasicUsage(t *testing.T) {
	//new(T) - standard Go: allocates memory, zero value, returns *T
	p1 := new(int)    // *int  = 0
	p2 := new(bool)   // *bool = false
	p3 := new(string) // *string = ""

	assert.Equal(t, 0, *p1)
	assert.Equal(t, false, *p2)
	assert.Equal(t, "", *p3)
}

func TestBuildProfile_BeforeAndWithPtr_SameResult(t *testing.T) {
	//BuildProfileBefore: temporary variables (old way)
	before := p.BuildProfileBefore()
	//BuildProfileWithPtr: Ptr[T]() generic (modern way for all versions of Go)
	withPtr := p.BuildProfileWithPtr()

	require.NotNil(t, before.IsActive)
	require.NotNil(t, withPtr.IsActive)
	assert.Equal(t, *before.IsActive, *withPtr.IsActive, "IsActive is the same")

	require.NotNil(t, before.Age)
	require.NotNil(t, withPtr.Age)
	assert.Equal(t, *before.Age, *withPtr.Age, "Age is the same")

	require.NotNil(t, before.Bio)
	require.NotNil(t, withPtr.Bio)
	assert.Equal(t, *before.Bio, *withPtr.Bio, "Bio is the same")
}

func TestBuildProfile_CalculateAge_Positive(t *testing.T) {
	//Checking that age is calculated correctly
	birthDate := time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC)
	age := p.CalculateAge(birthDate)
	assert.Greater(t, age, 0, "age must be positive")

	//Through Ptr - the same result as through a temporary variable
	ptrAge := p.Ptr(age)
	require.NotNil(t, ptrAge)
	assert.Equal(t, age, *ptrAge)
}

// ─────────────────────────────────────────────────────────────────
//PART 5: Value vs Pointer Methods
// ─────────────────────────────────────────────────────────────────

func TestPointerReceiver_MutatesOriginal(t *testing.T) {
	c := &p.Counter{}

	c.Increment()
	c.Increment()
	c.Increment()

	assert.Equal(t, 3, c.Value())

	c.Reset()
	assert.Equal(t, 0, c.Value())
}

func TestValueReceiver_DoesNotMutate(t *testing.T) {
	//BrokenCounter with value receiver - Increment does nothing!
	bc := p.BrokenCounter{}
	bc.IncrementBroken()
	bc.IncrementBroken()

	//The value has NOT changed - the method worked on a copy
	assert.Equal(t, 0, bc.Value(),
		"value receiver on a mutating method: the original does not change!")
}

func TestCounter_String(t *testing.T) {
	c := p.NewCounter("hits")
	c.Increment()
	c.Increment()
	assert.Equal(t, "Counter(hits)=2", c.String())
}

// ─────────────────────────────────────────────────────────────────
//PART 6: Mistakes and Pitfalls
// ─────────────────────────────────────────────────────────────────

func TestSafeDereference(t *testing.T) {
	//nil - safe
	val, ok := p.SafeDereference(nil)
	assert.Equal(t, 0, val)
	assert.False(t, ok)

	//not nil - get the value
	n := 42
	val, ok = p.SafeDereference(&n)
	assert.Equal(t, 42, val)
	assert.True(t, ok)
}

func TestPointerEquality(t *testing.T) {
	sameAddr, sameVal := p.PointerEquality()
	assert.True(t, sameAddr, "p1 and p2 point to the same x")
	assert.True(t, sameVal, "the values ​​are the same (*p1 == *p3)")
}

func TestLoopPointer_NoBug(t *testing.T) {
	ptrs := p.LoopPointerCorrect()
	require.Len(t, ptrs, 3)
	assert.Equal(t, 0, *ptrs[0])
	assert.Equal(t, 1, *ptrs[1])
	assert.Equal(t, 2, *ptrs[2])
}

// ─────────────────────────────────────────────────────────────────
//PART 7: Interfaces and receivers
// ─────────────────────────────────────────────────────────────────

func TestInterface_ValueVsPointerReceiver(t *testing.T) {
	results := p.InterfaceDemo()
	assert.Equal(t, "value:A", results[0])
	assert.Equal(t, "value:B", results[1])
	assert.Equal(t, "pointer:D", results[2])
}

// ─────────────────────────────────────────────────────────────────
//CHEET SHEET (tests-documentation)
// ─────────────────────────────────────────────────────────────────

func TestCheatSheet_Ampersand(t *testing.T) {
	//& - take the address (address-of operator)
	x := 10
	ptr := &x //ptr is *int, contains the address x
	assert.NotNil(t, ptr)
	assert.Equal(t, x, *ptr) //*ptr → x value
}

func TestCheatSheet_Star_InType(t *testing.T) {
	//* in type - declaration of "pointer to T"
	var ptr *int // nil
	assert.Nil(t, ptr)

	n := 55
	ptr = &n
	assert.Equal(t, 55, *ptr)
}

func TestCheatSheet_Star_InExpression(t *testing.T) {
	//* in expression - dereferencing
	n := 77
	ptr := &n

	*ptr = 88              //write via pointer
	assert.Equal(t, 88, n) //n has changed!

	got := *ptr //read through the index
	assert.Equal(t, 88, got)
}

func TestCheatSheet_PtrVsTemporaryVar(t *testing.T) {
	//Comparing two ways to take the address of a literal

	//The old way is a temporary variable:
	tmp := 42
	old := &tmp

	//The modern way (Go 1.18+) is Ptr[T]():
	modern := p.Ptr(42)

	assert.Equal(t, *old, *modern, "both methods give the same result")

	//Go 1.26 - new(expr), tests in new126_test.go
}

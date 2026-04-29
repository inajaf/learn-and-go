//Package pointers - A complete explanation of pointers in Go.
//
//The module covers:
//1. What are & and * - address and dereferencing
//2. When to pass a value when a pointer
//3. Pointers in structures - optional fields
//4. Ptr[T]() and new() - creating a pointer from a literal
//5. new(expr) in Go 1.26 - see file new126.go
//6. Methods on value vs pointer
//7. The most common mistakes with pointers
package pointers

import (
	"fmt"
	"time"
)

// ═══════════════════════════════════════════════════════════════
//PART 1: WHAT IS AN INDEX
// ═══════════════════════════════════════════════════════════════

//BasicPointerDemo demonstrates basic operation with & and *.
//Returns a string with explanations for the test.
func BasicPointerDemo() string {
	x := 42

	//& — take the address of a variable. Type p: *int ("pointer to int")
	p := &x

	result := fmt.Sprintf("x = %d\n", x)
	result += fmt.Sprintf("&x = %v (memory address x)\\n", p)
	result += fmt.Sprintf("*p = %d (value at address p)\\n", *p)

	//Changing x THROUGH the pointer
	*p = 100

	result += fmt.Sprintf("after *p = 100: x = %d (x has changed!)\\n", x)

	return result
}

// ═══════════════════════════════════════════════════════════════
//PART 2: PASS BY VALUE VS BY POINTER
//
//By value - the function receives a COPY. The original does not change.
//By pointer - the function receives the address. Changes the original.
// ═══════════════════════════════════════════════════════════════

//doubleByValue - accepts a copy, the original is not changed.
func doubleByValue(n int) {
	n *= 2 //we change only the local copy
	_ = n
}

//doubleByPointer - accepts an address, changes the original.
func doubleByPointer(n *int) {
	*n *= 2 //dereference and change the value at the address
}

//ValueVsPointerDemo shows the difference clearly.
func ValueVsPointerDemo() (byValue, byPointer int) {
	a := 10
	doubleByValue(a)
	byValue = a //still 10 - the copy has been changed, the original has not

	b := 10
	doubleByPointer(&b)
	byPointer = b //20 - the original has been changed via a pointer

	return
}

// ═══════════════════════════════════════════════════════════════
//PART 3: WHEN TO USE POINTER IN PARAMETERS
//
//✅ Use the pointer when:
//- The function must MODIFY the passed object
//- BIG structure (avoiding expensive copying)
//- You need to pass nil (no value)
//
//✅ Use the meaning when:
//- Small type (int, bool, small struct)
//- The function should NOT change data
//- Do you want to guarantee immutability within a function?
// ═══════════════════════════════════════════════════════════════

//SmallStruct is a small structure, better passed by value.
type SmallStruct struct {
	X, Y int
}

//LargeStruct is a large structure, it is better to pass it by pointer.
type LargeStruct struct {
	Data [1024]byte //1 KB - copying is expensive
	Name string
	ID   int
}

//TranslateByValue accepts SmallStruct by value: the copy is cheap, mutation is not needed.
func TranslateByValue(s SmallStruct, dx, dy int) SmallStruct {
	//We return a new structure - purely functional
	return SmallStruct{X: s.X + dx, Y: s.Y + dy}
}

//ProcessByPointer accepts LargeStruct by pointer: copy is expensive.
func ProcessByPointer(s *LargeStruct) {
	s.Name = "processed" //change the original - no copying
}

// ═══════════════════════════════════════════════════════════════
//PART 4: POINTERS IN STRUCTURES - optional fields
//
//If a field may be "missing" - use a pointer.
//nil = "field not set", not nil = "field is set (even if 0 or false)"
//
//This pattern is used in:
//- HTTP API (partial update: passed the field - update, no - skip)
//- DB (NULL values ​​- Module 10)
//- Configurations (optional parameters)
// ═══════════════════════════════════════════════════════════════

//UserProfile is a structure with optional fields.
type UserProfile struct {
	ID       int
	Username string
	//Pointer = optional. nil means "not set".
	//Without a pointer, it is impossible to distinguish "age 0" from "age not specified."
	Age      *int    //nil = not specified
	IsActive *bool   //nil = not specified
	Bio      *string //nil = not specified
}

//UpdateProfileRequest - DTO for partial update (Module 2: partial update).
//Pointer + omitempty: nil = "the client did not transmit this field."
type UpdateProfileRequest struct {
	Age      *int    //nil = do not update
	IsActive *bool   //nil = do not update
	Bio      *string //nil = do not update
}

//ApplyUpdate applies a partial update.
//Updates only fields that are not nil.
func ApplyUpdate(profile *UserProfile, req UpdateProfileRequest) {
	if req.Age != nil {
		profile.Age = req.Age //no need to dereference - just reassign pointer
	}
	if req.IsActive != nil {
		profile.IsActive = req.IsActive
	}
	if req.Bio != nil {
		profile.Bio = req.Bio
	}
}

// ═══════════════════════════════════════════════════════════════
//PART 5: HOW TO GET THE ADDRESS OF A LITERAL
//
//&42, &true, &"hello" is a compiler error in all versions of Go.
//Solutions:
//
//1. Temporary variable: tmp := 42; p := &tmp
//   2. Ptr[T]() — generic:     p := Ptr(42)          (Go 1.18+)
//3. new(expr) - Go 1.26: p := new(42) (Go 1.26+ only, see new126.go)
// ═══════════════════════════════════════════════════════════════

//Ptr returns a pointer to the passed value.
//A universal solution for all versions of Go since 1.18.
//
//Usage:
//
//	Ptr(42)       → *int
//	Ptr(true)     → *bool
//	Ptr("hello")  → *string
func Ptr[T any](v T) *T { return &v }

//CalculateAge calculates age from date of birth.
func CalculateAge(birthDate time.Time) int {
	now := time.Now()
	years := now.Year() - birthDate.Year()
	if now.YearDay() < birthDate.YearDay() {
		years--
	}
	return years
}

//BuildProfileBefore - creating a profile the old way (temporary variables).
func BuildProfileBefore() UserProfile {
	birthDate := time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC)

	//Method 1: Temporary Variables (Verbose)
	activeVal := true
	ageVal := CalculateAge(birthDate)
	bioVal := "Senior Go Developer"

	return UserProfile{
		ID:       101,
		Username: "Gopher",
		IsActive: &activeVal, //take the address of the temporary variable
		Age:      &ageVal,
		Bio:      &bioVal,
	}
}

//BuildProfileWithPtr - creating a profile via Ptr[T]() (Go 1.18+).
//Works in all versions of Go and is a clear IDE.
func BuildProfileWithPtr() UserProfile {
	birthDate := time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC)

	return UserProfile{
		ID:       101,
		Username: "Gopher",
		IsActive: Ptr(true),                    // *bool = true
		Age:      Ptr(CalculateAge(birthDate)), //*int = call result
		Bio:      Ptr("Senior Go Developer"),   // *string = "..."
	}
}

// ═══════════════════════════════════════════════════════════════
//PART 6: VALUE-BASED VS POINTER-BASED METHODS
//
//Value receiver (s Shape) - works on a copy. DOES NOT change the original.
//Pointer receiver (s *Shape) - works on the original. MAY change.
//
//Selection rule:
//- Method changes state → POINTER receiver
//- The structure is large (avoid copies) → POINTER receiver
//- The method only reads → VALUE receiver (if the structure is small)
//- One method with pointer → ALL others are also pointer (consistency)
// ═══════════════════════════════════════════════════════════════

//Counter - counter. All methods are on pointer receiver, because
//at least one method changes state (consistency rule).
type Counter struct {
	value int
	name  string
}

//NewCounter creates a counter with a name.
func NewCounter(name string) *Counter {
	return &Counter{name: name}
}

//Increment increases the counter - changes state → pointer receiver.
func (c *Counter) Increment() { c.value++ }

//Reset resets the counter - changes the state → pointer receiver.
func (c *Counter) Reset() { c.value = 0 }

//Value returns the current value - a pointer for consistency.
func (c *Counter) Value() int { return c.value }

//String formats the counter - pointer for consistency.
func (c *Counter) String() string {
	return fmt.Sprintf("Counter(%s)=%d", c.name, c.value)
}

//BrokenCounter shows a TYPICAL ERROR:
//value receiver on the method that tries to change the state.
type BrokenCounter struct{ value int }

//IncrementBroken - WRONG: value receiver does not change the original!
func (c BrokenCounter) IncrementBroken() {
	c.value++ //we change only the COPY
}

//Value returns a value that proves that increment did not work.
func (c BrokenCounter) Value() int { return c.value }

// ═══════════════════════════════════════════════════════════════
//PART 7: COMMON MISTAKES WITH POINTERS
// ═══════════════════════════════════════════════════════════════

//SafeDereference safely dereferences a pointer.
//Never dereference a pointer without checking for nil.
func SafeDereference(p *int) (value int, ok bool) {
	if p == nil {
		return 0, false
	}
	return *p, true
}

//LoopPointerCorrect - Correct capture of the loop variable.
//Explicit copy works correctly in all versions of Go.
func LoopPointerCorrect() []*int {
	result := make([]*int, 3)
	for i := range 3 {
		v := i //new variable at each iteration
		result[i] = &v
	}
	return result
}

//PointerEquality shows the difference between address and value equality.
func PointerEquality() (sameAddress, sameValue bool) {
	x := 42
	y := 42

	p1 := &x
	p2 := &x //same x
	p3 := &y //different y, same meaning

	sameAddress = (p1 == p2) //true: both → x
	sameValue = (*p1 == *p3) // true: 42 == 42
	return
}

// ═══════════════════════════════════════════════════════════════
//PART 8: POINTERS AND INTERFACES
// ═══════════════════════════════════════════════════════════════

//Stringer is a simple interface to demonstrate.
type Stringer interface{ String() string }

//ValueReceiver implements Stringer via value receiver.
//→ Both ValueReceiver and *ValueReceiver implement the interface.
type ValueReceiver struct{ Name string }

func (v ValueReceiver) String() string { return "value:" + v.Name }

//PointerReceiver implements Stringer via pointer receiver.
//→ The interface implements *PointerReceiver ONLY.
type PointerReceiver struct{ Name string }

func (p *PointerReceiver) String() string { return "pointer:" + p.Name }

//InterfaceDemo shows the difference in interface compatibility.
func InterfaceDemo() []string {
	var results []string

	var s1 Stringer = ValueReceiver{Name: "A"}  // ✅ value
	var s2 Stringer = &ValueReceiver{Name: "B"} //✅ pointer is OK too
	results = append(results, s1.String(), s2.String())

	//var s3 Stringer = PointerReceiver{Name: "C"} // ❌ does not compile!
	var s4 Stringer = &PointerReceiver{Name: "D"} //✅ pointer only
	results = append(results, s4.String())

	return results
}

// ═══════════════════════════════════════════════════════════════
//CHEET SHEET: WHEN & AND WHEN *
//
//& - “get address”:
//p := &x → p contains the address x
//f(&x) → pass address x to function
//&MyStruct{...} → create a structure and take the address
//
//* - two different meanings:
//1) In the type: *int, *MyStruct - "pointer to int/MyStruct"
//2) In the expression: *p - dereferencing (get the value by address)
//
//new(T) → allocates memory for T, returns *T (value = zero value)
//new(expr) → Go 1.26: allocates memory for the result of expr, returns *T
// ═══════════════════════════════════════════════════════════════

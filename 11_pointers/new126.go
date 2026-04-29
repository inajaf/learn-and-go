//go:build go1.26

//This file contains examples of new(expr), a new feature in Go 1.26.
//
//Build tag //go:build go1.26 ensures that the file compiles
//only with Go 1.26+. An IDE with an older analyzer will simply skip it.
package pointers

import "time"

//BuildProfileAfter - Build a profile with new(expr) from Go 1.26.
//
//Before Go 1.26, temporary variables were needed (see BuildProfileBefore)
//or generic Ptr[T]() (see BuildProfileWithPtr).
//
//Go 1.26 extended the new() built-in function to accept
//not only the type, but also any expression:
//
//new(42) → *int with value 42
//new(true) → *bool with the value true
//new(f(x)) → *T where T = return type f
func BuildProfileAfter() UserProfile {
	birthDate := time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC)

	//✅ Go 1.26: new(expr) - pointer directly from expression
	return UserProfile{
		ID:       101,
		Username: "Gopher",
		IsActive: new(true),                    // *bool = true
		Age:      new(CalculateAge(birthDate)), //*int = call result
		Bio:      new("Senior Go Developer"),   // *string = "..."
	}
}

//NewFromLiteral demonstrates all the variations of new(expr).
//Same result as Ptr[T](), but the syntax is built into the language.
func NewFromLiteral() (*int, *bool, *string) {
	return new(42), new(true), new("hello")
}

//NewFromExpression demonstrates new(expr) where expr is a function call.
func NewFromExpression(a, b int) *int {
	add := func(x, y int) int { return x + y }
	return new(add(a, b))
}

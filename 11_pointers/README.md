# Module 11 - Pointers: `&`, `*` and `new()` in Go 1.26

## 📌 What will you study

- What are `&` and `*` - address and dereferencing
- Passing by value vs by pointer - the difference and selection rules
- Pointers in structures - the "optional field" pattern
- `new()` before and after Go 1.26 - creating a pointer from an expression
- Methods on value vs pointer - when to use what
- The most common errors with pointers

---

## 🧠 Mental model

```
Computer memory is an array of cells, each with an address:

Address: 0x01 0x02 0x03 0x04 0x05
        ┌─────┬─────┬─────┬─────┬─────┐
Data:   │  42 │     │0x01 │     │     │
        └─────┴─────┴─────┴─────┴─────┘
           ↑           ↑
         x := 42     p := &x
(value) (address x - pointer)

x = 42 - variable, contains value
&x = 0x01 - address x in memory
p = 0x01 - p stores the address of x (p is a pointer)
*p = 42 - value at address p (dereference)
```

---

## 🔑 Operator `&` — take the address

```go
x := 42
p := &x // p — type *int, contains the ADDRESS of the variable x

fmt.Println(p) // 0xc0000b4010 (memory address)
fmt.Println(*p) // 42 (value at address)
```

`&` is read as **"address"**:
- `&x` → "address of variable x"
- `&MyStruct{...}` → "create a structure and take its address"

---

## 🔑 Operator `*` - two different meanings

### 1. In the type - declaration of the type "pointer to T"

```go
var p *int // p — pointer to int (still nil)
var s *MyStruct // s — pointer to MyStruct (nil for now)
func f(x *int) {} // function takes a pointer to int
```

### 2. In an expression - dereferencing (get/change value by address)

```go
x := 42
p := &x

value := *p // read: “value at address p” → 42
*p = 100 // write: “write 100 at address p” → x becomes 100
```

---

## 📊 Value vs Pointer - selection rules

```
PASS BY VALUE PASS BY POINTER
 ─────────────────────────        ────────────────────────
When?               The function DOES NOT change the data The function CHANGES the data
Small type (int, bool, Large structure
small struct) Need nil (absent)
Example func area(r Rect) float64 func double(n *int)
What the COPY function gets - the original is intact ADDRESS - changes the original
```

```go
// By VALUE - the function receives a copy, the original does not change:
func doubleByValue(n int) {
n *= 2 // copy only, caller will not see changes
}

a := 10
doubleByValue(a)
fmt.Println(a) // 10 - unchanged!

// BY POINTER - the function changes the original:
func doubleByPointer(n *int) {
*n *= 2 // change the value at address
}

b := 10
doubleByPointer(&b)
fmt.Println(b) // 20 - changed!
```

---

## 🧩 Pointers in structures - Optional Fields

If a field may be **missing**, use a pointer.
`nil` = "not set", `&value` = "set (even if the value is 0 or false)".

```go
// ❌ PROBLEM without pointer:
type User struct {
Age int // it is impossible to distinguish between "0 years" and "age not specified"
}

// ✅ SOLUTION with pointer:
type User struct {
Age *int // nil = not specified, &0 = specified and equal to zero
}
```

### Partial Update pattern (from Module 2):

```go
type UpdateRequest struct {
Age *int // nil = "not transmitted - do not update"
IsActive *bool // nil = "not transmitted - do not update"
}

func ApplyUpdate(user *User, req UpdateRequest) {
    if req.Age != nil {
user.Age = req.Age // update only if passed
    }
    if req.IsActive != nil {
        user.IsActive = req.IsActive
    }
}
```

---

## 🆕 `new()` - Before and After Go 1.26

### Problem: `&literal` doesn't work in Go

```go
// ❌ Compilation errors:
p := &42           // cannot take the address of 42
p := &true         // cannot take the address of true
p := &getAge()     // cannot take the address of getAge()
```

### Before Go 1.26, there are three workarounds:

```go
// Method 1: temporary variable (verbose)
tmp := 42
p := &tmp

// Method 2: helper function (separate code required)
func intPtr(v int) *int { return &v }
p := intPtr(42)

// Method 3: new(T) - only zero value
p := new(int) // *int = 0, not 42
```

### After Go 1.26 - `new(expr)`:

```go
// ✅ Go 1.26: new accepts an expression, not just a type
p := new(42)           // *int = 42
b := new(true)         // *bool = true
s := new("hello")      // *string = "hello"
age := new(getAge()) // *int = result of the function call
```

### Real example from Go 1.26 release notes:

```go
// ❌ Up to 1.26:
birthDate := time.Date(1986, 10, 1, 0, 0, 0, 0, time.UTC)
activeVal := true
ageVal := calculateAge(birthDate)
return UserProfile{
IsActive: &activeVal, // temporary variable
Age: &ageVal, // temporary variable
}

// ✅ After 1.26:
birthDate := time.Date(1986, 10, 1, 0, 0, 0, 0, time.UTC)
return UserProfile{
IsActive: new(true), // directly
Age: new(calculateAge(birthDate)), // directly from the call
}
```

---

## ⚙️ Methods: Value Receiver vs Pointer Receiver

```go
type Counter struct { value int }

// VALUE receiver - works on a COPY:
func (c Counter) Value() int { return c.value } // read only - OK

// POINTER receiver - works on the ORIGINAL:
func (c *Counter) Increment() { c.value++ } // change - we need a pointer
func (c *Counter) Reset() { c.value = 0 } // change - we need a pointer
```

### Receiver selection rule:

| Condition                                         | Receiver               |
|---------------------------------------------------|------------------------|
| Method **changes** state                          | `*T` (pointer)         |
| Structure **large**                               | `*T` (pointer)         |
| There is already another method with `*T`         | `*T` (for consistency) |
| The method only **reads**, the structure is small | `T` (value)            |

```go
// ❌ TRAP: value receiver does not change the original!
func (c Counter) BrokenIncrement() {
c.value++ // copy only! c outside will not change
}

c := Counter{}
c.BrokenIncrement()
fmt.Println(c.value) // 0 - unchanged!
```

---

## ⚠️ Pointers and interfaces

```go
type Stringer interface { String() string }

// Value receiver → interface implements both T and *T:
type A struct{}
func (a A) String() string { return "A" }

var _ Stringer = A{}   // ✅ OK
var _ Stringer = &A{}  // ✅ OK

// Pointer receiver → interface implements ONLY *T:
type B struct{}
func (b *B) String() string { return "B" }

var _ Stringer = &B{}  // ✅ OK
var _ Stringer = B{} // ❌ compiler: B does not implement Stringer
```

---

## 🐛 Common mistakes

### 1. Dereferencing nil - panic!

```go
var p *int
fmt.Println(*p)  // panic: runtime error: nil pointer dereference

// ✅ Always check:
if p != nil {
    fmt.Println(*p)
}
```

### 2. Erroneous comparison of pointers

```go
x, y := 42, 42
px, py := &x, &y

px == py // false: different addresses!
*px == *py // true: same values

// Comparing addresses or values ​​- these are different things!
```

### 3. Returning a pointer to a local variable is SAFE in Go

```go
// This is NORMAL in Go - the compiler will allocate the variable on the heap:
func newInt(v int) *int {
return &v // ✅ safe: Go will move v to the heap itself
}
// (In C/C++ this would be an error - dangling pointer)
```

---

## 🗺️ Cheat sheet - everything in one place

```go
// Announcement
var p *int // nil pointer to int
p := new(int)        // *int = 0 (zero value)
p := new(42)         // *int = 42 (Go 1.26+)

// Get the address
p := &x // variable address
p := &MyStruct{...} // structure address

// Dereferencing
value := *p // read the value at address
*p = 42 // write value to address

// Examination
if p == nil { } // check before dereferencing

// Type in declaration
func f(p *int) { } // accepts a pointer
func f() *int { } // returns a pointer

// Receiver
func (s *MyStruct) Method() { } // pointer receiver - can change s
func (s MyStruct) Method() { } // value receiver - works on a copy
```

---

## 📁 Module files

| File               | What does                                                  |
|--------------------|------------------------------------------------------------|
| `pointers.go`      | All examples with comments: `&`, `*`, `new()`, receivers   |
| `pointers_test.go` | 23 tests - each demonstrating a specific situation         |

---

## ▶️ Launch

```bash
go test ./11_pointers/... -v
```

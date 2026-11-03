# 01. Go prerequisites for a TypeScript developer

**Goal of this note:** read and write basic Go without stopping at every line. Each section shows the TypeScript version first, then Go.

## 1. The mental model

| TypeScript / Node | Go |
|---|---|
| Compiled to JavaScript, run by Node | Compiled to one native binary, no runtime to install |
| Types are erased at runtime | Types exist at runtime and are checked at compile time |
| Classes with inheritance | Structs with methods; no inheritance |
| Exceptions (`throw`, `try/catch`) | Errors are return values (`value, err`) |
| `async/await`, Promises | Plain blocking calls plus goroutines (`go f()`) and channels |
| `null` and `undefined` | `nil` for pointers, slices, maps, interfaces, functions, channels; everything else has a **zero value** |
| Modules with `import`/`export` | Packages (one folder = one package); capitalised names are exported |

The language is deliberately small. There is one loop keyword (`for`), no `while`, no ternary, no classes, no overloading, and one formatter that decides style (`gofmt`).

## 2. Variables and types

```ts
// TypeScript
const name = "Ali";            // string, can't be reassigned
let fee: number = 12000;       // number (float!)
let paid = false;              // boolean
```
```go
// Go
name := "Ali"                  // short declaration, type inferred (only inside functions)
var fee int64 = 1200000        // explicit type, int64 for money in paisa
var paid bool                  // no value given -> zero value false
const Currency = "PKR"         // constants
```
- `:=` declares and assigns. `=` only assigns. You cannot use `:=` outside a function; use `var` there.
- Go has specific number types: `int`, `int64`, `float64`, and so on. For money we always use `int64`, never a float.
- **Zero values** replace `undefined`. `var s Student` gives every field its zero value (`""`, `0`, `false`, `nil`). This is why a struct is usable immediately, but it also means "forgot to set" hides as zero, so validate.

## 3. Functions and the "name then type" order

```ts
function add(a: number, b: number): number { return a + b; }
const add2 = (a: number, b: number): number => a + b;
```
```go
func add(a int, b int) int { return a + b }
func add2(a, b int) int   { return a + b }   // shared type shorthand
```
Go writes the **name first, then the type**. This is the most common source of confusion, so read `a int` as "a is an int".

### Multiple return values replace exceptions
```ts
async function findStudent(id: string): Promise<Student> {
  const row = await db.query(...);
  if (!row) throw new Error("not found");   // errors thrown
  return row;
}
```
```go
func findStudent(id string) (Student, error) {
    row, err := db.Query(id)
    if err != nil {
        return Student{}, fmt.Errorf("finding student %s: %w", id, err)
    }
    return row, nil
}
```
- Every fallible call returns `(result, error)`. You check `err != nil` right after it. This is verbose on purpose: failure paths are visible.
- `%w` wraps the error so callers can still inspect the original with `errors.Is` / `errors.As`. Think of it as `new Error(msg, { cause })`.
- Never ignore an error with `_` unless you have a reason you can write in a comment.

## 4. Structs instead of classes

```ts
interface Student { id: string; fullName: string; feePayerId?: string }
const s: Student = { id: "s1", fullName: "Ali Khan" };
```
```go
type Student struct {
    ID        string
    FullName  string
    FeePayerID string   // empty string means "none"; Go has no optional fields
}
s := Student{ID: "s1", FullName: "Ali Khan"}
```
- A struct is data. Behaviour is attached with methods (section 6 in the next note).
- Field names starting with a capital letter are exported (visible outside the package). `FullName` is public; `fullName` would be private.
- Structs are **copied on assignment**. `b := a` makes a copy. Node objects are references, which surprises people. Use pointers when you want to share or modify (see the next section).

## 5. Pointers, briefly

```ts
const a = { balance: 0 };
const b = a;           // same object
b.balance = 100;       // a.balance is now 100
```
```go
a := Account{Balance: 0}
b := a                 // copy
b.Balance = 100        // a.Balance is still 0

p := &a                // p is a pointer to a
p.Balance = 100        // a.Balance is now 100 (Go follows the pointer automatically)
```
- `&x` takes an address, `*T` is "pointer to T".
- Rule of thumb: use a pointer when a function must change the value, or when the struct is large. Otherwise pass by value.
- There is no pointer arithmetic. Go pointers are safe to use in everyday code.

## 6. Slices, maps and loops

```ts
const ids: string[] = [];
ids.push("a");
const fee = fees["grade-5"];          // undefined if missing
for (const id of ids) console.log(id);
ids.map(x => x.toUpperCase());
```
```go
var ids []string                      // nil slice, len 0, works with append
ids = append(ids, "a")                // append returns the new slice; always assign it back
fee, ok := fees["grade-5"]            // ok tells you whether the key exists
if !ok { /* missing */ }

for _, id := range ids {              // _ ignores the index
    fmt.Println(id)
}
// Go has no .map(); write a loop, or use the slices package for common helpers
```
- Maps must be created before writing: `m := make(map[string]int64)`. Writing to a nil map panics. This is a classic first-week bug.
- `for i := range n` (Go 1.22+) loops from 0 to n-1.
- `range` gives `(index, value)`. Use `_` for the one you don't need.

## 7. Control flow

```go
if fee > 0 {
    // ...
} else if fee == 0 {
    // ...
}

switch status {          // no fallthrough by default; no need for break
case "paid", "partial":
    // ...
default:
    // ...
}
```
- `if` takes an optional init statement: `if err := save(); err != nil { ... }`. The `err` exists only inside that `if`. This is idiomatic.
- Only `for` exists for loops. `for cond { }` is `while`. `for { }` is infinite.
- `==` compares values with no type coercion. `1 == "1"` is a compile error. There is no `===` because there is no `==` with coercion.

## 8. `defer` (the `finally` you will use constantly)

```go
rows, err := db.Query(ctx, sql)
if err != nil { return err }
defer rows.Close()   // runs when the function returns, even on early return or panic
```
Deferred calls run in reverse order when the function exits. Use them for cleanup right after acquiring a resource.

## 9. Strings and characters (matters for Urdu later)

- A Go `string` is a sequence of **bytes** (UTF-8). `len("Ali")` is 3, but `len("علی")` is 6 bytes, not 3 characters.
- To iterate characters, use `for _, r := range s` (runes) or `[]rune(s)`.
- Never slice strings by byte offsets when the text may be Urdu. This matters for names and receipts later.

## 10. Errors you will hit in week one

| Error | Meaning | Fix |
|---|---|---|
| `declared and not used: x` | You declared a variable and never read it | Use it or delete it (`_ = x` only if you must) |
| `"os" imported and not used` | Unused import | Remove it; your editor's "organise imports" does this |
| `no new variables on left side of :=` | `:=` with nothing new | Use `=` |
| `panic: assignment to entry in nil map` | Map not created with `make` | `m := make(map[K]V)` |
| `cannot use x (type int) as type int64` | Go never converts numeric types for you | `int64(x)` explicitly |
| `nil pointer dereference` | You used a nil pointer | Check `if p == nil` or make sure it is set |

## 11. Money example, tying it together

```go
// Money is paisa. 100 paisa = Rs 1.
type Money int64

func (m Money) String() string {
    return fmt.Sprintf("Rs %d.%02d", m/100, m%100)   // Rs 12000.00 for 1200000
}

func Add(a, b Money) Money { return a + b }  // no floating error possible
```
`Money` is a new type based on `int64`. You cannot accidentally add a plain `int64` to it without converting, which is the point.

## Read next
- A Tour of Go, sections 1 to 4: https://go.dev/tour/
- Effective Go: https://go.dev/doc/effective_go
- Next note: `02-context-and-cancellation.md`

# 04. Structs, methods and interfaces (Go's answer to classes)

## 1. Class vs struct + methods

```ts
// TypeScript
class FeeService {
  constructor(private store: FeeStore, private students: StudentLookup) {}

  async recordPayment(input: PaymentInput): Promise<Receipt> {
    const payer = await this.students.feePayer(input.studentId);
    return this.store.save(input, payer);
  }
}
```
```go
// Go
type FeeService struct {
    store    FeeStore
    students StudentLookup
}

func NewFeeService(store FeeStore, students StudentLookup) *FeeService {
    return &FeeService{store: store, students: students}
}

func (s *FeeService) RecordPayment(ctx context.Context, in PaymentInput) (Receipt, error) {
    payer, err := s.students.FeePayer(ctx, in.StudentID)
    if err != nil {
        return Receipt{}, fmt.Errorf("finding fee payer: %w", err)
    }
    return s.store.Save(ctx, in, payer)
}
```

Breaking the Go version down:
- `type FeeService struct { ... }` is the data. The fields are lowercase, so they are private to the package.
- `func (s *FeeService) RecordPayment(...)` is a **method**. `(s *FeeService)` is the **receiver**, like `this` but named, and written as the first parameter.
- `*FeeService` means the method works on a pointer. Use this to share state. Be consistent: if one method uses a pointer receiver, make them all pointer receivers.
- `NewFeeService` is the **constructor** by convention. Go has no `constructor`. It returns a pointer, `&FeeService{...}`.
- `private store: FeeStore` in the constructor is TypeScript's shorthand for fields. Go has no shorthand; you spell out the fields.

## 2. Interfaces: satisfied by behaviour

```ts
// TypeScript: you must declare that the class implements the interface
class StudentsModule implements StudentLookup {
  async feePayer(id: string): Promise<Guardian> { ... }
}
```
```go
// Go: no declaration. If the type has the methods, it satisfies the interface.
type StudentLookup interface {
    FeePayer(ctx context.Context, studentID string) (Guardian, error)
}

// students.Service has FeePayer with the same signature, so it satisfies StudentLookup
// without ever mentioning the word "implements".
```
- The compiler checks it when you pass the value somewhere that needs the interface.
- Interfaces are declared **by the consumer**. `fees` declares the small interface it needs. `students` does not know `fees` exists. This is why our modules stay independent.
- Keep interfaces small: one or two methods. Go's standard library uses tiny interfaces (`io.Reader` has one method).

## 3. Dependency injection without a container

```go
// cmd/api/main.go (the only place that knows every module)
studentsSvc := students.NewService(studentsStore)
feesSvc := fees.NewService(feesStore, studentsSvc) // studentsSvc satisfies StudentLookup
```
In NestJS you would use a decorator and a container. Here it is just function arguments. Easy to read, easy to test, nothing magic.

### Testing with a fake
```go
type fakeLookup struct{}

func (fakeLookup) FeePayer(ctx context.Context, id string) (Guardian, error) {
    return Guardian{Name: "Kamran Khan"}, nil
}

svc := fees.NewService(fakeStore, fakeLookup{})
```
Jest mocks become small hand-written fakes that satisfy the interface. No mocking library is needed.

## 4. Composition instead of inheritance

Go has no `extends`. You embed a struct to reuse its fields and methods:

```go
type Base struct {
    ID        string
    CreatedAt time.Time
}

type Student struct {
    Base              // embedded: Student.ID and Student.CreatedAt work directly
    FullName string
}
```
Think of it as "has these fields and methods", not "is a". Use it lightly.

## 5. `any`, type switches and when to avoid them
- `any` is Go's `unknown` (formerly `interface{}`). Avoid it in business code; prefer real types.
- A type switch is like narrowing with `instanceof`:
```go
switch v := value.(type) {
case string:
    fmt.Println("text", v)
case int64:
    fmt.Println("money", v)
}
```
- Generics exist (Go 1.18+) and are useful for slices and maps helpers. Keep business code concrete.

## 6. Rules of thumb for this product
- Each module exposes a small set of types and a `Service` with methods that take `ctx`.
- Interfaces are defined where they are used, and only with the methods that user needs.
- Constructors (`NewX`) validate their inputs and return errors instead of half-built objects.

## Read next
- Effective Go, "Methods" and "Interfaces": https://go.dev/doc/effective_go
- Go Proverbs (short, memorable rules): https://go-proverbs.github.io/
- Next note: `05-runtime-lifecycle-and-goroutines.md`

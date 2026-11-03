# 02. `context.Context`: what it is and why every function takes it

**Why it confuses people:** in Node, a request's data and cancellation live implicitly on `req` or in `AsyncLocalStorage`. In Go they are passed **explicitly** as the first argument of every function that does I/O. Once you see the three jobs, the signature stops looking strange.

## 1. The three jobs of a context

| Job | Node equivalent | Example in this product |
|---|---|---|
| **Cancellation**: "stop working, the caller gave up" | `AbortSignal` passed around | The browser closed the tab; the DB query stops |
| **Deadline / timeout**: "finish within 5 s" | `AbortSignal.timeout(5000)` | Payment webhook processing must finish quickly |
| **Request-scoped values**: data for this request only | `req.locals` / `AsyncLocalStorage` | The tenant ID and the acting user |

```ts
// Node: the signal is an extra argument you must remember to pass
async function getStudent(id: string, signal: AbortSignal) {
  return db.query("select ...", [id], { signal });
}
```
```go
// Go: the context is the first argument, by convention, in every I/O function
func (s *Service) GetStudent(ctx context.Context, id string) (Student, error) {
    return s.store.Student(ctx, id)
}
```

## 2. Creating contexts

```go
ctx := context.Background()              // root: used in main, tests, and startup code

ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()                           // always call cancel to release resources

ctx, cancel = context.WithCancel(ctx)    // cancel it yourself when needed
```
- `context.Background()` is the empty root. Never invent contexts deep inside code.
- Every `WithX` returns a **child** context and a `cancel` function. Forgetting `defer cancel()` leaks resources. `go vet` warns about this.

## 3. Where a request's context comes from

`net/http` creates one context per incoming request. It is cancelled when the client disconnects or the handler returns.

```go
func (h *Handler) GetStudent(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()                    // the request's context
    st, err := h.svc.GetStudent(ctx, r.PathValue("id"))
    // ...
}
```
In Node you would write `req.signal`. Same idea.

## 4. Passing values: the tenant example

Our rule: the tenant comes from the token, set by middleware, and read by everything below it. Context values are the right tool for this, used sparingly.

```go
// internal/platform/tenant/tenant.go
package tenant

import "context"

type ctxKey struct{}              // unexported type: no other package can collide with this key

func With(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, ctxKey{}, id)
}

func From(ctx context.Context) (string, bool) {
    id, ok := ctx.Value(ctxKey{}).(string)
    return id, ok
}
```
Middleware calls `tenant.With`; the store calls `tenant.From` and fails if it is missing. In TypeScript this is like `AsyncLocalStorage.getStore()`, but explicit.

## 5. Rules to follow

1. **First parameter**, named `ctx`. Always.
2. **Never store a context in a struct.** Pass it down each call.
3. **Use context values only for request-scoped data** (tenant, user, request ID), never for optional parameters. Those belong in function arguments.
4. **Never pass `nil`.** Use `context.TODO()` if unsure, and fix it later.
5. **Check it in long loops and waits**: `if err := ctx.Err(); err != nil { return err }`, or use `select` with `<-ctx.Done()`.
6. Pass the context to the database driver (`pgx`) so queries stop when the request is cancelled.

## 6. What a cancelled request looks like

```go
// Mock: a slow fee calculation that respects cancellation
func calculateDues(ctx context.Context, studentID string) (int64, error) {
    for _, challan := range loadChallans(studentID) {
        select {
        case <-ctx.Done():               // the caller gave up or the deadline passed
            return 0, ctx.Err()         // context.Canceled or context.DeadlineExceeded
        default:
        }
        // ... add challan.Amount
    }
    return total, nil
}
```
- `select` with `default` checks the channel without blocking. `ctx.Done()` returns a channel that closes when cancelled.

## 7. Common mistakes
- Creating `context.Background()` inside a handler instead of using `r.Context()`. The request's cancellation is then lost.
- Not calling `cancel()`, which leaks a timer.
- Using context values as a bag of arguments.

## Read next
- Go blog, "Go Concurrency Patterns: Context": https://go.dev/blog/context
- Next note: `03-packages-modules-and-imports.md`

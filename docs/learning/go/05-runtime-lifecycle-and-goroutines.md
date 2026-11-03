# 05. Program lifecycle, the runtime and goroutines

## 1. From source to running process

```
Node:  node_modules (hundreds of MB) + your .ts -> (transpile) .js -> node runs it on V8
Go:    .go files -> go build -> ONE binary (a few MB) -> runs directly on the OS
```
- `go run ./cmd/api` compiles to a temporary binary and runs it. `go build -o bin/api ./cmd/api` produces the binary that will go into a Docker image.
- The binary contains the runtime (garbage collector and scheduler). Production servers need no Node, no `node_modules`, and no `npm install` step.
- Compilation is fast, and type errors appear before anything runs.

## 2. What happens when a Go program starts

1. Every imported package is initialised first, in dependency order. Package-level variables are set and `init()` functions run.
2. `main.main()` starts.
3. `main` starts other goroutines and then usually blocks.
4. When `main` returns, the program exits, **even if other goroutines are still running**. This is why graceful shutdown must wait explicitly.

Node comparison: the process runs the module top to bottom, then the event loop keeps it alive while there is pending work. Go has no implicit "keep alive" by pending work. `main` decides when the process ends.

## 3. Goroutines versus the Node event loop

| | Node.js | Go |
|---|---|---|
| Model | One thread runs JavaScript; I/O is asynchronous through the event loop | The runtime schedules many lightweight **goroutines** over a few OS threads |
| Create a unit of concurrency | `Promise` / `async` function | `go f()` |
| Waiting for I/O | `await` yields to the event loop | Goroutine blocks; the runtime runs another one |
| CPU-heavy work | Blocks the single thread (use a worker thread) | Runs in parallel on other cores |
| Cost | Cheap | Also cheap: a goroutine starts with a few KB of stack |

Go code looks **synchronous**. You do not write `async` or `await` everywhere. A blocking database call only parks its own goroutine, so other requests keep running. This is why Go handlers read like plain code.

```go
// Looks sequential. It is safe and concurrent across requests.
rows, err := db.Query(ctx, "select ...")
```

## 4. A goroutine in our `main.go`

```go
go func() {
    log.Info("listening", "addr", addr)
    if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Error("server failed", "err", err)
        stop()
    }
}()
```
- `go func() { ... }()` starts a function concurrently and returns immediately.
- `ListenAndServe` blocks while serving, so it must run in its own goroutine. Otherwise `main` would never reach shutdown code.
- `ErrServerClosed` is the normal result of `Shutdown`, so it is not treated as an error.

## 5. Waiting for a signal (`<-ctx.Done()`)

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
<-ctx.Done()           // blocks until Ctrl-C or SIGTERM (or stop() was called)
```
- `<-ch` receives from a channel and blocks until a value (or close) arrives.
- `ctx.Done()` is a channel that closes on cancellation. This is the Go version of `process.on('SIGTERM', ...)`.

## 6. Channels in one minute

```go
results := make(chan string)     // unbuffered: sender waits for a receiver

go func() {
    results <- "receipt-123"     // send
}()

msg := <-results                  // receive; blocks until available
```
Analogy: a Node `EventEmitter` or a queue of messages, but typed and blocking. Use channels to hand work or results between goroutines. Use a `sync.WaitGroup` to wait for a set of goroutines to finish.

## 7. The rule that prevents goroutine leaks
**Every goroutine needs an owner and a way to stop.** A goroutine that waits forever on a channel nobody writes to is a leak, and it never shows as an error. Pass a `ctx` and select on `ctx.Done()`.

## 8. Data races
Two goroutines writing the same variable without coordination is a **data race**. The result is undefined. Go gives you:
- `go test -race ./...` detects races while tests run. We run it in CI. (Note: it needs cgo; see the getting-started note.)
- `sync.Mutex` for protecting shared memory, or channels to pass ownership.
- Stateless handlers: each request gets its own variables, so most of our code has no race at all.

## 9. Lifecycle of one HTTP request at runtime

```
client TCP connect
  -> net/http accepts the connection and starts a goroutine for it
  -> reads the request line and headers
  -> ServeMux matches "GET /healthz" to a handler
  -> middleware runs (request ID, logging, auth, tenant)
  -> handler decodes JSON, calls the service with r.Context()
  -> service calls the store; the store waits on Postgres (goroutine parks)
  -> handler writes the JSON response
  -> connection is kept alive for the next request, or closed
```
The architecture note in `docs/learning/architecture/01-three-tier-and-request-flow.md` shows this with a real payment.

## 10. Lifecycle of a shutdown (what our `main.go` does)
1. Ctrl-C or SIGTERM arrives (blue-green deploys send SIGTERM).
2. `ctx` is cancelled, so `<-ctx.Done()` unblocks.
3. `srv.Shutdown(ctx with 10 s)` stops accepting new connections and waits for in-flight requests.
4. If requests finish within 10 s, the program exits cleanly. Otherwise `Shutdown` returns an error, which we log.

Node equivalent: `server.close()` plus a timeout, with the `SIGTERM` handler.

## Read next
- Go by Example, goroutines and channels: https://gobyexample.com/
- The Go Memory Model (short, worth reading once): https://go.dev/ref/mem
- Next note: `06-http-server-routing-middleware.md`

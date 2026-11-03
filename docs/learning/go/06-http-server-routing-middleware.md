# 06. HTTP server, routing (`ServeMux`), handlers and middleware

## 1. Express vs `net/http`

| Express | Go (standard library) |
|---|---|
| `express()` | `http.NewServeMux()` (the router) |
| `app.get('/healthz', handler)` | `mux.HandleFunc("GET /healthz", handler)` |
| `(req, res, next)` | `(w http.ResponseWriter, r *http.Request)` (**writer first**) |
| `req.params.id` | `r.PathValue("id")` (Go 1.22+) |
| `req.body` (after `express.json()`) | `json.NewDecoder(r.Body).Decode(&v)` |
| `res.status(201).json(obj)` | `w.WriteHeader(201)` then `json.NewEncoder(w).Encode(obj)` |
| `app.use(mw)` | a function that wraps the next handler (section 4) |
| `app.listen(8080)` | `srv := &http.Server{Addr: ":8080", Handler: mux}`, then `ListenAndServe()` |
| `server.close()` | `srv.Shutdown(ctx)` |

**What is `ServeMux`?** It is the router, the piece that looks at method and path and chooses a handler, like Express's routing table. Since Go 1.22 it understands methods (`GET`) and path variables (`{id}`), so Express is not needed for most routing.

**What is a "handler"?** Anything with a method `ServeHTTP(w, r)`. A plain function with the same signature is wrapped automatically by `HandleFunc`.

## 2. A complete handler (planned shape)

```go
// internal/students/http.go
func (h *Handler) GetStudent(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")                                  // route: /api/v1/students/{id}

    st, err := h.svc.GetStudent(r.Context(), id)
    if err != nil {
        httpx.WriteError(w, err)                            // planned helper: maps errors to the API error format
        return
    }
    httpx.WriteJSON(w, http.StatusOK, st)                   // planned helper
}
```
Compare with Express:
```ts
app.get("/api/v1/students/:id", async (req, res, next) => {
  try {
    const st = await svc.getStudent(req.params.id, req.signal);
    res.status(200).json(st);
  } catch (err) { next(err); }
});
```
- Go has no `try/catch` middleware for you. Errors are returned, checked, and mapped in one place (`httpx`), which keeps the error format consistent (REQ-API-011).
- **Order matters:** headers and status must be set **before** the body is written. `WriteHeader` called after `Write` is ignored.

## 3. Reading JSON input

```go
var in CreateStudentRequest
dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)) // limit body size to 1 MiB
dec.DisallowUnknownFields()                                    // reject fields we do not know
if err := dec.Decode(&in); err != nil {
    httpx.WriteError(w, httpx.BadRequest("invalid JSON body"))
    return
}
```
- `Decode(&in)` takes a pointer so the decoder can fill the struct.
- `DisallowUnknownFields` and a size limit are good defaults for a public API. In Express you would add `express.json({ limit })` and a schema validator.

## 4. Middleware: the wrapping pattern

```go
// Logs every request and its duration.
func Logging(log *slog.Logger, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)                          // call the next handler in the chain
        log.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "duration_ms", time.Since(start).Milliseconds(),
        )
    })
}
```
Usage in `main.go`:
```go
handler := Logging(log, RequestID(mux))   // outer wraps inner
srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
```
Express: `app.use(logging); app.use(requestId); app.get(...)`. Same idea: each middleware calls `next`, but here the chaining is explicit function wrapping instead of a hidden list.

### Auth and tenant middleware (outline)
```go
func Auth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
        principal, err := tokens.Verify(r.Context(), token)  // token format is decided in design
        if err != nil {
            httpx.WriteError(w, httpx.Unauthorized())
            return
        }
        ctx := tenant.With(r.Context(), principal.TenantID)  // tenant from the token, never the body
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```
- `r.WithContext(ctx)` returns a copy of the request with the new context. Requests are not mutated.
- The tenant ID comes from the verified token, never from the request body (REQ-API-010 and the tenancy ADR).
- The token format (opaque vs signed) is still an open design decision, listed in the API conventions spec.

## 5. Server settings that protect you (already in `main.go`)
- `ReadHeaderTimeout`: a client that sends headers very slowly gets disconnected. Protects against slowloris-style attacks.
- Later add `ReadTimeout`, `WriteTimeout` and `IdleTimeout` once we know the longest legitimate requests (bulk imports go async, REQ-API-014).

## 6. Our `main.go`, line by line

```go
mux := http.NewServeMux()                      // the router (Express app)
mux.HandleFunc("GET /healthz", httpx.Health)   // register a route; method + path pattern

srv := &http.Server{                            // the HTTP server (Express app.listen)
    Addr:              addr,
    Handler:           mux,                     // every request goes through the router
    ReadHeaderTimeout: 5 * time.Second,
}

ctx, stop := signal.NotifyContext(...)          // ctx is cancelled on Ctrl-C / SIGTERM
go func() { srv.ListenAndServe() }()            // serve in the background
<-ctx.Done()                                    // main waits for the signal
srv.Shutdown(shutdownCtx)                       // stop accepting, finish in-flight, exit
```

## 7. Health endpoints (we will need two)
- **Liveness** (`/healthz`): "the process is alive". The orchestrator restarts it if this fails. Keep it cheap: no database calls.
- **Readiness** (`/readyz`, later): "the process can serve traffic". It checks the database and returns 503 while the pool is down. The load balancer uses this to stop sending traffic during deploys.

## 8. Testing a handler without a server

```go
rec := httptest.NewRecorder()
req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
httpx.Health(rec, req)
// rec.Code and rec.Body hold the response
```
Node equivalent: supertest. The test in `internal/platform/httpx/health_test.go` does exactly this.

## Read next
- Go blog, "Routing Enhancements for Go 1.22": https://go.dev/blog/routing-enhancements
- Let's Go (Alex Edwards), chapters on routing and middleware: https://lets-go.alexedwards.net/
- Next note: `07-tooling-formatting-and-linting.md`

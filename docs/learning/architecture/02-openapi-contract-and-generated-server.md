# 02. The OpenAPI contract and the generated Go server

**Slice:** housekeeping for the walking skeleton (tooling, route contract, generator).
**Read with:** `api/openapi/openapi.yaml`, `services/api/internal/api/server.go`, `services/api/internal/api/gen/api.gen.go`, ADR-0003.

## What changed and why
The spec said `/healthz` lived under `/api/v1`, but the code served it at the root. Spec and code disagreed, and nothing noticed. Now the spec is the contract and the code follows it:

1. `openapi.yaml` sets `servers: /`. Infrastructure paths (`/healthz`) sit at the root. Product paths will carry `/api/v1` themselves.
2. `make gen` runs `oapi-codegen` and writes `internal/api/gen/api.gen.go`. Nobody edits that file (a hook blocks it).
3. `api.Server` implements the generated `StrictServerInterface`. If the spec says a route exists and the method is missing, the build fails.

## Go concepts in this slice

**Interfaces are satisfied implicitly.** Node/TypeScript needs `implements`. In Go, `*api.Server` satisfies `gen.StrictServerInterface` just by having a method with the right signature (`server.go`). The compiler checks it when `NewStrictHandler(api.NewServer(), nil)` is called in `main.go`.

**Typed responses instead of `res.status(200).json()`.** The handler returns `gen.GetHealth200JSONResponse{Status: "ok"}`. A handler cannot return a status code the spec does not declare. In Express you could send any status from anywhere.

**Go 1.22 routing.** `http.NewServeMux` accepts `"GET /healthz"` patterns, so the standard library routes by method and path (`std-http` in the generated code). No router library is needed (ADR-0001).

**Context first.** `GetHealth(_ context.Context, ...)` takes a context first, as the Go rules require for anything doing I/O. Health does no I/O yet, so the parameter is unused (`_`).

**Tests use the real router.** `server_test.go` builds the same handler `main.go` uses and calls `/healthz` through it. It does not call the method directly, so it proves the route the spec describes is the route served.

## Tool notes
- `make tools` pins four versions in the `Makefile`. These tools need Go 1.26 to build. Your Go is 1.23, so `go` downloads go1.26 on demand (`GOTOOLCHAIN`). The project's own `go.mod` stays at 1.23.
- CI runs `make gen` and then `git diff --exit-code`, so a stale generated file fails the build.

## Check it yourself
```bash
make gen && git status --short services/api/internal/api   # no output: generated code is current
make check                                                   # lint + tests
curl -i http://localhost:8080/healthz                        # 200, {"status":"ok"}
curl -i http://localhost:8080/api/v1/healthz                 # 404 (not in the spec)
```

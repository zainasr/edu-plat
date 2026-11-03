# 07. Tooling: formatting, vetting, linting and what `make check` runs

## 1. Node vs Go tools, side by side

| Job | Node / TypeScript | Go | Notes |
|---|---|---|---|
| Compile and type-check | `tsc --noEmit` | `go build ./...` / `go vet` | Go's compiler is strict: unused vars and imports are errors |
| Formatting | Prettier | `gofmt` (built in) | No config. One style for everyone. |
| Basic correctness checks | ESLint core rules | `go vet` (built in) | Printf argument mismatches, copied locks, unreachable code |
| Full linting | ESLint + plugins | `golangci-lint` (runs ~50 linters) | Config in `.golangci.yml` |
| Unit tests | Jest / Vitest | `go test` (built in) | Test files end with `_test.go` |
| Race detector | none built in | `go test -race` | Finds real data races at run time |
| Dependency vulnerabilities | `npm audit` | `govulncheck` | Checks only code paths you actually call |
| Dependency tidy | `npm prune` | `go mod tidy` | Keeps `go.mod` and `go.sum` accurate |
| Docs | TypeDoc | `go doc` / pkg.go.dev | Doc comments start with the name: `// Health reports ...` |

## 2. What each one catches, with examples

```go
fmt.Printf("%d", "Ali")        // go vet: wrong verb for a string
var mu sync.Mutex
copyMu := mu                   // go vet: copies a lock (a classic concurrency bug)
x := 1                         // compiler: declared and not used
```

```ts
// ESLint equivalents (for intuition)
const unused = 1;                 // @typescript-eslint/no-unused-vars
if (x = 1) {}                     // no-cond-assign
```

## 3. Formatting is not a debate in Go
- Run `gofmt -w .` (or let your editor do it on save). It rewrites files to the one standard style.
- Tabs for indentation, braces on the same line, imports sorted.
- Stop arguing about style: CI rejects unformatted code, which is what our `make lint` does.

## 4. `golangci-lint`: the ESLint of Go

- It runs many linters in one pass. Config lives in `.golangci.yml` at the module root.
- We will start with a small set: `errcheck` (unchecked errors), `govet`, `staticcheck` (bugs and simplifications), `ineffassign`, `unused`, `gosec` (security mistakes), `depguard` (forbidden imports, which can enforce module borders later).
- Turn a linter off only with a reason in a comment, for example `//nolint:gosec // reason`.
- Not installed yet. It is part of the start procedure in `docs/engineering/getting-started.md`.

## 5. What "linting" means (your definition, sorted)
Your definition was: formatting, standard practices, brackets, and so on. In practice:
- **Brackets and syntax:** the compiler, not a linter. You cannot build code with a syntax error.
- **Formatting:** `gofmt` (Prettier's role).
- **Standard practices and likely bugs:** `go vet` plus `golangci-lint` (ESLint's role).

## 6. What `make check` runs today

```make
check: lint test

lint:
    gofmt -l .        # lists files that are not formatted; the Makefile fails if any
    go vet ./...      # the built-in bug finder

test:
    go test ./...     # all unit tests
```
Later additions, as they are installed: `golangci-lint run`, `go test -race ./...`, `govulncheck ./...`, and the OpenAPI and sqlc generation check.

## 7. Editor setup (both give you these for free)
- **VS Code:** install the "Go" extension (by the Go team). It runs `gopls`, the language server, which gives completion, go-to-definition, rename and inline errors, similar to the TypeScript server.
- **GoLand:** works out of the box.
- `gopls` is the single engine behind editor support. Install it through the Go extension or with `go install golang.org/x/tools/gopls@latest`.

## Read next
- Go command reference: https://go.dev/doc/cmd
- golangci-lint linters list: https://golangci-lint.run/docs/linters/
- Next note: `08-observability-from-day-one.md`

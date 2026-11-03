# 03. Packages, modules and imports (`go.mod` vs `package.json`)

## 1. Vocabulary

| Go term | Node/TS equivalent | Notes |
|---|---|---|
| **Module** | An npm package (the whole project) | Defined by `go.mod` at the root of the module |
| **Package** | A folder that behaves like one ES module with `index.ts` | Every `.go` file in a folder belongs to the same package |
| `go.mod` | `package.json` (name, version of Go, dependencies) | Module path plus Go version plus requirements |
| `go.sum` | `package-lock.json` / `pnpm-lock.yaml` | Checksums for dependencies; commit it |
| `go mod tidy` | `npm prune` + install what is used | Adds missing and removes unused requirements |
| `go get` | `npm install` | Adds a dependency |
| `go build ./...` | `tsc` build of the whole project | Compiles every package |

## 2. `go.mod` in this repo

```
module github.com/zainasr/edu-plat/services/api

go 1.23
```
- The **module path** is how other packages import yours. It must match your real repo path before the first commit (it is a placeholder).
- `services/api/` has its own `go.mod`, so it is a separate module. Other folders (for example a future `tools/`) can be separate modules too. The API is one module, not one module per folder.

## 3. A package is a folder

```
services/api/internal/platform/httpx/
  health.go        package httpx
  health_test.go   package httpx
```
- Every file in a folder declares the same `package` name, at the top.
- `package main` is special. It marks a program with a `func main()`. `cmd/api/main.go` is one.
- The folder is the namespace. You call `httpx.Health`, and `Health` is exported because it starts with a capital letter.

## 4. Imports

```go
import (
    "encoding/json"                                                  // standard library
    "net/http"

    "github.com/zainasr/edu-plat/services/api/internal/platform/httpx"  // our package
)
```
```ts
import { Health } from "./platform/httpx"; // TypeScript equivalent of the last line
```
- Standard library imports look like `"net/http"`, with no domain name.
- Your own packages import by **full module path**, so the path is always unambiguous.
- Unused imports are compile errors. Your editor adds and removes them automatically.

## 5. Visibility: capital letters are the export keyword

| Name | Visible to |
|---|---|
| `Health` | Any package that imports `httpx` |
| `health` (lowercase) | Only code inside `httpx` |

There is no `private` or `public` keyword. Use it to design what a module exposes. In TypeScript you would write `export` on a few things and keep the rest local. Same idea, enforced by capitalisation.

## 6. `internal/`: the compiler-enforced border

```
services/api/internal/students/...
```
Code under any folder named `internal` can only be imported by code **inside the parent of that folder**. So `services/api/internal/...` cannot be imported by another repo or a random tool. Node has nothing equivalent; the nearest is a package's `exports` field in `package.json`.

Our layout relies on this. Each bounded context (`students`, `fees`, ...) lives under `internal/`, and the rest of the product talks to it through exported interfaces.

## 7. Import cycles are a compile error

If `fees` imports `students` and `students` imports `fees`, Go refuses to compile. Node tolerates cycles and gives you half-initialised values at runtime. The Go rule forces clean module borders, which is why the modular monolith works in Go.

## 8. `init()` runs before `main`

```go
package platform

func init() {
    // runs once, automatically, when the package is loaded
}
```
Avoid `init()` for business logic. It runs at import time and makes tests hard to control. Use it only for things like registering a driver. Our code wires dependencies explicitly in `main` instead.

## 9. Folder structure of this repo, annotated

```
services/api/
  go.mod                   module definition (like package.json)
  go.sum                   checksums (like a lockfile)
  cmd/api/main.go          program entry point: wires everything, no business rules
  internal/
    platform/              shared plumbing, no product rules
      httpx/               HTTP helpers (health, JSON writing, errors later)
      tenant/              tenant ID in context (later)
      db/                  connection pool (later)
    students/              one bounded context (later)
      students.go          public types and interface other modules may use
      service.go           business rules
      http.go              HTTP handlers (adapter)
      store/               sqlc-generated SQL access
  migrations/              SQL schema history (goose)
```
Rules to remember: `cmd/` contains start-up code only; `internal/` contains everything else; `platform/` never imports a product module.

## 10. Commands you will use every day

```bash
go mod tidy          # sync go.mod and go.sum with the code
go build ./...       # compile everything (no binary written to a folder you care about)
go test ./...        # run all tests
go doc net/http ServeMux    # like MDN or TypeDoc, from the terminal
go list ./...        # list packages
```

## Read next
- Go modules reference: https://go.dev/ref/mod
- Organising a Go module: https://go.dev/doc/modules/layout
- Next note: `04-structs-methods-and-interfaces.md`

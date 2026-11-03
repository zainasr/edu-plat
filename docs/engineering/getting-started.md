# Getting started: set up a machine and run the API

**Who this is for:** an engineer (or agent) starting work on the repo for the first time. Ubuntu 24.04 is the reference OS (matches the current machine). macOS notes are in brackets where they differ.

Work through the steps in order. Each step ends with a **check**: a command and what you should see. If a check fails, stop and fix it before continuing.

---

## 1. Tools and why each one exists

| Tool | Used for | Required now? |
|---|---|---|
| **Go 1.23** | Build and run the API, tests, lint | Yes |
| **Docker + Compose** | Local PostgreSQL (and later Redis) with no installs on the host | Yes |
| **make** | One entry point for `check`, `run`, `db-up` (see `Makefile`) | Yes |
| **git** | Version control | Yes |
| **psql** (PostgreSQL client) | Looking inside the database by hand | Recommended |
| **jq** | Reading JSON in the shell; used by the Claude hooks | Yes (hooks use it) |
| **gcc** | Needed by the race detector (`go test -race`) | Yes for CI parity |
| **golangci-lint** | Full linting (ESLint-equivalent for Go) | Yes, from the first PR |
| **sqlc** | Generates type-safe Go from SQL (`make gen`) | Yes, from the first module |
| **goose** | Runs SQL migrations | Yes, from the first module |
| **gopls** | Editor intelligence for Go | Yes (editor) |
| **Docker** (again) | Runs the Postgres MCP server for Claude (image pinned by digest in `.mcp.json`) | Yes for Claude |
| **Node 20+ and npm** | Web (Next.js) and Playwright MCP | Later for web; present now |
| **Claude Code** | The AI engineering harness this repo is set up for | Yes |

Not needed yet: Terraform, the React Native toolchain, Python. They arrive with their slices.

---

## 2. Install the base tools (Ubuntu)

### 2.1 Go
Go 1.23 is installed here (`go version` shows `go1.23.4`). Match the version in `services/api/go.mod`.

```bash
go version          # expect: go version go1.23.x linux/amd64
```
- If you need a different version, use the official download (https://go.dev/dl/) or `go install golang.org/dl/go1.23.4@latest`. Avoid the Ubuntu `golang` apt package for this project: it lags behind.
- [macOS: `brew install go@1.23`]

**Check:** `go env GOPATH` prints a path. Ensure `$(go env GOPATH)/bin` is on your `PATH`, or installed tools will "not be found".

```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc   # or ~/.zshrc
```

### 2.2 Docker and Compose
```bash
docker version && docker compose version
```
- If `docker` needs `sudo`, add yourself to the group and log out/in:
  ```bash
  sudo usermod -aG docker "$USER"
  ```
- Docker is present here (Compose v5.1.4).

### 2.3 make, git, jq, gcc
```bash
sudo apt-get update
sudo apt-get install -y make git jq build-essential
```
- `build-essential` installs gcc, which the race detector needs.

### 2.4 PostgreSQL client (psql)
This is the client only. The server runs in Docker.
```bash
sudo apt-get install -y postgresql-client
psql --version
```
- Note: a local PostgreSQL server on port 5432 would clash with the Docker one. Either stop the local service (`sudo systemctl stop postgresql`) or change the port in `deploy/docker-compose.yml` and `DATABASE_URL`.

### 2.5 Docker image for the Postgres MCP
The Postgres MCP runs from the `crystaldba/postgres-mcp` image, pinned by digest in `.mcp.json`. Docker pulls it on first use, so nothing else is installed on the host.
```bash
docker pull crystaldba/postgres-mcp      # optional: warm the cache
```
Why not `uvx postgres-mcp`: the PyPI package with that name is a different project, and the upstream Git tags pin `pglast` versions that are not published. See ADR-0010 if we revisit.

---

## 3. Install the Go developer tools

Versions are pinned in the `Makefile` (see ADR-0009), so every machine and CI run the same tools. Install them into `$HOME/go/bin` with:

```bash
make tools      # golangci-lint, sqlc, goose, oapi-codegen at the pinned versions
go install golang.org/x/tools/gopls@latest          # editor only, not pinned
```

The pinned tools need Go 1.26 to build. If your Go is 1.23, the `go` command downloads go1.26 automatically (`GOTOOLCHAIN`). That is expected.

**Checks:**
```bash
golangci-lint version     # expect: golangci-lint has version 2.x
sqlc version             # expect: v1.x or v2.x
goose -version           # expect: goose version: v3.x
gopls version            # expect: golang.org/x/tools/gopls v0.x
```
- If a command says "not found", the `$HOME/go/bin` PATH step (2.1) is missing.
- `gopls` is already present on this machine (v0.24.0).

---

## 4. Get the code and set the module path

The Go module path is `github.com/zainasr/edu-plat/services/api`, set in `services/api/go.mod` and in every import that uses it (`cmd/api/main.go`, `internal/platform/httpx/...`). If the repository ever moves, change those lines with:
```bash
grep -rl "zainasr/edu-plat" --include='*.go' --include='go.mod' services | xargs sed -i 's#github.com/zainasr/edu-plat#github.com/NEW-ORG/NEW-REPO#g'
cd services/api && go mod tidy && cd -
```

---

## 5. Configure the environment

```bash
cp .env.example .env
```
Open `.env` and check the values. **Important Go fact:** the Go program does **not** read `.env` by itself (unlike Node's `dotenv`). Environment variables must be exported in the shell:

```bash
set -a; source .env; set +a      # exports every variable in .env to this shell
```
Later we add a small loader or use `direnv`. Until then, source it per terminal session, or rely on the defaults in `main.go` (`HTTP_ADDR` defaults to `:8080`).

Never commit `.env`. `.gitignore` already excludes it, and `.env.example` documents the names.

---

## 6. Start the database and the API

```bash
make db-up                       # starts Postgres 16 in Docker on port 5432
docker compose -f deploy/docker-compose.yml ps   # expect: postgres ... running
```

Connect with psql to confirm the database accepts logins:
```bash
psql "postgresql://app:app@localhost:5432/app_dev" -c "select version();"
```
Expect a line starting with `PostgreSQL 16`.

Start the API in one terminal:
```bash
make run                 # listens on :8080 (go run ./cmd/api)
```
In a second terminal:
```bash
curl -i http://localhost:8080/healthz
```
Expect `HTTP/1.1 200 OK` and the body `{"status":"ok"}`. Stop the API with `Ctrl-C` and watch the shutdown log line (this is the graceful shutdown from note 05).

Stop the database when you finish: `make db-down`.

---

## 7. Run the checks

```bash
make check               # lint (gofmt + vet) + all tests
```
Expect `ok` for `internal/platform/httpx`. Two extra checks, run them when the tools are installed:
```bash
cd services/api && golangci-lint run ./...     # full lint (no config yet, so default linters)
cd services/api && go test -race ./...         # race detector (needs gcc)
```
`make check` will get `golangci-lint` and `-race` once the config is added with the first slice (decision recorded there).

---

## 8. Editor setup
- **VS Code:** install the Go extension (by the Go team). Open the repo root, not `services/api`, and accept the `gopls` install prompt.
- On save, it runs `gofmt` and organises imports. This matches `make lint`.
- Use "Go: Test Package" above a test function to run one test.

---

## 9. Claude Code setup (for this repo)
- Start with `claude` in the repo root. Project settings are in `.claude/settings.json`; the team's rules are in `CLAUDE.md` and `.claude/rules/`.
- Check the MCP servers with `/mcp`. The `playwright` server downloads its package on first run (`npx`), and the `postgres` server starts a Docker container. Press retry in `/mcp` if either times out. The database MCP needs `make db-up` first.
- Run `/context` to see what has loaded; `/doctor` to check the setup.
- Your personal overrides go in `.claude/settings.local.json` and `CLAUDE.local.md` (both ignored by git).

---

## 10. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `command not found: golangci-lint` | `$HOME/go/bin` not on PATH | Step 2.1 |
| `bind: address already in use` on :8080 | Another process uses 8080 | `HTTP_ADDR=:8081 make run` |
| Compose error on port 5432 | Local PostgreSQL running | Step 2.4 note |
| `permission denied` on Docker socket | User not in `docker` group | Step 2.2 |
| `go: go.mod requires go >= 1.23` | Older Go installed | Step 2.1 |
| `cgo: C compiler not found` with `-race` | No gcc | Step 2.3 |
| Lint says `declared and not used` | Real error, not noise | Fix or remove the variable |
| `make check` fails only on formatting | Unformatted file | `gofmt -w services/api` then rerun |

---

## 11. First-week reading (in this order)
1. `docs/learning/go/01-prerequisites-typescript-to-go.md`
2. `docs/learning/go/02-context-and-cancellation.md`
3. `docs/learning/go/03-packages-modules-and-imports.md`
4. `docs/learning/go/05-runtime-lifecycle-and-goroutines.md`
5. `docs/learning/go/06-http-server-routing-middleware.md`
6. `docs/learning/architecture/01-three-tier-and-request-flow.md`

Then read `CLAUDE.md`, `docs/engineering/repo-layout.md` and the ADRs in `docs/adr/`.

.PHONY: check test lint run db-up db-down db-roles migrate gen tools evaluate test-hooks spec-coverage
API := services/api

# Pinned developer tools. Bump a version here and in the same PR, then run `make tools`.
GOLANGCI_LINT_VERSION := v2.15.0
SQLC_VERSION          := v1.31.1
GOOSE_VERSION         := v3.29.0
OAPI_CODEGEN_VERSION  := v2.8.2

# Database URLs. ADMIN is the migration owner (migrations, cmd/bootstrap); the API and
# integration tests use the non-owner app_rt role so RLS applies (ADR-0011 §1).
ADMIN_DATABASE_URL ?= postgresql://app:app@localhost:5432/app_dev
DATABASE_URL       ?= postgresql://app_rt:app_rt@localhost:5432/app_dev
export ADMIN_DATABASE_URL DATABASE_URL

# The gate before any work is called done. The last line records that it passed for this exact
# working tree, so the Stop hook can tell "checked" from "changed since".
check: lint test
	@scripts/stamp.sh

# The independent evaluation: every gate in one table (see docs/engineering/agentic-sdlc.md).
# Slower than `make check`: it forces uncached tests, regenerates code and builds a scratch database.
evaluate:
	@scripts/evaluate.sh

# Tests the Claude hooks themselves. Run after any change to .claude/hooks or .claude/settings.json.
test-hooks:
	@scripts/test-hooks.sh

# Which requirement IDs of a module are tested? Usage: make spec-coverage MOD=FEE
spec-coverage:
	@scripts/spec-coverage.sh $(MOD)

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)

# Tests need a migrated database (make db-up, make db-roles, make migrate).
test:
	cd $(API) && go test ./...

lint:
	@out=$$(cd $(API) && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	cd $(API) && go vet ./...
	cd $(API) && golangci-lint run ./...

run:
	cd $(API) && go run ./cmd/api

db-up:
	docker compose -f deploy/docker-compose.yml up -d postgres

db-down:
	docker compose -f deploy/docker-compose.yml down

# Creates the app_rt runtime role. Idempotent; needed once per database volume.
db-roles:
	docker compose -f deploy/docker-compose.yml exec -T postgres psql -U app -d app_dev -v ON_ERROR_STOP=1 < deploy/db/roles.sql

migrate:
	goose -dir $(API)/migrations postgres "$(ADMIN_DATABASE_URL)" up

gen:
	mkdir -p $(API)/internal/api/gen
	cd $(API) && oapi-codegen -generate types,std-http,strict-server -package gen -o internal/api/gen/api.gen.go ../../api/openapi/openapi.yaml
	cd $(API) && sqlc generate

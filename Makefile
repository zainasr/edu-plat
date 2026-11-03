.PHONY: check test lint run db-up db-down gen tools
API := services/api

# Pinned developer tools. Bump a version here and in the same PR, then run `make tools`.
GOLANGCI_LINT_VERSION := v2.15.0
SQLC_VERSION          := v1.31.1
GOOSE_VERSION         := v3.29.0
OAPI_CODEGEN_VERSION  := v2.8.2

check: lint test

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)

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

gen:
	mkdir -p $(API)/internal/api/gen
	cd $(API) && oapi-codegen -generate types,std-http,strict-server -package gen -o internal/api/gen/api.gen.go ../../api/openapi/openapi.yaml
	# sqlc is added with the first module that has tables (see docs/engineering/getting-started.md)

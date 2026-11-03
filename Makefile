.PHONY: check test lint run db-up db-down gen
API := services/api

check: lint test

test:
	cd $(API) && go test ./...

lint:
	@out=$$(cd $(API) && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	cd $(API) && go vet ./...

run:
	cd $(API) && go run ./cmd/api

db-up:
	docker compose -f deploy/docker-compose.yml up -d postgres

db-down:
	docker compose -f deploy/docker-compose.yml down

gen:
	@echo "gen: OpenAPI + sqlc generation is added with the first module (see ADR-0003)"

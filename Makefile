.PHONY: server worker test test-coverage lint fmt all docker-up docker-down docker-clean

# --- Build & Run ---

server:
	go run ./cmd/server/

worker:
	go run ./cmd/worker/

# --- Quality ---

test:
	go test ./...

test-coverage:
	@echo "Running tests with coverage (only domain business logic)..."
	@go test -coverprofile=coverage.out ./... > /dev/null 2>&1
	@grep -v -E '(mocks/|\.pb\.go|query\.sql\.go|repository/db/models\.go|repository/db/db\.go|view\.go|main\.go|internal/client/cli/|internal/client/application/|internal/client/container/|internal/client/gui/|internal/client/version/|internal/client/infrastructure/|internal/server/api/|internal/server/application/|internal/server/config/|internal/server/infrastructure/|pkg/logger/|pkg/posgres/|app\.go|config\.go|logger\.go)' coverage.out > coverage.filtered.out || true
	@go tool cover -func=coverage.filtered.out | grep total | awk '{print "Coverage: " $$3}'

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

all: fmt lint test

# --- Docker ---

docker-up:
	docker-compose up --build

docker-down:
	docker-compose down

docker-clean:
	docker-compose down -v && docker volume prune -f

# --- Help ---

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

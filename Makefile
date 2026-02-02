.PHONY: help build run test lint clean docker-up docker-down migrate

# Variables
APP_NAME := testforge
VERSION := 0.1.0
GO := go
GOFLAGS := -ldflags "-X main.version=$(VERSION)"

# Default target
help:
	@echo "TestForge - AI-Powered Testing Platform"
	@echo ""
	@echo "Usage:"
	@echo "  make build          Build all binaries"
	@echo "  make run            Run the API server"
	@echo "  make test           Run tests"
	@echo "  make lint           Run linters"
	@echo "  make clean          Clean build artifacts"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-up      Start all services"
	@echo "  make docker-down    Stop all services"
	@echo "  make docker-logs    View service logs"
	@echo "  make docker-ps      Show service status"
	@echo ""
	@echo "Database:"
	@echo "  make migrate        Run database migrations"
	@echo "  make migrate-down   Rollback last migration"
	@echo ""
	@echo "Development:"
	@echo "  make dev            Run API with hot reload (requires air)"
	@echo "  make deps           Download dependencies"
	@echo "  make tidy           Tidy go.mod"

# Build targets
build: build-api

build-api:
	@echo "Building API server..."
	$(GO) build $(GOFLAGS) -o bin/api ./cmd/api

build-all: build-api
	@echo "Building all binaries..."
	$(GO) build $(GOFLAGS) -o bin/worker-discovery ./cmd/worker-discovery || true
	$(GO) build $(GOFLAGS) -o bin/worker-ai ./cmd/worker-ai || true
	$(GO) build $(GOFLAGS) -o bin/worker-execution ./cmd/worker-execution || true
	$(GO) build $(GOFLAGS) -o bin/cli ./cmd/cli || true

# Run targets
run: build-api
	@echo "Starting API server..."
	./bin/api

dev:
	@which air > /dev/null || (echo "Installing air..." && go install github.com/air-verse/air@latest)
	air -c .air.toml

# Test targets
test:
	@echo "Running tests..."
	$(GO) test -v -race -cover ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration:
	@echo "Running integration tests..."
	$(GO) test -v -tags=integration ./tests/integration/...

# Lint targets
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...
	goimports -w .

# Clean targets
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Docker targets
docker-up:
	@echo "Starting services..."
	docker-compose up -d
	@echo ""
	@echo "Waiting for services to be healthy..."
	@sleep 5
	docker-compose ps

docker-down:
	@echo "Stopping services..."
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-ps:
	docker-compose ps

docker-clean:
	@echo "Removing all containers and volumes..."
	docker-compose down -v --remove-orphans

# Database targets
migrate:
	@echo "Running migrations..."
	@for f in migrations/*.sql; do \
		echo "Applying $$f..."; \
		PGPASSWORD=testforge psql -h localhost -U testforge -d testforge -f $$f || exit 1; \
	done
	@echo "Migrations complete!"

migrate-docker:
	@echo "Running migrations via docker..."
	@for f in migrations/*.sql; do \
		echo "Applying $$f..."; \
		docker exec -i testforge-postgres psql -U testforge -d testforge < $$f || exit 1; \
	done
	@echo "Migrations complete!"

# Dependency management
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download

tidy:
	@echo "Tidying go.mod..."
	$(GO) mod tidy

vendor:
	@echo "Creating vendor directory..."
	$(GO) mod vendor

# Development helpers
setup: deps docker-up
	@echo "Waiting for PostgreSQL..."
	@sleep 10
	@make migrate-docker
	@echo ""
	@echo "Setup complete! Run 'make run' to start the API server."

generate:
	@echo "Running go generate..."
	$(GO) generate ./...

# Quick check before commit
check: fmt lint test
	@echo "All checks passed!"

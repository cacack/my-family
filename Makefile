.PHONY: help build test fmt vet generate generate-api generate-types verify-generated clean run dev setup check-coverage lint security check check-full docker-smoke-test

# Default target
.DEFAULT_GOAL := help

# Tool versions - update these when upgrading
GOLANGCI_LINT_VERSION := v2.7.2
GO_TEST_COVERAGE_VERSION := latest

help: ## Display this help message
	@echo "my-family Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# Build all Go packages
build: ## Build all Go packages
	go build ./...

binary: frontend ## Build binary with embedded frontend
	mkdir -p internal/web/dist
	rm -rf internal/web/dist/*
	cp -r web/build/* internal/web/dist/
	go build -o myfamily ./cmd/myfamily

frontend: ## Build frontend only
	cd web && npm run build

test: ## Run all tests (Go + frontend)
	go test ./...
	cd web && npm test -- --run

test-go: ## Run only Go tests
	go test ./...

test-coverage: ## Run tests with HTML coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

fmt: ## Format Go code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

generate-api: ## Generate Go code from OpenAPI spec
	go generate ./internal/api/...

generate-types: ## Generate TypeScript types from OpenAPI spec
	cd web && npm run generate:types

generate: generate-api generate-types ## Generate all code from OpenAPI spec

verify-generated: ## Verify generated code matches OpenAPI spec
	@echo "Checking Go generated code..."
	@go generate ./internal/api/...
	@if ! git diff --quiet internal/api/generated.go 2>/dev/null; then \
		echo "ERROR: Go API code is out of sync with OpenAPI spec"; \
		echo "Run 'make generate-api' and commit the changes"; \
		git diff --stat internal/api/generated.go; \
		exit 1; \
	fi
	@echo "  Go generated code is up-to-date"
	@echo "Checking TypeScript generated types..."
	@cd web && npm run generate:types
	@if ! git diff --quiet web/src/lib/api/types.generated.ts 2>/dev/null; then \
		echo "ERROR: TypeScript types are out of sync with OpenAPI spec"; \
		echo "Run 'make generate-types' and commit the changes"; \
		git diff --stat web/src/lib/api/types.generated.ts; \
		exit 1; \
	fi
	@echo "  TypeScript generated types are up-to-date"
	@echo "All generated code is in sync"

clean: ## Clean build artifacts
	rm -f myfamily coverage.out coverage.html
	rm -rf web/dist web/build web/.svelte-kit internal/web/dist

run: ## Run the server (development)
	go run ./cmd/myfamily serve

dev-frontend: ## Run frontend dev server
	cd web && npm run dev

deps: ## Install dependencies
	go mod download
	cd web && npm install

lint: ## Run golangci-lint
	@GOLANGCI_LINT=$$(command -v golangci-lint || echo "$$HOME/go/bin/golangci-lint"); \
	if [ ! -x "$$GOLANGCI_LINT" ]; then \
		GOLANGCI_LINT="$$(go env GOPATH)/bin/golangci-lint"; \
	fi; \
	$$GOLANGCI_LINT run ./...

security: ## Run security checks (gosec)
	@GOLANGCI_LINT=$$(command -v golangci-lint || echo "$$HOME/go/bin/golangci-lint"); \
	if [ ! -x "$$GOLANGCI_LINT" ]; then \
		GOLANGCI_LINT="$$(go env GOPATH)/bin/golangci-lint"; \
	fi; \
	$$GOLANGCI_LINT run --enable-only gosec ./...

check: fmt vet test ## Run all checks (CI validation)
	cd web && npm run check

check-full: fmt vet lint test ## Full CI check including lint
	cd web && npm run check

check-coverage: ## Check coverage thresholds (same as CI)
	go test -coverprofile=coverage.out ./...
	@GO_TEST_COVERAGE=$$(command -v go-test-coverage || echo "$$HOME/go/bin/go-test-coverage"); \
	if [ ! -x "$$GO_TEST_COVERAGE" ]; then \
		GO_TEST_COVERAGE="$$(go env GOPATH)/bin/go-test-coverage"; \
	fi; \
	$$GO_TEST_COVERAGE --config=.testcoverage.yml --profile=coverage.out

setup: deps ## Set up development environment
	@echo "Installing development tools..."
	go install github.com/vladopajic/go-test-coverage/v2@$(GO_TEST_COVERAGE_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
	ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
	ln -sf ../../scripts/pre-push .git/hooks/pre-push
	@echo ""
	@echo "✓ Development environment ready"
	@echo ""
	@echo "  Git hooks installed:"
	@echo "    pre-commit: gofmt, go vet, golangci-lint, tests"
	@echo "    pre-push:   coverage threshold checks (85%)"
	@echo ""
	@echo "  Run 'make help' to see available commands"

docker-smoke-test: ## Build and test Docker image
	@echo "Building Docker image..."
	docker compose build
	@echo "Starting container..."
	docker compose up -d
	@echo "Waiting for container to be healthy..."
	@timeout=60; \
	while [ $$timeout -gt 0 ]; do \
		status=$$(docker compose ps --format json 2>/dev/null | jq -r '.[0].Health // "starting"'); \
		if [ "$$status" = "healthy" ]; then \
			echo "Container is healthy"; \
			break; \
		fi; \
		echo "  Status: $$status ($$timeout s remaining)"; \
		sleep 5; \
		timeout=$$((timeout - 5)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "ERROR: Container failed to become healthy"; \
		docker compose logs; \
		docker compose down; \
		exit 1; \
	fi
	@echo "Running smoke test..."
	@curl -sf http://localhost:8080/api/v1/persons > /dev/null || \
		(echo "ERROR: Health endpoint failed"; docker compose logs; docker compose down; exit 1)
	@echo "Smoke test passed"
	@docker compose down
	@echo "Docker smoke test complete"

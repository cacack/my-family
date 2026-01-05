.PHONY: help build test fmt vet generate clean run dev setup check-coverage lint security check check-full

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

generate: ## Generate code (OpenAPI, etc.)
	go generate ./...

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

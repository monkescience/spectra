VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

COVERAGE_DIR := tests/coverage
COVERAGE_OUT := $(COVERAGE_DIR)/coverage.out
COVERAGE_HTML := $(COVERAGE_DIR)/coverage.html

.PHONY: help generate fmt lint test coverage build clean mod-tidy

help: ## Show help
	@awk 'BEGIN {FS = ":.*##"; printf "Targets:\n"} /^[a-zA-Z_-]+:.*##/ {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

generate: ## Run code generation
	go generate ./...

fmt: ## Format code (gofumpt + goimports via golangci-lint)
	golangci-lint fmt

lint: ## Lint code
	golangci-lint run

test: ## Run all tests with race detection
	@mkdir -p $(COVERAGE_DIR)
	go test -race -v -coverprofile=$(COVERAGE_OUT) -covermode=atomic ./...

coverage: test ## Generate HTML coverage report
	go tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)

build: ## Build packages
	go build ./...

clean: ## Clean build artifacts
	rm -rf $(COVERAGE_DIR)

mod-tidy: ## Tidy Go modules
	go mod tidy

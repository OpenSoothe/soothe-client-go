.PHONY: help build test test-short test-integration vet fmt lint clean install run deps

# Project variables
PACKAGE := github.com/mirasoth/soothe-client-go
BINARY := soothe-client
GO := go
GOFLAGS :=

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Building
build: ## Build the package
	@echo "Building..."
	$(GO) build $(GOFLAGS) ./...

install: ## Install the package
	@echo "Installing..."
	$(GO) install $(GOFLAGS) ./...

# Testing
test: ## Run all tests (including integration tests)
	@echo "Running all tests..."
	$(GO) test -v ./...

test-short: ## Run short/unit tests only (skip integration tests)
	@echo "Running unit tests..."
	$(GO) test -short -v ./...

test-integration: ## Run integration tests only (requires running daemon)
	@echo "Running integration tests..."
	$(GO) test -v -run Integration ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Code quality
vet: ## Run go vet
	@echo "Running go vet..."
	$(GO) vet ./...

fmt: ## Format code with go fmt
	@echo "Formatting code..."
	$(GO) fmt ./...

lint: ## Run golangci-lint (if installed)
	@echo "Running lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install from: https://golangci-lint.run/usage/install/"; \
	fi

# Dependencies
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

deps-update: ## Update dependencies
	@echo "Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy

# Cleanup
clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GO) clean ./...
	rm -f coverage.out coverage.html
	rm -f $(BINARY)

# Utilities
check: vet fmt lint ## Run all code quality checks
	@echo "All checks passed!"

list: ## List all Go files
	@echo "Go source files:"
	@find . -name "*.go" -not -path "./.git/*" | sort

packages: ## List all import paths
	@echo "Package import paths:"
	$(GO) list ./...

# Development
dev: fmt vet test-short ## Format, vet, and run unit tests
	@echo "Development checks complete!"

all: deps build test vet ## Full build and test pipeline
	@echo "All tasks completed successfully!"

# Info
info: ## Show project information
	@echo "Package: $(PACKAGE)"
	@echo "Binary: $(BINARY)"
	@echo "Go version:"
	@$(GO) version
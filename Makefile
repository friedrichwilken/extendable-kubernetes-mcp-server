# Extendable Kubernetes MCP Server Makefile

# Variables
BINARY_NAME=extendable-k8s-mcp
MAIN_PATH=./cmd
BUILD_DIR=./build
TEST_DIR=./test

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Build flags
LDFLAGS=-ldflags "-s -w"

# Test flags
TEST_FLAGS=-v -race -coverprofile=coverage.out
SHORT_TEST_FLAGS=-v -short -race

.PHONY: all build clean test test-unit test-integration test-e2e test-coverage benchmark setup-envtest test-integration-envtest deps tidy fmt fmt-modern lint lint-fix run help code-quality pre-commit-check

# Default target
all: clean deps build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "Clean completed"

# Run all tests
test: lint
	@echo "Running all tests..."
	$(GOTEST) $(TEST_FLAGS) ./...

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) $(TEST_FLAGS) $(TEST_DIR)/unit/...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) $(TEST_FLAGS) $(TEST_DIR)/integration/...

# Run end-to-end tests
test-e2e:
	@echo "Running e2e tests..."
	$(GOTEST) $(TEST_FLAGS) $(TEST_DIR)/e2e/...

# Run performance benchmarks
benchmark:
	@echo "Running performance benchmarks..."
	$(GOTEST) -bench=. -benchmem $(TEST_DIR)/e2e/

# Setup envtest binaries for Kubernetes integration tests
setup-envtest:
	@echo "Setting up envtest binaries..."
	@chmod +x scripts/setup-envtest.sh
	@./scripts/setup-envtest.sh

# Run integration tests with envtest (requires setup-envtest)
test-integration-envtest:
	@echo "Running integration tests with envtest..."
	@if [ -f test/envtest/env.sh ]; then \
		source test/envtest/env.sh && $(GOTEST) $(TEST_FLAGS) $(TEST_DIR)/integration/...; \
	else \
		echo "Envtest not setup. Run 'make setup-envtest' first."; \
		exit 1; \
	fi

# Run short tests (for development)
test-short:
	@echo "Running short tests..."
	$(GOTEST) $(SHORT_TEST_FLAGS) ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) $(TEST_FLAGS) ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOGET) -d -v ./...

# Tidy modules
tidy:
	@echo "Tidying modules..."
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run

# Run linter with automatic fixes
lint-fix:
	@echo "Running linter with automatic fixes..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run --fix

# Modern Go formatting: Convert interface{} to any and apply strict formatting
fmt-modern:
	@echo "Applying modern Go formatting (interface{} -> any)..."
	@which gofumpt > /dev/null || (echo "Installing gofumpt..." && go install mvdan.cc/gofumpt@latest)
	gofumpt -w -extra .

# Run the server locally (stdio mode)
run: build
	@echo "Starting server in stdio mode..."
	$(BUILD_DIR)/$(BINARY_NAME)

# Run the server with HTTP transport
run-http: build
	@echo "Starting server in HTTP mode on port 8080..."
	$(BUILD_DIR)/$(BINARY_NAME) --port 8080

# Install dependencies and build
install: deps tidy build

# Create a release build
release: clean
	@echo "Building release binary..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Release builds completed in $(BUILD_DIR)/"

# Show help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build targets:"
	@echo "  all            - Clean, download deps, and build"
	@echo "  build          - Build the binary"
	@echo "  clean          - Remove build artifacts"
	@echo "  release        - Build release binaries for multiple platforms"
	@echo ""
	@echo "Test targets:"
	@echo "  test           - Run all tests with coverage"
	@echo "  test-unit      - Run unit tests only"
	@echo "  test-integration - Run integration tests only"
	@echo "  test-e2e       - Run end-to-end tests only"
	@echo "  test-short     - Run short tests (for development)"
	@echo "  test-coverage  - Run tests and generate HTML coverage report"
	@echo "  benchmark      - Run performance benchmarks"
	@echo "  setup-envtest  - Download envtest binaries for Kubernetes tests"
	@echo "  test-integration-envtest - Run integration tests with envtest"
	@echo ""
	@echo "Development targets:"
	@echo "  deps           - Download dependencies"
	@echo "  tidy           - Tidy modules"
	@echo "  fmt            - Format code"
	@echo "  fmt-modern     - Modern Go formatting (interface{} -> any)"
	@echo "  lint           - Run golangci-lint"
	@echo "  lint-fix       - Run golangci-lint with automatic fixes"
	@echo "  code-quality   - Run all formatting and linting (tidy, fmt, fmt-modern, lint-fix)"
	@echo "  pre-commit-check - Run code quality and all tests"
	@echo "  run            - Build and run server in stdio mode"
	@echo "  run-http       - Build and run server in HTTP mode"
	@echo "  dev-setup      - Setup development environment"
	@echo ""
	@echo "Other targets:"
	@echo "  install        - Install dependencies and build"
	@echo "  help           - Show this help message"

# Code quality - runs all formatting and linting
code-quality: tidy fmt fmt-modern lint-fix
	@echo "Code quality checks completed"

# Pre-commit check - runs code quality and all tests
pre-commit-check: code-quality test
	@echo "Pre-commit checks completed successfully"

# Development targets
dev-setup: deps tidy fmt
	@echo "Development environment setup complete"

dev-test: test-short
	@echo "Development tests completed"
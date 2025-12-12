.PHONY: all build test lint fmt vet clean cover help

# Default target
all: lint test build

# Build the module
build:
	@echo "Building..."
	@go build -v ./...

# Build examples
build-examples:
	@echo "Building examples..."
	@for dir in examples/*/; do \
		echo "Building $$dir..."; \
		(cd "$$dir" && go build -v .); \
	done

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race ./...

# Run tests with coverage
cover:
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out
	@echo "\nTo view coverage in browser: go tool cover -html=coverage.out"

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run

# Format code
fmt:
	@echo "Formatting code..."
	@gofmt -s -w .
	@goimports -w -local github.com/yourorg/volt .

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	@go mod tidy
	@go mod verify

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f coverage.out
	@go clean -cache -testcache

# Install development tools
tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest

# Run all checks (CI simulation)
ci: tidy fmt vet lint test
	@echo "All CI checks passed!"

# Show help
help:
	@echo "Available targets:"
	@echo "  all            - Run lint, test, and build (default)"
	@echo "  build          - Build the module"
	@echo "  build-examples - Build all examples"
	@echo "  test           - Run tests with race detector"
	@echo "  cover          - Run tests with coverage report"
	@echo "  lint           - Run golangci-lint"
	@echo "  fmt            - Format code with gofmt and goimports"
	@echo "  vet            - Run go vet"
	@echo "  tidy           - Tidy and verify dependencies"
	@echo "  clean          - Clean build artifacts"
	@echo "  tools          - Install development tools"
	@echo "  ci             - Run all CI checks locally"
	@echo "  help           - Show this help"

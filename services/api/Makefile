# NPDMS API Makefile

.PHONY: help test test-coverage test-unit test-integration build run clean lint

# Default target
help:
	@echo "NPDMS API - Available Commands:"
	@echo ""
	@echo "  make test              - Run all tests"
	@echo "  make test-unit         - Run unit tests only"
	@echo "  make test-integration  - Run integration tests"
	@echo "  make test-coverage     - Run tests with coverage report"
	@echo "  make build             - Build the API binary"
	@echo "  make run               - Run the API server"
	@echo "  make clean             - Clean build artifacts"
	@echo "  make lint              - Run linter"
	@echo ""

# Run all tests
test:
	@echo "Running all tests..."
	go test -v ./...

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	go test -v -short ./...

# Run integration tests (requires database)
test-integration:
	@echo "Running integration tests..."
	go test -v -run Integration ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	go tool cover -func=coverage.out | grep total

# Build the API
build:
	@echo "Building API..."
	go build -o bin/npdms-api main.go

# Run the API server
run:
	@echo "Starting API server..."
	go run main.go

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Run tests in watch mode (requires gotestsum)
test-watch:
	@echo "Running tests in watch mode..."
	gotestsum --watch

# Database setup for testing
test-db-setup:
	@echo "Setting up test database..."
	docker-compose up -d postgres
	sleep 3
	@echo "Test database ready"

# Database teardown
test-db-teardown:
	@echo "Tearing down test database..."
	docker-compose down

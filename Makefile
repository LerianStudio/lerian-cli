.PHONY: build clean install test test-unit test-integration test-coverage test-race test-verbose test-bench lint fmt deps help

# Variables
BINARY_NAME=lerian
BUILD_DIR=./build/bin
MAIN_PATH=./cmd/lerian
GO_FILES=$(shell find . -name '*.go' -type f)

# Version information
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILT_BY?=$(shell whoami)@$(shell hostname)

# Linker flags
LDFLAGS=-ldflags "\
	-s -w \
	-X github.com/lerian-studio/lerian-cli/internal/version.Version=${VERSION} \
	-X github.com/lerian-studio/lerian-cli/internal/version.Commit=${COMMIT} \
	-X github.com/lerian-studio/lerian-cli/internal/version.Date=${DATE} \
	-X github.com/lerian-studio/lerian-cli/internal/version.BuiltBy=${BUILT_BY}"

# Default target
all: build

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go get github.com/spf13/cobra@latest
	go get gopkg.in/yaml.v3

# Build the binary
build: deps
	@echo "Building ${BINARY_NAME} ${VERSION}..."
	@mkdir -p ${BUILD_DIR}
	go build ${LDFLAGS} -o ${BUILD_DIR}/${BINARY_NAME} ${MAIN_PATH}
	@echo "Build complete: ${BUILD_DIR}/${BINARY_NAME}"

# Install the binary to GOPATH/bin
install:
	@echo "Installing ${BINARY_NAME} ${VERSION}..."
	go install ${LDFLAGS} ${MAIN_PATH}
	@echo "Installed to $(shell go env GOPATH)/bin/${BINARY_NAME}"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf ${BUILD_DIR}
	@rm -f ${BINARY_NAME}
	@echo "Clean complete"

# Run all tests
test:
	@echo "Running all tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	go test -v -short ./...

# Run integration tests only
test-integration:
	@echo "Running integration tests..."
	go test -v -run Integration ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -v -race ./...

# Run tests in verbose mode
test-verbose:
	@echo "Running tests in verbose mode..."
	go test -v -count=1 ./...

# Run benchmarks
test-bench:
	@echo "Running benchmarks..."
	go test -v -bench=. -benchmem ./...

# Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run --timeout=5m

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# Run the application
run: build
	@${BUILD_DIR}/${BINARY_NAME}

# Show help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build              - Build the binary"
	@echo "  make install            - Install the binary to GOPATH/bin"
	@echo "  make clean              - Remove build artifacts"
	@echo "  make deps               - Install dependencies"
	@echo "  make run                - Build and run the application"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test               - Run all tests with coverage and race detector"
	@echo "  make test-unit          - Run unit tests only (fast)"
	@echo "  make test-integration   - Run integration tests only"
	@echo "  make test-coverage      - Run tests and generate HTML coverage report"
	@echo "  make test-race          - Run tests with race detector"
	@echo "  make test-verbose       - Run tests in verbose mode"
	@echo "  make test-bench         - Run benchmarks"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint               - Run golangci-lint"
	@echo "  make fmt                - Format code with gofmt"
	@echo ""
	@echo "  make help               - Show this help message"


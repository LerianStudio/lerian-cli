.PHONY: build clean install dev dev-uninstall test test-unit test-integration test-coverage test-race test-verbose test-bench lint fmt deps help

# Variables
BINARY_NAME=lerian
DEV_BINARY_NAME=lerian-dev
BUILD_DIR=./build/bin

# Where the official installer puts things, so a dev build lands beside the
# release instead of in a second directory that may or may not be on PATH.
INSTALL_DIR?=$(HOME)/.local/bin
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
	@echo ""
	@echo "NOTE: this is a second binary also called '${BINARY_NAME}'. If the release"
	@echo "      is installed in ${INSTALL_DIR}, which of the two runs now depends on"
	@echo "      the order of your PATH. Use 'make dev' to build a binary that cannot"
	@echo "      be confused with the release."

# dev builds the working tree as ${DEV_BINARY_NAME}, beside the installed release
# rather than over it.
#
# Testing a change used to mean either cutting a release or overwriting the
# binary you depend on. This does neither: 'lerian' stays whatever was installed
# from the releases page, and 'lerian-dev' is whatever is checked out right now.
# Both are on PATH, and which is which is never a question.
#
# The version carries the branch and, when git says the tree is dirty, a -dirty
# suffix — so a binary built from uncommitted work says so when asked.
# The branch is read inside the recipe, by the shell, for two reasons.
#
# symbolic-ref rather than rev-parse --abbrev-ref: on a detached HEAD the latter
# succeeds and prints "HEAD", so the || fallback never runs and the build claims
# to come from a branch called HEAD. symbolic-ref fails there, which is what
# makes the fallback mean something.
#
# And a shell variable rather than a make one: make expands its variables into
# the recipe text before the shell parses it, so a branch name carrying shell
# syntax would be executed rather than printed. Read by the shell it is data.
dev:
	@mkdir -p "${INSTALL_DIR}"
	@branch="$$(git symbolic-ref -q --short HEAD 2>/dev/null || echo detached)"; \
	echo "Building ${DEV_BINARY_NAME} from $$branch (${VERSION})..."; \
	go build -trimpath -ldflags "\
		-X github.com/lerian-studio/lerian-cli/internal/version.Version=${VERSION}+$$branch \
		-X github.com/lerian-studio/lerian-cli/internal/version.Commit=${COMMIT} \
		-X github.com/lerian-studio/lerian-cli/internal/version.Date=${DATE} \
		-X github.com/lerian-studio/lerian-cli/internal/version.BuiltBy=${BUILT_BY}" \
		-o "${INSTALL_DIR}/${DEV_BINARY_NAME}" ${MAIN_PATH}
	@echo ""
	@"${INSTALL_DIR}/${DEV_BINARY_NAME}" version
	@echo ""
	@echo "Installed: ${INSTALL_DIR}/${DEV_BINARY_NAME}"
	@echo "The release stays untouched. Compare them:"
	@echo "    ${BINARY_NAME} version"
	@echo "    ${DEV_BINARY_NAME} version"

dev-uninstall:
	@rm -f "${INSTALL_DIR}/${DEV_BINARY_NAME}"
	@echo "Removed ${INSTALL_DIR}/${DEV_BINARY_NAME}"

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
	@echo "  make dev                - Build the working tree as lerian-dev, beside the release"
	@echo "  make dev-uninstall      - Remove lerian-dev"
	@echo "  make install            - Install as a second 'lerian' in GOPATH/bin (see note)"
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


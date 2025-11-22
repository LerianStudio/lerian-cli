.PHONY: build clean install test deps help

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

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run the application
run: build
	@${BUILD_DIR}/${BINARY_NAME}

# Show help
help:
	@echo "Available targets:"
	@echo "  make build    - Build the binary"
	@echo "  make install  - Install the binary to GOPATH/bin"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make test     - Run tests"
	@echo "  make deps     - Install dependencies"
	@echo "  make run      - Build and run the application"
	@echo "  make help     - Show this help message"

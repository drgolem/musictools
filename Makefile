# musictools Makefile
# Lock-free audio ringbuffer library with real-time playback

.PHONY: help build install test test-race vet fmt clean all

# Binary name
BINARY_NAME=musictools
# Module name from go.mod
MODULE=github.com/drgolem/musictools
# Build output directory
BUILD_DIR=bin

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=gofmt
GOMOD=$(GOCMD) mod
GOINSTALL=$(GOCMD) install

# Build flags
LDFLAGS=-ldflags "-s -w"
BUILD_FLAGS=-v

# Default target
all: vet test build

help:
	@echo "musictools - Lock-free SPSC ringbuffer audio player"
	@echo ""
	@echo "Available targets:"
	@echo "  make build       - Build the musictools binary"
	@echo "  make install     - Install musictools to \$$GOPATH/bin"
	@echo "  make test        - Run all tests"
	@echo "  make test-race   - Run tests with race detector"
	@echo "  make vet         - Run go vet"
	@echo "  make fmt         - Format all Go code"
	@echo "  make clean       - Remove built binaries and test artifacts"
	@echo "  make deps        - Download and verify dependencies"
	@echo "  make all         - Run vet, test, and build"
	@echo ""
	@echo "Build artifacts:"
	@echo "  ./$(BINARY_NAME) - Main binary"

# Build the main binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BINARY_NAME) .
	@echo "Build complete: ./$(BINARY_NAME)"

# Install to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME) to \$$GOPATH/bin..."
	$(GOINSTALL) $(LDFLAGS) .
	@echo "Installed: $(BINARY_NAME)"

# Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with race detector (CRITICAL for thread safety)
test-race:
	@echo "Running tests with race detector..."
	$(GOTEST) -race -v ./...

# Run go vet (static analysis)
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Format all Go code
fmt:
	@echo "Formatting Go code..."
	$(GOFMT) -w .
	@echo "Formatting complete"

# Download and verify dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	@echo "Verifying dependencies..."
	$(GOMOD) verify
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME)
	@rm -rf $(BUILD_DIR)
	@rm -f *.test
	@rm -f *.out
	@rm -f *.prof
	@echo "Clean complete"

# Development workflow: format, vet, test with race detector, build
dev: fmt vet test-race build
	@echo "Development build complete!"

# Quick build without tests (use sparingly)
quick:
	@echo "Quick build (no tests)..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

# Production build health check
health:
	@echo "Running health check..."
	@echo "1. Building..."
	@$(GOBUILD) ./... > /dev/null 2>&1 && echo "   ✓ Build successful" || echo "   ✗ Build failed"
	@echo "2. Running go vet..."
	@$(GOVET) ./... > /dev/null 2>&1 && echo "   ✓ Vet passed" || echo "   ✗ Vet failed"
	@echo "3. Running tests with race detector..."
	@$(GOTEST) -race ./... > /dev/null 2>&1 && echo "   ✓ Tests passed (no races)" || echo "   ✗ Tests failed or races detected"
	@echo "Health check complete"

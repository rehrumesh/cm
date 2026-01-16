# Project settings
BINARY_NAME := cm
MODULE := cm
DIST_DIR := dist
INSTALL_PATH := $(HOME)/.local/bin

# Build settings
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Go settings
GO := go
GOFLAGS := -trimpath
LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildTime=$(BUILD_TIME)

# Platforms for cross-compilation
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: all build build-all install uninstall clean test lint fmt vet tidy dev help

# Default target
all: build

# Build for current platform
build: $(DIST_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY_NAME) .

# Create dist directory
$(DIST_DIR):
	mkdir -p $(DIST_DIR)

# Build for all platforms
build-all: $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=$(DIST_DIR)/$(BINARY_NAME)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then output=$$output.exe; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $$output . || exit 1; \
	done
	@echo "Build complete. Binaries in $(DIST_DIR)/"

# Install to user bin directory
install: build
	@mkdir -p $(INSTALL_PATH)
	@rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@cp $(DIST_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	@chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_PATH)"
	@echo "Make sure $(INSTALL_PATH) is in your PATH"

# Uninstall from user bin directory
uninstall:
	@rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Removed $(BINARY_NAME) from $(INSTALL_PATH)"

# Run tests
test:
	$(GO) test -v -race -cover ./...

# Run tests with coverage report
coverage: $(DIST_DIR)
	$(GO) test -v -race -coverprofile=$(DIST_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(DIST_DIR)/coverage.out -o $(DIST_DIR)/coverage.html
	@echo "Coverage report: $(DIST_DIR)/coverage.html"

# Run linter (requires golangci-lint)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	elif [ -x "$(HOME)/go/bin/golangci-lint" ]; then \
		$(HOME)/go/bin/golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Format code
fmt:
	$(GO) fmt ./...

# Run go vet
vet:
	$(GO) vet ./...

# Tidy dependencies
tidy:
	$(GO) mod tidy

# Clean build artifacts
clean:
	rm -rf $(DIST_DIR)
	rm -f $(BINARY_NAME)

# Development: build and run
run: build
	$(DIST_DIR)/$(BINARY_NAME)

# Development: hot reload (requires air)
dev:
	@if command -v air >/dev/null 2>&1; then \
		air; \
	elif [ -x "$(HOME)/go/bin/air" ]; then \
		$(HOME)/go/bin/air; \
	else \
		echo "air not installed. Install with:"; \
		echo "  go install github.com/air-verse/air@latest"; \
		echo "Then add ~/go/bin to your PATH"; \
	fi

# Show help
help:
	@echo "Available targets:"
	@echo "  build      - Build for current platform (output: $(DIST_DIR)/$(BINARY_NAME))"
	@echo "  build-all  - Build for all platforms"
	@echo "  install    - Install to $(INSTALL_PATH)"
	@echo "  uninstall  - Remove from $(INSTALL_PATH)"
	@echo "  run        - Build and run"
	@echo "  dev        - Run with hot reload (requires air)"
	@echo "  test       - Run tests"
	@echo "  coverage   - Run tests with coverage report"
	@echo "  lint       - Run golangci-lint"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  tidy       - Tidy go.mod"
	@echo "  clean      - Remove build artifacts"
	@echo "  help       - Show this help"

.PHONY: build build-dev run clean test docker

# Version info
VERSION ?= 0.0.1
ifeq ($(OS),Windows_NT)
	BUILD_TIME := $(shell powershell -Command "Get-Date -UFormat '%Y-%m-%dT%H:%M:%SZ'")
    GIT_COMMIT := $(shell powershell -Command "try { git rev-parse --short HEAD } catch { 'unknown' }")
else
    BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
    GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
endif

# Build flags
LDFLAGS := -ldflags "-w -s \
	-X gs-panel/internal/version.Version=$(VERSION) \
	-X gs-panel/internal/version.GitCommit=$(GIT_COMMIT) \
	-X gs-panel/internal/version.BuildTime=$(BUILD_TIME)"

# Default target
build: templ
	@echo "Building GS Panel v$(VERSION)..."
	go build $(LDFLAGS) -o bin/server cmd/server/main.go
	@echo "Build complete: bin/server"

# Development build (no version injection)
build-dev: templ
	@echo "Building (dev mode)..."
	go build -o bin/server cmd/server/main.go

# Run the server
run: build
	./bin/server

# Run in dev mode
run-dev: build-dev
	./bin/server

# Clean build artifacts
clean:
	rm -rf bin/
	@echo "Cleaned build artifacts"

# Run tests
test:
	go test -v ./...

# Build Docker image locally
docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t gs-panel:$(VERSION) .

# Check version
version: build
	./bin/server --version 2>/dev/null || echo "Version: $(VERSION) (commit: $(GIT_COMMIT))"

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run

# Generate templ files
templ:
	templ generate

dev: templ run-dev

# Full release build for all platforms
release:
	@echo "Building for multiple platforms..."
	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/server-linux-amd64 cmd/server/main.go
	# Linux ARM64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/server-linux-arm64 cmd/server/main.go
	# Windows AMD64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/server-windows-amd64.exe cmd/server/main.go
	@echo "Release builds complete"

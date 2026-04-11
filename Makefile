.PHONY: build build-orbit build-apsis test test-short test-verbose test-coverage lint clean install modernize

# Build metadata
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)

# Build both binaries
build: build-orbit build-apsis

# Build orbit only
build-orbit:
	go build -ldflags="$(LDFLAGS)" -o orbit ./cmd/orbit

# Build apsis only
build-apsis:
	go build -ldflags="$(LDFLAGS)" -o apsis ./cmd/apsis

# Run all tests
test:
	go test ./...

# Run tests quickly (skip slow retry tests)
test-short:
	go test -short ./...

# Run tests with verbose output
test-verbose:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -f orbit apsis coverage.out coverage.html

# Install both binaries to GOPATH/bin
install:
	go install ./cmd/orbit
	go install ./cmd/apsis

# Run modernize to update code to modern Go idioms
modernize:
	go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix ./...

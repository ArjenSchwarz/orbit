.PHONY: build build-orbit build-apsis test test-short test-verbose test-coverage lint clean install modernize

# Version from git describe
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")

# Build both binaries
build: build-orbit build-apsis

# Build orbit only
build-orbit:
	go build -o orbit ./cmd/orbit

# Build apsis only (with version injection)
build-apsis:
	go build -ldflags="-X main.version=$(VERSION)" -o apsis ./cmd/apsis

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

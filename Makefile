.PHONY: build build-orbit build-apsis test lint clean install modernize

# Build both binaries
build: build-orbit build-apsis

# Build orbit only
build-orbit:
	go build -o orbit ./cmd/orbit

# Build apsis only
build-apsis:
	go build -o apsis ./cmd/apsis

# Run all tests
test:
	go test ./...

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

.PHONY: build test lint clean install modernize

# Build the binary
build:
	go build -o orbit ./cmd/orbit

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
	rm -f orbit coverage.out coverage.html

# Install to GOPATH/bin
install:
	go install ./cmd/orbit

# Run modernize to update code to modern Go idioms
modernize:
	go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix ./...

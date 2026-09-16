.PHONY: build run test test-integration clean install fmt

# Binary name
BINARY=dresden-opendata-mcp
MAIN=cmd/server/main.go

# Build the binary
build:
	go build -o $(BINARY) $(MAIN)

# Run the server
run:
	go run $(MAIN)

# Install the binary
install:
	go build -o $$(go env GOPATH)/bin/$(BINARY) $(MAIN)

# Run unit tests
test:
	go test ./...

# Also run the tests that talk to the live portal
test-integration:
	go test -tags integration ./...

# Format code
fmt:
	go fmt ./...

# Clean build artifacts
clean:
	rm -f $(BINARY)
	rm -rf cache/

# Build for multiple platforms
build-all:
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 $(MAIN)
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 $(MAIN)
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 $(MAIN)
	GOOS=windows GOARCH=amd64 go build -o $(BINARY)-windows-amd64.exe $(MAIN)

# Tidy dependencies
tidy:
	go mod tidy

# Download dependencies
deps:
	go mod download

# Run with verbose logging
debug:
	LOG_LEVEL=debug go run $(MAIN)

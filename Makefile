.PHONY: build run test test-integration clean install fmt release

# Binary name
BINARY=od3
MAIN=cmd/od3/main.go

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

# Bump the version everywhere it is kept, commit and tag. Pushing is left to
# the caller so the release stays a deliberate step.
release:
	@test -n "$(VERSION)" || { echo "usage: make release VERSION=x.y.z"; exit 1; }
	@git diff --quiet HEAD || { echo "working tree is dirty"; exit 1; }
	sed -i.bak 's/^const version = ".*"/const version = "$(VERSION)"/' $(MAIN) && rm $(MAIN).bak
	sed -i.bak 's/tag: "[^"]*"/tag: "$(VERSION)"/' Formula/od3.rb && rm Formula/od3.rb.bak
	go test ./...
	git commit -am "bump version to $(VERSION)"
	git tag -a $(VERSION) -m "$(VERSION)"
	@echo "now run: git push --follow-tags"

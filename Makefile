# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=loki-mcp
BINARY_UNIX=$(BINARY_NAME)_unix
MAIN_PATH=./cmd/loki-mcp

.PHONY: all build clean test test-integration test-all run deps tidy lint fmt help

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

test:
	$(GOTEST) -v ./...

test-integration:
	$(GOTEST) -v -tags=integration ./internal/handlers -timeout 5m

test-all: test test-integration

run:
	$(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PATH)
	./$(BINARY_NAME)

deps:
	$(GOGET) -v -t ./...

tidy:
	$(GOMOD) tidy

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v $(MAIN_PATH)

help:
	@echo "Make commands:"
	@echo "  build            - Build the binary"
	@echo "  clean            - Remove binary and cache files"
	@echo "  test             - Run unit tests"
	@echo "  test-integration - Run integration tests (requires Docker)"
	@echo "  test-all         - Run all tests (unit + integration)"
	@echo "  run              - Build and run the binary"
	@echo "  deps             - Get dependencies"
	@echo "  tidy             - Tidy go.mod file"
	@echo "  lint             - Run golangci-lint"
	@echo "  fmt              - Format code with gofmt"
	@echo "  build-linux      - Cross-compile for Linux"
	@echo "  help             - Display this help message"

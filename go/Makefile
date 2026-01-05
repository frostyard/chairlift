.PHONY: all build run clean deps tidy

# Binary name
BINARY_NAME=chairlift

# Build directory
BUILD_DIR=build

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# CGO is not needed with puregotk!
CGO_ENABLED=0

all: deps build

deps:
	$(GOMOD) download

tidy:
	$(GOMOD) tidy

build:
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/chairlift

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

test:
	$(GOTEST) -v ./...

# Development build with race detector (requires CGO)
dev:
	CGO_ENABLED=1 $(GOBUILD) -race -o $(BUILD_DIR)/$(BINARY_NAME)-dev ./cmd/chairlift

# Install dependencies
install-deps:
	$(GOGET) github.com/jwijenbergh/puregotk
	$(GOGET) gopkg.in/yaml.v3

# Format code
fmt:
	gofmt -s -w .

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Cross-compile for different architectures
build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/chairlift

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/chairlift

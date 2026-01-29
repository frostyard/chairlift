.PHONY: all build run clean deps tidy install uninstall

# Binary name
BINARY_NAME=chairlift

# Build directory
BUILD_DIR=build

# Installation directories
PREFIX ?= /usr/local
BINDIR = $(PREFIX)/bin
DATADIR = $(PREFIX)/share
ICONSDIR = $(DATADIR)/icons
APPLICATIONSDIR = $(DATADIR)/applications
POLKITACTIONSDIR = $(DATADIR)/polkit-1/actions
POLKITRULESDIR = $(DATADIR)/polkit-1/rules.d

# Go parameters - use Homebrew's Go if available, otherwise fall back to system Go
HOMEBREW_GO=/home/linuxbrew/.linuxbrew/bin/go
GOCMD=$(shell if [ -x $(HOMEBREW_GO) ]; then echo $(HOMEBREW_GO); else echo go; fi)
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
	./$(BUILD_DIR)/$(BINARY_NAME) --dry-run

wet: build
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

# Install the application
install: build
	# Install binary
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	# Install wrapper script
	install -Dm755 data/chairlift-wrapper.sh $(DESTDIR)$(BINDIR)/chairlift-wrapper
	# Install desktop file
	install -Dm644 data/org.frostyard.ChairLift.desktop $(DESTDIR)$(APPLICATIONSDIR)/org.frostyard.ChairLift.desktop
	# Install icons
	install -Dm644 data/icons/hicolor/scalable/apps/org.frostyard.ChairLift.svg $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift.svg
	install -Dm644 data/icons/hicolor/scalable/apps/org.frostyard.ChairLift-flower.svg $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift-flower.svg
	install -Dm644 data/icons/hicolor/symbolic/apps/org.frostyard.ChairLift-symbolic.svg $(DESTDIR)$(ICONSDIR)/hicolor/symbolic/apps/org.frostyard.ChairLift-symbolic.svg
	# Install PolicyKit policy and rules for nbc
	install -Dm644 data/org.frostyard.ChairLift.nbc.policy $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.nbc.policy
	install -Dm644 data/org.frostyard.ChairLift.nbc.rules $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.nbc.rules

# Uninstall the application
uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	rm -f $(DESTDIR)$(BINDIR)/chairlift-wrapper
	rm -f $(DESTDIR)$(APPLICATIONSDIR)/org.frostyard.ChairLift.desktop
	rm -f $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift.svg
	rm -f $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift-flower.svg
	rm -f $(DESTDIR)$(ICONSDIR)/hicolor/symbolic/apps/org.frostyard.ChairLift-symbolic.svg
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.nbc.policy
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.nbc.rules

bump: ## generate a new version with svu
	@$(MAKE) build
	@$(MAKE) test
	@$(MAKE) fmt
	$(MAKE) lint
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working directory is not clean. Please commit or stash changes before bumping version."; \
		exit 1; \
	fi
	@echo "Creating new tag..."
	@version=$$(svu next); \
		git tag -a $$version -m "Version $$version"; \
		echo "Tagged version $$version"; \
		echo "Pushing tag $$version to origin..."; \
		git push origin $$version

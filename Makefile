.PHONY: all build run clean deps tidy install uninstall e2e

# Binary names
BINARY_NAME=chairlift
HELPER_NAME=chairlift-updex-helper

# Build directory
BUILD_DIR=build

# Installation directories
#
# PREFIX defaults to /usr, matching both the fixed absolute path pkexec
# matches against PolicyKit's org.freedesktop.policykit.exec.path annotation
# (data/org.frostyard.ChairLift.updex.policy, internal/updex.HelperPath) and
# the layout .goreleaser.yaml's nFPM packages already install to. PolicyKit
# itself only ever reads actions from /usr/share/polkit-1/actions — it does
# not consult PREFIX or XDG_DATA_DIRS — so installing under any other PREFIX
# means the .policy files land somewhere polkit never looks. Override PREFIX
# for a non-privileged dev install (e.g.
# `make install PREFIX=$$HOME/.local`), but understand that the
# PolicyKit-authenticated updex helper path then no longer resolves to the
# fixed exec.path annotation. DESTDIR (below) still layers under PREFIX
# unchanged, for staged/packaged installs.
PREFIX ?= /usr
BINDIR = $(PREFIX)/bin
DATADIR = $(PREFIX)/share
ICONSDIR = $(DATADIR)/icons
APPLICATIONSDIR = $(DATADIR)/applications
CONFIGDIR = $(DATADIR)/chairlift
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

build: build-app build-helper

build-app:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/chairlift

build-helper:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(HELPER_NAME) ./cmd/chairlift-updex-helper

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --dry-run

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

test:
	$(GOTEST) -v ./...

# End-to-end smoke tests require GTK4, Libadwaita, dbus-run-session, and Xvfb.
# They run separately from ci because the ordinary unit-test gate is
# intentionally usable on hosts without those runtime libraries.
e2e: build
	CHAIRLIFT_E2E_BUILD_DIR=$(abspath $(BUILD_DIR)) $(GOTEST) -v ./test/e2e

# Development build with race detector (requires CGO)
dev:
	CGO_ENABLED=1 $(GOBUILD) -race -o $(BUILD_DIR)/$(BINARY_NAME)-dev ./cmd/chairlift

# Install dependencies
install-deps:
	$(GOGET) codeberg.org/puregotk/puregotk
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
	# Install package-maintainer defaults; /etc/chairlift/config.yml remains the administrator-owned override
	install -Dm644 config.yml $(DESTDIR)$(CONFIGDIR)/config.yml
	# Install icons
	install -Dm644 data/icons/hicolor/scalable/apps/org.frostyard.ChairLift.svg $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift.svg
	install -Dm644 data/icons/hicolor/scalable/apps/org.frostyard.ChairLift-flower.svg $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift-flower.svg
	install -Dm644 data/icons/hicolor/symbolic/apps/org.frostyard.ChairLift-symbolic.svg $(DESTDIR)$(ICONSDIR)/hicolor/symbolic/apps/org.frostyard.ChairLift-symbolic.svg
	# Install updex helper binary
	install -Dm755 $(BUILD_DIR)/$(HELPER_NAME) $(DESTDIR)$(BINDIR)/$(HELPER_NAME)
	# Remove legacy passwordless rules from prior source installs
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.bootc.rules
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.updex.rules
	# Install PolicyKit policy for bootc
	install -Dm644 data/org.frostyard.ChairLift.bootc.policy $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.bootc.policy
	# Install PolicyKit policy for updex
	install -Dm644 data/org.frostyard.ChairLift.updex.policy $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.updex.policy
	# Install PolicyKit policy for native A/B sysupdate staging
	install -Dm644 data/org.frostyard.ChairLift.sysupdate.policy $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.sysupdate.policy

# Uninstall the application
uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY_NAME)
	rm -f $(DESTDIR)$(BINDIR)/chairlift-wrapper
	rm -f $(DESTDIR)$(APPLICATIONSDIR)/org.frostyard.ChairLift.desktop
	rm -f $(DESTDIR)$(CONFIGDIR)/config.yml
	rm -f $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift.svg
	rm -f $(DESTDIR)$(ICONSDIR)/hicolor/scalable/apps/org.frostyard.ChairLift-flower.svg
	rm -f $(DESTDIR)$(ICONSDIR)/hicolor/symbolic/apps/org.frostyard.ChairLift-symbolic.svg
	rm -f $(DESTDIR)$(BINDIR)/$(HELPER_NAME)
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.bootc.policy
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.bootc.rules
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.updex.policy
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.updex.rules
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.sysupdate.policy

# One command mirrors CI's host-independent gates (verify → lint → unit → race
# → build), in fail-fast order. The GTK/Xvfb-dependent E2E job runs separately
# through `make e2e`. The mill's deep gate calls this target.
#
# The build step reproduces CI's GOOS/GOARCH matrix (linux/amd64 and
# linux/arm64) into per-arch subdirectories, then rebuilds natively so
# build/chairlift is left runnable on this host. A host-arch-only build would
# let an arm64 compile failure pass locally and break CI.
.PHONY: ci
ci:
	@echo "==> verify: go.mod is tidy"
	$(GOMOD) tidy
	git diff --exit-code go.mod go.sum
	@echo "==> verify: go vet"
	CGO_ENABLED=$(CGO_ENABLED) $(GOCMD) vet ./...
	@echo "==> verify: gofmt"
	test -z "$$(gofmt -l .)"
	@echo "==> lint"
	CGO_ENABLED=$(CGO_ENABLED) golangci-lint run
	@echo "==> unit tests"
	CGO_ENABLED=$(CGO_ENABLED) $(GOTEST) ./internal/... -run "^Test[^I]" -skip "Integration"
	@echo "==> race detector"
	CGO_ENABLED=1 $(GOTEST) -race -short ./internal/... -run "^Test[^I]" -skip "Integration"
	@echo "==> build"
	GOOS=linux GOARCH=amd64 $(MAKE) build BUILD_DIR=$(BUILD_DIR)/ci-linux-amd64
	GOOS=linux GOARCH=arm64 $(MAKE) build BUILD_DIR=$(BUILD_DIR)/ci-linux-arm64
	$(MAKE) build
	@echo "==> CI mirror passed"

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

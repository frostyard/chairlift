.PHONY: all build build-app build-helper build-first-run run clean deps tidy test e2e dev install-deps fmt lint lint-version-check check verify verify-static verify-template corrections-base ci build-linux-amd64 build-linux-arm64 install uninstall bump

# Binary names
BINARY_NAME=chairlift
HELPER_NAME=chairlift-updex-helper
# Pure-Go first-run gate (ADR-0014); it must never import a GTK binding
FIRSTRUN_NAME=chairlift-first-run

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

# Pinned golangci-lint release, read from mise.toml — the single source of
# every tool pin: `mise install` provisions it locally and in CI
# (jdx/mise-action), verified against mise.lock. Bump it there in a dedicated
# commit; never edit this line. Compared against `golangci-lint version
# --short`, which prints the bare "MAJOR.MINOR.PATCH".
GOLANGCI_LINT_VERSION := $(strip $(shell sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml))
# The Go release this module is built with, from go.mod's go line — the only
# Go pin (mise reads the same line). golangci-lint must be built with a Go at
# least this new, or its embedded gofmt and typechecker disagree with the
# toolchain.
GO_VERSION := $(strip $(shell sed -n 's/^go \(.*\)/\1/p' go.mod))
# The major version of the Node on PATH (mise.toml pins it), checked by
# make verify-template so the template gate needs no package install.
NODE_MAJOR := $(shell node -p "process.versions.node.split('.')[0]" 2>/dev/null)

# CGO is not needed with puregotk!
CGO_ENABLED=0

all: deps build

deps:
	$(GOMOD) download

tidy:
	$(GOMOD) tidy

build: build-app build-helper build-first-run

build-app:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/chairlift

build-helper:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(HELPER_NAME) ./cmd/chairlift-updex-helper

build-first-run:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -o $(BUILD_DIR)/$(FIRSTRUN_NAME) ./cmd/chairlift-first-run

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --dry-run

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

test:
	$(GOTEST) -v ./...

# End-to-end smoke tests require GTK4, Libadwaita, dbus-run-session, and Xvfb.
# They also run the pure-Go chairlift-first-run gate against temporary homes.
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

# Lint code (requires golangci-lint; fails if the mise.toml pin is missing or
# the installed release differs)
lint:
	$(MAKE) --no-print-directory lint-version-check
	CGO_ENABLED=$(CGO_ENABLED) golangci-lint run

# Fail unless the installed golangci-lint is the mise.toml pin and was built
# with a Go no older than go.mod's go line
lint-version-check:
	@test -n "$(GOLANGCI_LINT_VERSION)" || { echo "mise.toml pins no golangci-lint"; exit 1; }
	@installed="$$(golangci-lint version --short 2>/dev/null)" || { \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required (not installed; run: mise install)"; exit 1; }; \
	if [ "$$installed" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "expected golangci-lint $(GOLANGCI_LINT_VERSION), found $$installed (run: mise install)"; \
		exit 1; \
	fi; \
	built="$$(golangci-lint version 2>/dev/null | sed -n 's/.*built with go\([0-9.]*\).*/\1/p')"; \
	if [ -n "$$built" ] && [ "$$(printf '%s\n%s\n' "$(GO_VERSION)" "$$built" | sort -V | head -1)" != "$(GO_VERSION)" ]; then \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) was built with go$$built, older than go.mod's go$(GO_VERSION): bump golangci-lint first"; \
		exit 1; \
	fi

# Check the agent surfaces, docs index, risk paths, corrections and vendored
# copies (needs Node.js 22 or later, from mise.toml)
verify-template:
	@if ! command -v node >/dev/null 2>&1; then echo "FAIL: verify-template requires Node.js 22 or later (run: mise install)"; exit 1; fi
	@if [ "$(NODE_MAJOR)" = "" ] || [ "$(NODE_MAJOR)" -lt 22 ]; then echo "FAIL: verify-template requires Node.js 22 or later (found: $$(node --version 2>/dev/null || echo missing)); run: mise install"; exit 1; fi
	node scripts/check-template.mjs
	node --test test/*.test.mjs

# The non-mutating static checks shared by verify and ci: tidy diff, vet,
# gofmt, the exact-pin lint, and the template gate
verify-static:
	@echo "==> verify: go.mod is tidy"
	$(GOMOD) tidy -diff
	@echo "==> verify: go vet"
	CGO_ENABLED=$(CGO_ENABLED) $(GOCMD) vet ./...
	@echo "==> verify: gofmt"
	test -z "$$(gofmt -l .)"
	@echo "==> lint"
	$(MAKE) --no-print-directory lint
	@echo "==> verify: template, docs index, risk paths, corrections, vendored copies"
	$(MAKE) --no-print-directory verify-template

# Credential-free, non-mutating gate for read-only reviewers: verify-static
# plus the unit tests. It changes no file and needs no secret.
verify: verify-static
	@echo "==> unit tests"
	CGO_ENABLED=$(CGO_ENABLED) $(GOTEST) ./internal/... -run "^Test[^I]" -skip "Integration"

# Developer gate: format, then verify
check: fmt verify

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
	# Install the first-run gate; the image, not ChairLift, ships its trigger
	install -Dm755 $(BUILD_DIR)/$(FIRSTRUN_NAME) $(DESTDIR)$(BINDIR)/$(FIRSTRUN_NAME)
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
	rm -f $(DESTDIR)$(BINDIR)/$(FIRSTRUN_NAME)
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.bootc.policy
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.bootc.rules
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.updex.policy
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.updex.rules
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.sysupdate.policy

# One command mirrors CI's host-independent gates. It runs verify (the
# go.mod tidy diff, go vet, the gofmt check, the pinned golangci-lint, the
# template gate and the unit tests), then the corrections check against BASE
# (corrections-base), then the race detector, then the build matrix, in
# fail-fast order. The GTK/Xvfb-dependent E2E job runs separately through
# `make e2e`.
#
# The build step reproduces CI's GOOS/GOARCH matrix (linux/amd64 and
# linux/arm64) into per-arch subdirectories, then rebuilds natively so
# build/chairlift is left runnable on this host. A host-arch-only build would
# let an arm64 compile failure pass locally and break CI.
ci: verify
	$(MAKE) --no-print-directory corrections-base
	@echo "==> race detector"
	CGO_ENABLED=1 $(GOTEST) -race -short ./internal/... -run "^Test[^I]" -skip "Integration"
	@echo "==> build"
	GOOS=linux GOARCH=amd64 $(MAKE) build BUILD_DIR=$(BUILD_DIR)/ci-linux-amd64
	GOOS=linux GOARCH=arm64 $(MAKE) build BUILD_DIR=$(BUILD_DIR)/ci-linux-arm64
	$(MAKE) build
	@echo "==> CI mirror passed"

# The corrections check compares .memory/corrections.jsonl with BASE: lines
# are append-only (only an empty promoted_to may be filled in). The contract
# starts with the gate: a BASE that does not hold
# scripts/check-corrections.mjs had no such rule, so the comparison is skipped
# with a notice (ADR-0016). Every BASE after the adoption holds the script.
#
# A BASE that does not resolve to a commit is an error, never an old base: a
# typo or an unfetched ref must not turn the comparison into a silent pass.
corrections-base:
	@if [ -z "$(BASE)" ]; then exit 0; fi; \
	if ! git rev-parse --verify --quiet "$(BASE)^{commit}" >/dev/null; then \
		echo "FAIL: BASE $(BASE) does not resolve to a commit (fetch it first)"; \
		exit 1; \
	fi; \
	if git cat-file -e "$(BASE):scripts/check-corrections.mjs" 2>/dev/null; then \
		node scripts/check-corrections.mjs --base "$(BASE)"; \
	else \
		echo "==> corrections: BASE $(BASE) predates the corrections gate; append-only comparison skipped"; \
	fi

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

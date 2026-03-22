# ==============================================================
# Makefile — watcher agent
# ==============================================================
# WSL-aware: resolves the Go binary from the system PATH.
# If your Go installation is in a non-standard location,
# override it when calling make:
#   make build GO=/usr/local/go/bin/go
# ==============================================================

# ── Go binary ─────────────────────────────────────────────────
# Resolves from PATH automatically (works in WSL, Linux, macOS).
# Override with: make <target> GO=/path/to/go
GO := $(shell which go)

ifeq ($(GO),)
$(error "go not found in PATH. Install Go or set GO=/path/to/go")
endif

# ── Project config ────────────────────────────────────────────
MODULE      := $(shell $(GO) list -m 2>/dev/null)
CMD_PATH    := ./cmd/watcher
BINARY_NAME := watcher.exe
BIN_DIR     := bin
TEST_PKG    := ./internal/...
WEB_DIR     := web

# ── Build config ──────────────────────────────────────────────
GOOS        := windows
GOARCH      := amd64
CGO_ENABLED := 0

# ── Version (from git tag or fallback) ───────────────────────
VERSION     := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

# ── Output path ───────────────────────────────────────────────
OUT := $(BIN_DIR)/$(BINARY_NAME)

# ==============================================================
# Targets
# ==============================================================

.PHONY: all build build-web package test test-github test-verbose run dev clean info help

## all: run tests then build
all: test build

## dev: start air hot-reload (install air if missing)
dev:
	@command -v air > /dev/null 2>&1 || (echo "Installing air..." && $(GO) install github.com/air-verse/air@latest)
	@echo ""
	@echo "  ▸ Starting dev server with hot reload..."
	@echo "  ▸ API at http://localhost:$${API_PORT:-8080}"
	@echo "  ▸ Run 'cd web && bun run dev' in another terminal for SPA dev server"
	@echo ""
	air -c .air.toml

## build-web: build the SvelteKit SPA into web/build/
build-web:
	@echo ""
	@echo ">>> Building SvelteKit SPA"
	@echo "    Dir : $(WEB_DIR)"
	@echo ""
	cd $(WEB_DIR) && bun install --frozen-lockfile && bun run build
	@echo ""
	@echo "    OK: $(WEB_DIR)/build/"
	@echo ""

## build: build SPA + cross-compile watcher.exe for Windows (amd64)
build: build-web
	@echo ""
	@echo ">>> Building $(OUT)"
	@echo "    Go      : $(GO)"
	@echo "    Module  : $(MODULE)"
	@echo "    Version : $(VERSION)"
	@echo "    Target  : $(GOOS)/$(GOARCH)"
	@echo ""
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build \
		-ldflags="$(LDFLAGS)" \
		-o $(OUT) \
		$(CMD_PATH)
	@echo ""
	@echo "    OK: $(OUT) (SPA embedded)"
	@echo ""

## package: build watcher.exe and zip with shell/ scripts + .env.example + INSTALL.md
package: build
	@echo ""
	@echo ">>> Packaging release zip"
	@mkdir -p $(BIN_DIR)/staging/shell
	@cp $(OUT)                       $(BIN_DIR)/staging/
	@cp install.bat                  $(BIN_DIR)/staging/
	@cp shell/install-watcher.ps1    $(BIN_DIR)/staging/shell/
	@cp .env.example                 $(BIN_DIR)/staging/
	@cp INSTALL.md                   $(BIN_DIR)/staging/
	@cd $(BIN_DIR)/staging && zip -r ../$(APP_NAME)-$(VERSION).zip . && cd ../..
	@echo ""
	@echo "    OK: $(BIN_DIR)/$(APP_NAME)-$(VERSION).zip"
	@echo "    Contents:"
	@unzip -l $(BIN_DIR)/$(APP_NAME)-$(VERSION).zip
	@echo ""

## test: run all tests
test:
	@echo ""
	@echo ">>> Running all tests"
	@echo "    Go  : $(GO)"
	@echo "    Pkg : $(TEST_PKG)"
	@echo ""
	$(GO) test $(TEST_PKG) -count=1

## test-github: run only github.go tests
test-github:
	@echo ""
	@echo ">>> Running github.go tests"
	@echo ""
	$(GO) test ./internal/agent/ -count=1 -run "TestParse|TestFetchMetadata|TestDownloadArtifact|TestNewRequest" -v

## test-verbose: run all tests with verbose output
test-verbose:
	@echo ""
	@echo ">>> Running all tests (verbose)"
	@echo ""
	$(GO) test $(TEST_PKG) -count=1 -v

## run: run the watcher locally (uses .env in current dir)
run:
	@echo ""
	@echo ">>> Running watcher (native OS, not Windows)"
	@echo "    Config : .env"
	@echo ""
	$(GO) run $(CMD_PATH) -config .env

## clean: remove build artifacts
clean:
	@echo ">>> Cleaning $(BIN_DIR)/ and $(WEB_DIR)/build/"
	@rm -rf $(BIN_DIR)
	@rm -rf $(WEB_DIR)/build
	@echo "    Done"

## info: print resolved Go environment
info:
	@echo ""
	@echo "=== Go Environment ==="
	@echo "  Binary  : $(GO)"
	@echo "  Version : $(shell $(GO) version)"
	@echo "  GOPATH  : $(shell $(GO) env GOPATH)"
	@echo "  GOROOT  : $(shell $(GO) env GOROOT)"
	@echo "  Module  : $(MODULE)"
	@echo "  Version : $(VERSION)"
	@echo ""

## help: show available targets
help:
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""
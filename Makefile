# ============================================================================
# RMM Build System
# Cross-platform builds using Go's native compilation
# ============================================================================

# --- Configuration -----------------------------------------------------------

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
BUILD_TIME  := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
VERSION_PKG := github.com/avaropoint/rmm/internal/version
LDFLAGS     := -ldflags "-s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)"

BIN_DIR     := bin
RELEASE_DIR := release
DATA_DIR    := data
CERTS_DIR   := certs

PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	linux/arm \
	windows/amd64 \
	windows/arm64

.PHONY: all server agent agents build-% \
        lint check \
        dev dev-tls dev-fresh enroll enroll-tls run-server run-agent stop \
        dev-certs \
        release checksums package-web dist \
        clean install help

# --- Build -------------------------------------------------------------------

all: lint server agents

server: lint
	@echo "Building server..."
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/server ./cmd/server

agent: lint
	@echo "Building agent..."
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/agent ./cmd/agent

agents:
	@echo "Building agents for all platforms..."
	@mkdir -p $(BIN_DIR)
	@$(foreach platform,$(PLATFORMS),\
		$(eval OS := $(word 1,$(subst /, ,$(platform))))\
		$(eval ARCH := $(word 2,$(subst /, ,$(platform))))\
		$(eval EXT := $(if $(filter windows,$(OS)),.exe,))\
		echo "  -> $(OS)/$(ARCH)" && \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
			-o $(BIN_DIR)/agent-$(OS)-$(ARCH)$(EXT) ./cmd/agent || exit 1;\
	)
	@echo "Done!" && ls -la $(BIN_DIR)/agent-*

build-%:
	@mkdir -p $(BIN_DIR)
	$(eval PARTS := $(subst -, ,$*))
	$(eval OS := $(word 1,$(PARTS)))
	$(eval ARCH := $(word 2,$(PARTS)))
	$(eval EXT := $(if $(filter windows,$(OS)),.exe,))
	@echo "Building agent for $(OS)/$(ARCH)..."
	GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
		-o $(BIN_DIR)/agent-$(OS)-$(ARCH)$(EXT) ./cmd/agent

# --- Quality -----------------------------------------------------------------

lint:
	@UNFORMATTED=$$(gofmt -l ./cmd/ ./internal/ 2>/dev/null); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Error: unformatted files:"; echo "$$UNFORMATTED"; \
		echo "Run: gofmt -w ./cmd/ ./internal/"; exit 1; \
	fi
	@go vet ./...

check:
	@echo "Running checks..."
	@$(MAKE) lint
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi
	@go build ./...
	@echo "All checks passed."

# --- Development -------------------------------------------------------------
# Workflow:
#   1. make run-server          (start server in background)
#   2. make enroll CODE=<code>  (one-time agent enrollment)
#   3. make run-agent           (connect enrolled agent)
#
# Or use 'make dev' / 'make dev-tls' for all-in-one.
# Use 'make dev-fresh' to wipe state and start clean.

run-server: server
	./$(BIN_DIR)/server -insecure -web ./web

run-agent: agent
	./$(BIN_DIR)/agent -insecure

dev: server agent
	@echo "Starting server (insecure mode)..."
	@./$(BIN_DIR)/server -insecure -web ./web &
	@sleep 2
	@echo "Starting agent (insecure mode)..."
	@./$(BIN_DIR)/agent -insecure

dev-tls: server agent
	@echo "Starting server (self-signed TLS)..."
	@./$(BIN_DIR)/server -web ./web &
	@sleep 2
	@echo "Starting agent..."
	@./$(BIN_DIR)/agent

dev-fresh: stop clean
	@rm -rf $(DATA_DIR) $(CERTS_DIR)
	@echo "Cleaned all state. Build and start fresh:"
	@echo "  make run-server"

enroll: agent
	@if [ -z "$(CODE)" ]; then \
		echo "Usage: make enroll CODE=<enrollment-code>"; \
		echo ""; \
		echo "Steps:"; \
		echo "  1. make run-server"; \
		echo "  2. Log in to dashboard, create enrollment token"; \
		echo "  3. make enroll CODE=<token>"; \
		exit 1; \
	fi
	./$(BIN_DIR)/agent -server http://localhost:8080 -enroll $(CODE) -insecure

enroll-tls: agent
	@if [ -z "$(CODE)" ]; then \
		echo "Usage: make enroll-tls CODE=<enrollment-code>"; \
		exit 1; \
	fi
	./$(BIN_DIR)/agent -server https://localhost:8443 -enroll $(CODE) -insecure

stop:
	@pkill -f "bin/server" 2>/dev/null || true
	@pkill -f "bin/agent" 2>/dev/null || true
	@echo "Stopped processes"

# --- Local TLS (mkcert) ------------------------------------------------------
# Generate locally-trusted TLS certificates using mkcert.
# No browser warnings, no agent trust issues, works behind firewalls.
#
#   brew install mkcert   (macOS)
#   mkcert -install        (one-time: installs local CA)
#   make dev-certs         (generates certs in certs/)
#
# Then: ./bin/server -cert certs/local.crt -key certs/local.key -web ./web

dev-certs:
	@if ! command -v mkcert >/dev/null 2>&1; then \
		echo "mkcert not found. Install it:"; \
		echo "  macOS:  brew install mkcert && mkcert -install"; \
		echo "  Linux:  https://github.com/FiloSottile/mkcert#installation"; \
		exit 1; \
	fi
	@mkdir -p $(CERTS_DIR)
	mkcert -cert-file $(CERTS_DIR)/local.crt -key-file $(CERTS_DIR)/local.key \
		localhost 127.0.0.1 ::1 $$(hostname)
	@echo ""
	@echo "Certificates generated in $(CERTS_DIR)/"
	@echo "Run server with: ./$(BIN_DIR)/server -cert $(CERTS_DIR)/local.crt -key $(CERTS_DIR)/local.key -web ./web"

# --- Release -----------------------------------------------------------------

release: clean
	@mkdir -p $(RELEASE_DIR)
	@echo "Building release $(VERSION)..."
	@$(foreach platform,$(PLATFORMS),\
		$(eval OS := $(word 1,$(subst /, ,$(platform))))\
		$(eval ARCH := $(word 2,$(subst /, ,$(platform))))\
		$(eval EXT := $(if $(filter windows,$(OS)),.exe,))\
		echo "  -> agent $(OS)/$(ARCH)" && \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
			-o $(RELEASE_DIR)/agent-$(OS)-$(ARCH)$(EXT) ./cmd/agent || exit 1;\
		echo "  -> server $(OS)/$(ARCH)" && \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
			-o $(RELEASE_DIR)/server-$(OS)-$(ARCH)$(EXT) ./cmd/server || exit 1;\
	)
	@cp scripts/install.sh $(RELEASE_DIR)/ 2>/dev/null || true
	@cp scripts/install.ps1 $(RELEASE_DIR)/ 2>/dev/null || true
	@chmod +x $(RELEASE_DIR)/install.sh 2>/dev/null || true
	@echo "" && echo "Release $(VERSION) complete!" && ls -la $(RELEASE_DIR)/

checksums:
	@cd $(RELEASE_DIR) && shasum -a 256 * > checksums.txt
	@echo "Created $(RELEASE_DIR)/checksums.txt"

package-web:
	@mkdir -p $(RELEASE_DIR)
	@tar -czf $(RELEASE_DIR)/web.tar.gz web/
	@echo "Packaged web assets"

dist: release checksums package-web
	@echo "" && echo "Distribution ready!" && echo ""
	@cat $(RELEASE_DIR)/checksums.txt

# --- Utility -----------------------------------------------------------------

clean:
	rm -rf $(BIN_DIR) $(RELEASE_DIR)

install: server agent
	@sudo cp $(BIN_DIR)/agent /usr/local/bin/agent
	@sudo cp $(BIN_DIR)/server /usr/local/bin/server
	@echo "Installed to /usr/local/bin/"

help:
	@echo "RMM Build System"
	@echo ""
	@echo "Build:"
	@echo "  make              Build server + all agent platforms"
	@echo "  make server       Build server (current platform)"
	@echo "  make agent        Build agent (current platform)"
	@echo "  make agents       Build agents for ALL platforms"
	@echo "  make build-OS-ARCH  Build agent for specific platform"
	@echo ""
	@echo "Development:"
	@echo "  make dev            Run insecure (no TLS)"
	@echo "  make dev-tls        Run with self-signed TLS"
	@echo "  make dev-fresh      Wipe state and start clean"
	@echo "  make dev-certs      Generate trusted local certs (mkcert)"
	@echo "  make run-server     Start server (insecure)"
	@echo "  make run-agent      Start enrolled agent (insecure)"
	@echo "  make enroll CODE=   Enroll agent (insecure server)"
	@echo "  make enroll-tls CODE=  Enroll agent (TLS server)"
	@echo "  make stop           Stop running processes"
	@echo ""
	@echo "Quality:"
	@echo "  make lint           Format check + go vet"
	@echo "  make check          Full CI check (lint + build)"
	@echo ""
	@echo "Release:"
	@echo "  make release        Build all binaries"
	@echo "  make dist           Release + checksums + web"
	@echo "  make clean          Remove build artifacts"
	@echo ""
	@echo "TLS Modes:"
	@echo "  -insecure           No TLS (dev only)"
	@echo "  (default)           Self-signed certs (auto-generated)"
	@echo "  -cert/-key          Custom certificate"
	@echo "  -acme <domain>      Let's Encrypt automatic certs"
	@echo "  dev-certs + -cert   Trusted local certs via mkcert"
	@echo ""
	@echo "Platforms: darwin/{amd64,arm64} linux/{amd64,arm64,arm} windows/{amd64,arm64}"

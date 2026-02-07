# Build System
# Build for all platforms from any machine using Go's native cross-compilation

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
VERSION_PKG := github.com/avaropoint/rmm/internal/version
LDFLAGS := -ldflags "-s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)"

# Output directories
BIN_DIR := bin
RELEASE_DIR := release

# All supported platforms for the agent
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	linux/arm \
	windows/amd64 \
	windows/arm64

.PHONY: all clean server agent agents release dist help dev stop install lint check

# Default: build everything
all: lint server agents

# Build server for current platform
server: lint
	@echo "Building server..."
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/server ./cmd/server

# Build agent for current platform only
agent: lint
	@echo "Building agent for current platform..."
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/agent ./cmd/agent

# Build agents for ALL platforms
agents:
	@echo "Building agents for all platforms..."
	@mkdir -p $(BIN_DIR)
	@$(foreach platform,$(PLATFORMS),\
		$(eval OS := $(word 1,$(subst /, ,$(platform))))\
		$(eval ARCH := $(word 2,$(subst /, ,$(platform))))\
		$(eval EXT := $(if $(filter windows,$(OS)),.exe,))\
		echo "  → $(OS)/$(ARCH)" && \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
			-o $(BIN_DIR)/agent-$(OS)-$(ARCH)$(EXT) ./cmd/agent || exit 1;\
	)
	@echo "Done! Agents built:"
	@ls -la $(BIN_DIR)/agent-*

# Build for a specific platform: make build-darwin-arm64
build-%:
	@mkdir -p $(BIN_DIR)
	$(eval PARTS := $(subst -, ,$*))
	$(eval OS := $(word 1,$(PARTS)))
	$(eval ARCH := $(word 2,$(PARTS)))
	$(eval EXT := $(if $(filter windows,$(OS)),.exe,))
	@echo "Building agent for $(OS)/$(ARCH)..."
	GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
		-o $(BIN_DIR)/agent-$(OS)-$(ARCH)$(EXT) ./cmd/agent

# Run server locally
run-server: server
	./$(BIN_DIR)/server

# Run agent locally
run-agent: agent
	./$(BIN_DIR)/agent -server ws://localhost:8080

# Clean build artifacts
clean:
	rm -rf $(BIN_DIR) $(RELEASE_DIR)

# Lint: format check + vet (fast, runs before every build)
lint:
	@UNFORMATTED=$$(gofmt -l ./cmd/ ./internal/ 2>/dev/null); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Error: Go files are not formatted:"; \
		echo "$$UNFORMATTED"; \
		echo "Run 'gofmt -w ./cmd/ ./internal/' to fix."; \
		exit 1; \
	fi
	@go vet ./...

# CI check: format + vet + build all (used by CI pipeline)
check:
	@echo "Running checks..."
	@$(MAKE) lint
	@go build ./...
	@echo "All checks passed."

# === RELEASE TARGETS ===

# Build release for all platforms
release: clean
	@mkdir -p $(RELEASE_DIR)
	@echo "Building release $(VERSION)..."
	@$(foreach platform,$(PLATFORMS),\
		$(eval OS := $(word 1,$(subst /, ,$(platform))))\
		$(eval ARCH := $(word 2,$(subst /, ,$(platform))))\
		$(eval EXT := $(if $(filter windows,$(OS)),.exe,))\
		echo "  → agent $(OS)/$(ARCH)" && \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
			-o $(RELEASE_DIR)/agent-$(OS)-$(ARCH)$(EXT) ./cmd/agent || exit 1;\
		echo "  → server $(OS)/$(ARCH)" && \
		GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
			-o $(RELEASE_DIR)/server-$(OS)-$(ARCH)$(EXT) ./cmd/server || exit 1;\
	)
	@cp scripts/install.sh $(RELEASE_DIR)/ 2>/dev/null || true
	@cp scripts/install.ps1 $(RELEASE_DIR)/ 2>/dev/null || true
	@chmod +x $(RELEASE_DIR)/install.sh 2>/dev/null || true
	@echo ""
	@echo "Release $(VERSION) complete!"
	@ls -la $(RELEASE_DIR)/

# Create checksums for release files
checksums:
	@cd $(RELEASE_DIR) && shasum -a 256 * > checksums.txt
	@echo "Created $(RELEASE_DIR)/checksums.txt"

# Package web assets
package-web:
	@mkdir -p $(RELEASE_DIR)
	@tar -czf $(RELEASE_DIR)/web.tar.gz web/
	@echo "Packaged web assets"

# Full distribution: release + checksums + web
dist: release checksums package-web
	@echo ""
	@echo "Distribution ready!"
	@echo ""
	@cat $(RELEASE_DIR)/checksums.txt

# === DEVELOPMENT TARGETS ===

# Quick dev: build and run both
dev: server agent
	@echo "Starting server..."
	@./$(BIN_DIR)/server -web ./web &
	@sleep 2
	@echo "Starting agent..."
	@./$(BIN_DIR)/agent -server ws://localhost:8080

# Stop running processes
stop:
	@pkill -f "bin/server" 2>/dev/null || true
	@pkill -f "bin/agent" 2>/dev/null || true
	@echo "Stopped processes"

# Install locally
install: server agent
	@sudo cp $(BIN_DIR)/agent /usr/local/bin/agent
	@sudo cp $(BIN_DIR)/server /usr/local/bin/server
	@echo "Installed to /usr/local/bin/"

# Show help
help:
	@echo "Build System"
	@echo ""
	@echo "Build Targets:"
	@echo "  make              - Build server + all agent platforms"
	@echo "  make server       - Build server for current platform"
	@echo "  make agent        - Build agent for current platform"
	@echo "  make agents       - Build agents for ALL platforms"
	@echo "  make build-OS-ARCH - Build agent for specific platform"
	@echo ""
	@echo "Release Targets:"
	@echo "  make release      - Build all binaries for release"
	@echo "  make dist         - Full release with checksums + web"
	@echo "  make checksums    - Generate SHA256 checksums"
	@echo "  make package-web  - Package web assets"
	@echo ""
	@echo "Development:"
	@echo "  make dev          - Build and run server + agent"
	@echo "  make run-server   - Build and run server"
	@echo "  make run-agent    - Build and run agent"
	@echo "  make stop         - Stop running processes"
	@echo "  make install      - Install to /usr/local/bin"
	@echo "  make lint         - Check formatting + vet"
	@echo "  make check        - Full CI check (lint + build)"
	@echo "  make clean        - Remove all build artifacts"
	@echo ""
	@echo "Supported platforms:"
	@echo "  darwin/amd64  - macOS Intel"
	@echo "  darwin/arm64  - macOS Apple Silicon"
	@echo "  linux/amd64   - Linux x86_64"
	@echo "  linux/arm64   - Linux ARM64 (Raspberry Pi 4, etc)"
	@echo "  linux/arm     - Linux ARM32 (older Raspberry Pi)"
	@echo "  windows/amd64 - Windows x86_64"
	@echo "  windows/arm64 - Windows ARM64"
	@echo ""
	@echo "Examples:"
	@echo "  make release VERSION=v1.0.0"
	@echo "  make dist"
	@echo "  make build-windows-amd64"

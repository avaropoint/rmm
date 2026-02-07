# RMM — Remote Monitoring & Management

A lightweight, cross-platform remote desktop and monitoring system built in Go.
Zero external runtime dependencies — a single binary for the server and a single
binary for each agent.

## Features

- **Real-time remote desktop** — JPEG screen capture streamed over binary
  WebSocket frames, rendered with `createImageBitmap` for zero-copy GPU
  compositing in the browser
- **Remote input** — Keyboard and mouse events forwarded from the browser to the
  agent
- **Cross-platform agents** — Builds for macOS (amd64/arm64), Linux
  (amd64/arm64/arm), and Windows (amd64/arm64)
- **Enrollment-based security** — Agents enroll via time-limited tokens;
  credentials are HMAC-SHA-512 signed by the server's Ed25519 platform identity
- **Four TLS modes** — Off (dev), self-signed (auto-generated), ACME
  (Let's Encrypt), and custom certificates
- **API key authentication** — Dashboard and REST APIs protected by bearer token
  auth
- **Pure Go SQLite** — Embedded database via `modernc.org/sqlite` — no CGo, no
  external database server
- **Single-binary deployment** — Server and agent each compile to a single
  static binary

## Quick Start

### Prerequisites

- Go 1.24+

### Build

```bash
make server agent
```

### Run (Development)

```bash
# Terminal 1 — Start server (insecure, no TLS)
./bin/server -insecure -web ./web

# Note the API key printed on first run — save it.

# Terminal 2 — Create an enrollment token
curl -X POST http://localhost:8080/api/enrollment \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"type":"attended","label":"my laptop"}'

# Terminal 3 — Enroll the agent
./bin/agent -server http://localhost:8080 -enroll <CODE> -insecure

# After enrollment, reconnect without the enrollment code:
./bin/agent -insecure
```

Open `http://localhost:8080` in a browser to access the dashboard.

### Run (Production — Self-Signed TLS)

```bash
# Server auto-generates CA + server certs in certs/
./bin/server -web ./web

# Enroll agent (accepts self-signed cert during enrollment)
./bin/agent -server https://yourhost:8443 -enroll <CODE> -insecure

# Subsequent connections trust the CA received at enrollment
./bin/agent
```

### Run (Production — Let's Encrypt)

```bash
# Requires ports 80 + 443 open, DNS pointing to this server
./bin/server -acme rmm.example.com -web ./web

# Agent trusts the ACME cert via system CA store
./bin/agent -server https://rmm.example.com -enroll <CODE>
```

## TLS Modes

| Mode | Flag | Listen | Certificates |
|------|------|--------|--------------|
| Off | `-insecure` | `:8080` | None (dev only) |
| Self-signed | *(default)* | `:8443` | Auto-generated in `certs/` |
| ACME | `-acme domain` | `:443` | Let's Encrypt auto-managed |
| Custom | `-cert`/`-key` | `:8443` | User-provided files |

### Local TLS with mkcert

For development with browser-trusted certificates behind a firewall:

```bash
brew install mkcert    # macOS
mkcert -install        # one-time: installs local CA
make dev-certs         # generates certs/local.crt + certs/local.key

./bin/server -cert certs/local.crt -key certs/local.key -web ./web
```

## Server Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:8443` | Listen address (auto-adjusts per TLS mode) |
| `-web` | *(auto-detect)* | Path to web assets directory |
| `-data` | `data` | Directory for database and platform identity |
| `-certs` | `certs` | Directory for TLS certificates |
| `-insecure` | `false` | Disable TLS (development only) |
| `-acme` | | Domain for Let's Encrypt |
| `-cert` | | Path to custom TLS certificate |
| `-key` | | Path to custom TLS key |

## Agent Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-server` | | Server URL for enrollment |
| `-enroll` | | Enrollment code |
| `-name` | *(hostname)* | Agent display name |
| `-insecure` | `false` | Skip TLS certificate verification |

## REST API

All endpoints except enrollment and auth-verify require an `Authorization: Bearer <API_KEY>` header.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/api/enroll` | No | Agent enrollment (with token code) |
| GET | `/api/agents` | Yes | List connected agents |
| GET/POST/DELETE | `/api/enrollment` | Yes | Manage enrollment tokens |
| GET | `/api/auth/verify` | Yes | Verify API key validity |
| WS | `/ws/agent` | Credential | Agent WebSocket connection |
| WS | `/ws/viewer` | No | Browser viewer WebSocket |

## Architecture

```
┌─────────┐       WebSocket (binary frames)       ┌──────────┐
│  Agent   │◄─────────────────────────────────────►│  Server  │
│ (Go bin) │  screen capture, input events, reg    │ (Go bin) │
└─────────┘                                        └────┬─────┘
                                                        │
                                     HTTP API + WS      │  Static files
                                                        │
                                                   ┌────▼─────┐
                                                   │ Dashboard │
                                                   │ (Browser) │
                                                   └──────────┘
```

The server is a single Go binary that:
1. Accepts agent connections over WebSocket (binary frames for screen data, JSON text frames for control)
2. Serves a browser-based dashboard for viewing agents and remote desktop
3. Brokers binary screen frames from agents directly to viewers with no re-encoding
4. Manages enrollment, authentication, and state via embedded SQLite

## Project Structure

```
cmd/
  server/
    main.go              Entry point, flag parsing, TLS mode selection
    server.go            Server struct, LiveAgent, NewServer
    websocket.go         RFC 6455 WebSocket upgrade
    handler_agent.go     Agent connection lifecycle
    handler_viewer.go    Viewer connection lifecycle
    handler_api.go       REST API handlers
  agent/
    main.go              Entry point, enrollment, reconnect loop
    agent.go             WebSocket connection, message dispatch
    capture.go           Screen capture (JPEG encoding)
    input.go             Mouse/keyboard input injection
    sysinfo.go           System info collection
    sysinfo_*.go         Platform-specific implementations

internal/
  protocol/
    message.go           Shared message types (Registration, DisplayInfo)
    websocket.go         RFC 6455 frame reader/writer
  security/
    tls.go               TLS types, self-signed loader, custom cert loader
    tls_selfsigned.go    Self-signed CA + server cert generation (ECDSA P-384)
    tls_acme.go          Let's Encrypt automatic cert management
    platform.go          Ed25519 platform identity, credential signing
    hmac.go              HMAC-SHA-512, constant-time comparison
    token.go             Enrollment tokens, API keys
    middleware.go        HTTP authentication middleware
  store/
    store.go             Persistence interface (Store)
    sqlite.go            SQLite implementation
  version/
    version.go           Build version injection

web/                     Browser dashboard (vanilla JS, no build step)
  index.html
  css/
  js/
    core/                WebSocket, HTTP, events, utilities
    modules/             Agents list, remote viewer
    components/          Modal, toast, icons

data/                    Runtime (gitignored)
  platform.db            SQLite database
  platform.key           Ed25519 platform identity

certs/                   TLS certificates (gitignored)
  ca.crt                 Self-signed CA (auto-generated)
  server.crt             Server certificate
  server.key             Server private key
```

## Security Model

- **Platform identity** — Ed25519 keypair generated on first run, stored in
  `data/platform.key`. The SHA-256 fingerprint uniquely identifies the
  deployment.
- **Agent credentials** — HMAC-SHA-512 signed by a key derived (HKDF-SHA-512)
  from the platform identity. Format: `v1.<agentID>.<hmac_hex>`. Quantum-safe
  for authentication (256-bit security against Grover's algorithm). Version
  prefix allows future upgrade to ML-DSA (FIPS 204).
- **Enrollment tokens** — Short-lived, single-use codes (SHA-256 hashed in DB).
  Support attended and unattended types.
- **API keys** — `rmm_` prefixed, SHA-256 hashed. First key auto-generated on
  initial server start.
- **TLS** — Minimum TLS 1.3 enforced on all modes. Go 1.23+ automatically
  negotiates X25519+ML-KEM-768 hybrid post-quantum key exchange when both peers
  support it.
- **WebSocket** — Custom RFC 6455 implementation (no external dependencies).

## Make Targets

```
Build:
  make              Build server + all agent platforms
  make server       Build server (current platform)
  make agent        Build agent (current platform)
  make agents       Build agents for ALL platforms

Development:
  make dev          Run insecure (no TLS)
  make dev-tls      Run with self-signed TLS
  make dev-fresh    Wipe all state and rebuild
  make dev-certs    Generate trusted local certs (mkcert)
  make run-server   Start server (insecure)
  make run-agent    Start enrolled agent (insecure)
  make enroll CODE= Enroll agent (insecure server)
  make stop         Stop running processes

Quality:
  make lint         Format check + go vet
  make check        Full CI check (lint + build)

Release:
  make release      Cross-compile all binaries
  make dist         Release + checksums + web archive
  make clean        Remove build artifacts
```

## Dependencies

| Module | Purpose |
|--------|---------|
| `modernc.org/sqlite` | Pure Go SQLite (no CGo) |
| `golang.org/x/crypto` | HKDF, ACME/autocert |

No JavaScript build tools, bundlers, or npm packages. The web dashboard is
vanilla HTML/CSS/JS served as static files.

## License

Copyright Avaropoint. All rights reserved.

# Remote Desktop Management

[![CI](https://github.com/avaropoint/rmm/actions/workflows/ci.yml/badge.svg)](https://github.com/avaropoint/rmm/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A fast, lightweight remote desktop management platform built with pure Go (no CGo, no third-party dependencies). Cross-platform agent with real-time screen sharing, input injection, and multi-monitor support.

## Features

- **Real-time screen sharing** — low-latency JPEG streaming over WebSocket
- **Remote input injection** — mouse and keyboard control across platforms
- **Multi-monitor support** — switch between displays from the viewer
- **Cross-platform agent** — macOS, Linux, Windows (amd64 + arm64)
- **Web dashboard** — clean HTML/JS/CSS UI, no framework dependencies
- **Auto-reconnect** — agent reconnects automatically on disconnect
- **Zero dependencies** — pure Go stdlib, no CGo required

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│     Server      │◄───────►│      Agent      │
│   (port 8080)   │   WS    │                 │
│                 │         │ • Screen Cap    │
│ • Web Dashboard │         │ • Input Handler │
│ • Agent Broker  │         │ • Display Mgmt  │
└────────┬────────┘         └─────────────────┘
         │
         │ HTTP / WS
         ▼
┌─────────────────┐
│   Web Browser   │
│    (Viewer)     │
└─────────────────┘
```

## Quick Start

### Build

```bash
make            # Build server + all agent platforms
make server     # Build server for current platform
make agent      # Build agent for current platform
```

### Run

```bash
# Start server with web dashboard
./bin/server -web ./web

# Connect agent (same or different machine)
./bin/agent -server ws://localhost:8080
./bin/agent -server ws://your-server:8080 -name "My Workstation"
```

Open http://localhost:8080 in your browser, click **Connect** on an agent to start remote viewing.

## Project Structure

```
├── cmd/
│   ├── agent/          # Agent binary
│   │   ├── main.go     # Entry point
│   │   ├── agent.go    # Core agent logic, WebSocket client
│   │   ├── capture.go  # Screen capture (macOS, Linux, Windows)
│   │   └── input.go    # Input injection (mouse, keyboard)
│   └── server/         # Server binary
│       ├── main.go     # Entry point
│       └── server.go   # HTTP server, WebSocket broker
├── internal/
│   ├── protocol/       # Shared WebSocket protocol
│   │   ├── message.go  # Message types and constants
│   │   └── websocket.go# Frame read/write helpers
│   └── version/        # Build-time version info
│       └── version.go
├── web/                # Web dashboard assets
│   ├── index.html
│   ├── css/
│   │   ├── theme.css   # CSS custom properties (palette, typography)
│   │   └── main.css    # Component styles
│   └── js/
│       ├── app.js      # Dashboard entry point
│       ├── core/       # EventEmitter, HTTP, WebSocket, utils
│       ├── components/ # Icons, toast, modal
│       └── modules/    # AgentManager, ScreenViewer
├── scripts/
│   ├── install.sh      # Linux/macOS installer
│   └── install.ps1     # Windows installer
├── .github/workflows/
│   ├── ci.yml          # Build + vet on push/PR
│   └── release.yml     # Cross-compile + GitHub Release on tag
├── Makefile
├── go.mod
├── LICENSE
└── README.md
```

## API Endpoints

| Endpoint | Description |
|---|---|
| `GET /` | Web dashboard |
| `GET /api/agents` | List connected agents (JSON) |
| `WS /ws/agent` | Agent WebSocket endpoint |
| `WS /ws/viewer?agent=ID` | Viewer WebSocket endpoint |

## Platform Requirements

### Agent Dependencies

| Platform | Screen Capture | Input Injection |
|---|---|---|
| **macOS** | `screencapture` (built-in) | `cliclick` (`brew install cliclick`) |
| **Linux** | `gnome-screenshot` or `scrot` | `xdotool` (`apt install xdotool`) |
| **Windows** | PowerShell (built-in) | PowerShell (built-in) |

### macOS Permissions

The agent requires **Screen Recording** and **Accessibility** permissions in System Preferences → Privacy & Security.

## Building for Release

```bash
make release                    # All platforms
make release VERSION=v0.1.0     # With explicit version
make dist                       # Release + checksums + web package
```

## Installation

### From GitHub Releases

```bash
# Linux / macOS
curl -sSL https://github.com/avaropoint/rmm/releases/latest/download/install.sh | bash

# Windows (PowerShell as Admin)
irm https://github.com/avaropoint/rmm/releases/latest/download/install.ps1 | iex
```

### From Source

```bash
git clone https://github.com/avaropoint/rmm.git
cd rmm
make install
```

## Development

```bash
make dev        # Build and run server + agent locally
make stop       # Stop running processes
make clean      # Remove build artifacts
make help       # Show all available targets
```

## Supported Platforms

| Platform | Architecture | Agent | Server |
|---|---|---|---|
| macOS | amd64, arm64 | ✅ | ✅ |
| Linux | amd64, arm64, arm | ✅ | ✅ |
| Windows | amd64, arm64 | ✅ | ✅ |

## License

[MIT](LICENSE) — Copyright (c) 2026 Avaropoint

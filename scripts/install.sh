#!/bin/bash
#
# Agent Installer
# Usage: curl -sSL https://your-domain.com/install.sh | bash
#        curl -sSL https://your-domain.com/install.sh | bash -s -- --server ws://your-server:8080
#

set -e

# Configuration
REPO="avaropoint/rmm"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="agent"
VERSION="${VERSION:-latest}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Parse arguments
SERVER_URL=""
while [[ $# -gt 0 ]]; do
    case $1 in
        --server)
            SERVER_URL="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

print_banner() {
    echo ""
    echo -e "${CYAN}Agent Installer${NC}"
    echo ""
}

info() {
    echo -e "${CYAN}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

detect_os() {
    OS="$(uname -s)"
    ARCH="$(uname -m)"
    
    case "$OS" in
        Linux*)     OS="linux" ;;
        Darwin*)    OS="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
        *)          error "Unsupported OS: $OS" ;;
    esac
    
    case "$ARCH" in
        x86_64|amd64)   ARCH="amd64" ;;
        arm64|aarch64)  ARCH="arm64" ;;
        armv7l|armv6l)  ARCH="arm" ;;
        *)              error "Unsupported architecture: $ARCH" ;;
    esac
    
    info "Detected: $OS/$ARCH"
}

check_dependencies() {
    info "Checking dependencies..."
    
    if [ "$OS" = "linux" ]; then
        # Check for xdotool (needed for input injection)
        if ! command -v xdotool &> /dev/null; then
            warn "xdotool not found - installing..."
            if command -v apt-get &> /dev/null; then
                sudo apt-get update && sudo apt-get install -y xdotool
            elif command -v yum &> /dev/null; then
                sudo yum install -y xdotool
            elif command -v pacman &> /dev/null; then
                sudo pacman -S --noconfirm xdotool
            else
                warn "Could not install xdotool automatically. Please install it manually."
            fi
        fi
        
        # Check for screenshot tools
        if ! command -v gnome-screenshot &> /dev/null && ! command -v scrot &> /dev/null; then
            warn "No screenshot tool found - installing scrot..."
            if command -v apt-get &> /dev/null; then
                sudo apt-get install -y scrot
            elif command -v yum &> /dev/null; then
                sudo yum install -y scrot
            fi
        fi
    fi
    
    if [ "$OS" = "darwin" ]; then
        # Check for cliclick (needed for input injection)
        if ! command -v cliclick &> /dev/null; then
            if command -v brew &> /dev/null; then
                info "Installing cliclick via Homebrew..."
                brew install cliclick
            else
                warn "cliclick not found and Homebrew not available."
                warn "Please install Homebrew first: https://brew.sh"
                warn "Then run: brew install cliclick"
            fi
        fi
    fi
    
    success "Dependencies checked"
}

download_binary() {
    info "Downloading agent..."
    
    # Construct download URL
    if [ "$VERSION" = "latest" ]; then
        DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/agent-${OS}-${ARCH}"
    else
        DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/agent-${OS}-${ARCH}"
    fi
    
    # For local testing, check if binary exists locally (multiple locations)
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    LOCAL_PATHS=(
        "${SCRIPT_DIR}/../release/agent-${OS}-${ARCH}"
        "${SCRIPT_DIR}/../bin/agent-${OS}-${ARCH}"
        "./release/agent-${OS}-${ARCH}"
        "./bin/agent-${OS}-${ARCH}"
    )
    
    LOCAL_BIN=""
    for path in "${LOCAL_PATHS[@]}"; do
        if [ -f "$path" ]; then
            LOCAL_BIN="$path"
            break
        fi
    done
    
    if [ -n "$LOCAL_BIN" ]; then
        info "Using local binary: $LOCAL_BIN"
        TEMP_BIN="$LOCAL_BIN"
    else
        TEMP_BIN="/tmp/agent-$$"
        
        if command -v curl &> /dev/null; then
            curl -fsSL "$DOWNLOAD_URL" -o "$TEMP_BIN" || error "Download failed. URL: $DOWNLOAD_URL"
        elif command -v wget &> /dev/null; then
            wget -q "$DOWNLOAD_URL" -O "$TEMP_BIN" || error "Download failed. URL: $DOWNLOAD_URL"
        else
            error "Neither curl nor wget found. Please install one."
        fi
    fi
    
    chmod +x "$TEMP_BIN"
    success "Downloaded successfully"
}

install_binary() {
    info "Installing to ${INSTALL_DIR}..."
    
    # Create install directory if needed
    if [ ! -d "$INSTALL_DIR" ]; then
        sudo mkdir -p "$INSTALL_DIR"
    fi
    
    # Copy binary
    sudo cp "$TEMP_BIN" "${INSTALL_DIR}/agent"
    sudo chmod +x "${INSTALL_DIR}/agent"

    
    # Cleanup temp file
    if [ "$TEMP_BIN" != "$LOCAL_BIN" ] && [ -f "$TEMP_BIN" ]; then
        rm -f "$TEMP_BIN"
    fi
    
    success "Installed to ${INSTALL_DIR}/agent"
}

setup_macos_permissions() {
    echo ""
    echo -e "${YELLOW}macOS Permissions Required${NC}"
    echo ""
    echo "The agent needs two permissions to work properly:"
    echo ""
    echo -e "${CYAN}1. Screen Recording${NC} - to capture your screen"
    echo -e "${CYAN}2. Accessibility${NC} - to control mouse/keyboard"
    echo ""
    echo "Opening System Preferences..."
    echo ""
    
    # Open Privacy & Security preferences
    if [[ $(sw_vers -productVersion | cut -d. -f1) -ge 13 ]]; then
        # macOS Ventura and later
        open "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
        echo -e "Please add ${GREEN}Terminal${NC} (or your terminal app) to:"
        echo "  • Privacy & Security → Screen Recording"
        echo ""
        read -p "Press Enter when done, then we'll open Accessibility settings..."
        open "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
        echo ""
        echo -e "Please add ${GREEN}Terminal${NC} (or your terminal app) to:"
        echo "  • Privacy & Security → Accessibility"
    else
        # macOS Monterey and earlier
        open "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
        echo -e "Please add ${GREEN}Terminal${NC} to Screen Recording, then Accessibility"
    fi
    
    echo ""
    echo -e "${GREEN}After granting permissions, you may need to restart Terminal.${NC}"
    echo ""
}

setup_linux_service() {
    if [ -z "$SERVER_URL" ]; then
        warn "No server URL provided. Skipping service setup."
        warn "Run with: --server ws://your-server:8080"
        return
    fi
    
    info "Setting up systemd service..."
    
    # Create systemd service file
    sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null <<EOF
[Unit]
Description=Remote Desktop Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/agent -server ${SERVER_URL}
Restart=always
RestartSec=10
User=${SUDO_USER:-$USER}
Environment=DISPLAY=:0
Environment=XAUTHORITY=/home/${SUDO_USER:-$USER}/.Xauthority

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable ${SERVICE_NAME}
    sudo systemctl start ${SERVICE_NAME}
    
    success "Service installed and started"
    info "Check status: sudo systemctl status ${SERVICE_NAME}"
}

setup_macos_launchd() {
    if [ -z "$SERVER_URL" ]; then
        warn "No server URL provided. Skipping service setup."
        warn "To run manually: agent -server ws://your-server:8080"
        return
    fi
    
    info "Setting up launchd service..."
    
    PLIST_PATH="$HOME/Library/LaunchAgents/com.avaropoint.agent.plist"
    mkdir -p "$HOME/Library/LaunchAgents"
    
    cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.avaropoint.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/agent</string>
        <string>-server</string>
        <string>${SERVER_URL}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/agent.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/agent.log</string>
</dict>
</plist>
EOF

    launchctl unload "$PLIST_PATH" 2>/dev/null || true
    launchctl load "$PLIST_PATH"
    
    success "Service installed and started"
    info "Check logs: tail -f /tmp/agent.log"
    info "Stop service: launchctl unload $PLIST_PATH"
}

print_success() {
    echo ""
    echo -e "${GREEN}Installation complete.${NC}"
    echo ""
    echo "Binary installed: ${INSTALL_DIR}/agent"
    echo ""
    
    if [ -z "$SERVER_URL" ]; then
        echo "To run manually:"
        echo -e "  ${CYAN}agent -server ws://your-server:8080${NC}"
        echo ""
        echo "To install as a service, re-run with:"
        echo -e "  ${CYAN}curl -sSL ... | bash -s -- --server ws://your-server:8080${NC}"
    fi
    echo ""
}

# Main
print_banner
detect_os
check_dependencies
download_binary
install_binary

if [ "$OS" = "darwin" ]; then
    setup_macos_permissions
    setup_macos_launchd
elif [ "$OS" = "linux" ]; then
    setup_linux_service
fi

print_success

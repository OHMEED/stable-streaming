#!/bin/bash
# ===========================================
# Stable Stream Solutions - Installation Script
# ===========================================
# This script installs the application as a systemd service
# for automatic startup on boot.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo ""
echo "=============================================="
echo "  STABLE STREAM SOLUTIONS INSTALLER"
echo "=============================================="
echo ""

# Check for root/sudo privileges
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This installer must be run with sudo${NC}"
    echo "Usage: sudo ./install.sh"
    exit 1
fi

# Get the actual user who ran sudo
ACTUAL_USER="${SUDO_USER:-$USER}"
ACTUAL_HOME=$(getent passwd "$ACTUAL_USER" | cut -d: -f6)

# Determine installation directory (where this script is located)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="$SCRIPT_DIR"

echo "Installation directory: $INSTALL_DIR"
echo "Installing for user: $ACTUAL_USER"
echo ""

# ===========================================
# Step 1: Run dependencies check
# ===========================================
echo "Step 1: Checking dependencies..."
if [ -f "$INSTALL_DIR/scripts/install_dependencies.sh" ]; then
    chmod +x "$INSTALL_DIR/scripts/install_dependencies.sh"
    bash "$INSTALL_DIR/scripts/install_dependencies.sh"
fi

# ===========================================
# Step 2: Run configuration if needed
# ===========================================
CONFIG_FILE="/etc/stable-stream/config.env"
if [ ! -f "$CONFIG_FILE" ]; then
    echo ""
    echo "Step 2: Running initial configuration..."
    # Source the configuration functions from start.sh
    # For first install, we'll create default config
    mkdir -p /etc/stable-stream
    
    # Generate default password hash
    DEFAULT_HASH=$(python3 -c "import bcrypt; print(bcrypt.hashpw(b'password', bcrypt.gensalt()).decode())")
    
    cat > "$CONFIG_FILE" <<EOF
USERNAME="admin"
PASSWORD_HASH="$DEFAULT_HASH"
MULTISTREAM_ENABLED="y"
MAX_MULTISTREAMS="5"
INGESTS_LOCKED="n"
EOF
    
    echo -e "${GREEN}✓ Default configuration created (admin/password)${NC}"
else
    echo "Step 2: Configuration already exists, skipping..."
fi

# ===========================================
# Step 3: Build the backend
# ===========================================
echo ""
echo "Step 3: Building backend..."

cd "$INSTALL_DIR/backend"
export GO111MODULE=on
export PATH=$PATH:/usr/local/go/bin

go mod tidy
go build -o "$INSTALL_DIR/bin/server" main.go

echo -e "${GREEN}✓ Backend built successfully${NC}"

# ===========================================
# Step 4: Build the frontend
# ===========================================
echo ""
echo "Step 4: Building frontend..."

cd "$INSTALL_DIR/frontend"
npm install
npm run build

echo -e "${GREEN}✓ Frontend built successfully${NC}"

# ===========================================
# Step 5: Install CLI control script
# ===========================================
echo ""
echo "Step 5: Installing CLI control script..."

cp "$INSTALL_DIR/scripts/stable-stream" /usr/local/bin/stable-stream
chmod +x /usr/local/bin/stable-stream

echo -e "${GREEN}✓ CLI installed to /usr/local/bin/stable-stream${NC}"

# ===========================================
# Step 6: Save install path for updates
# ===========================================
echo "$INSTALL_DIR" > /etc/stable-stream/install_path
chown "$ACTUAL_USER:$ACTUAL_USER" /etc/stable-stream/install_path

# ===========================================
# Step 7: Create systemd service
# ===========================================
echo ""
echo "Step 6: Creating systemd service..."

cat > /etc/systemd/system/stable-stream.service <<EOF
[Unit]
Description=Stable Stream Solutions Server
After=network.target obs-wayland.service
Wants=obs-wayland.service

[Service]
Type=simple
User=$ACTUAL_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/bin/server
Restart=on-failure
RestartSec=5
Environment=HOME=$ACTUAL_HOME

# Load configuration
EnvironmentFile=/etc/stable-stream/config.env

[Install]
WantedBy=multi-user.target
EOF

# ===========================================
# Step 8: Enable and start service
# ===========================================
echo ""
echo "Step 7: Enabling service..."

systemctl daemon-reload
systemctl enable stable-stream.service

echo -e "${GREEN}✓ Service enabled for autostart on boot${NC}"

# ===========================================
# Step 9: Start the service
# ===========================================
echo ""
echo "Step 8: Starting service..."

systemctl start stable-stream.service

# Wait a moment for startup
sleep 2

if systemctl is-active --quiet stable-stream.service; then
    echo -e "${GREEN}✓ Service started successfully${NC}"
else
    echo -e "${YELLOW}⚠ Service may have failed to start. Check logs with: stable-stream logs${NC}"
fi

# ===========================================
# Complete
# ===========================================
echo ""
echo "=============================================="
echo -e "${GREEN}  INSTALLATION COMPLETE${NC}"
echo "=============================================="
echo ""
echo "Available commands:"
echo "  stable-stream start     - Start the service"
echo "  stable-stream stop      - Stop the service"
echo "  stable-stream restart   - Restart the service"
echo "  stable-stream status    - Check service status"
echo "  stable-stream configure - Re-run interactive configuration"
echo "  stable-stream logs      - View service logs"
echo "  stable-stream update    - Update from GitHub"
echo ""
echo "Default login: admin / password"
echo "Access the web UI at: http://$(hostname -I | awk '{print $1}'):8080"
echo ""

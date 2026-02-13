#!/bin/bash

# Build and Run Stable Streaming Solutions
set -e

# Aitum Plugin Installation Functions
# These plugins are GPL-2.0 licensed - source available at:
# - https://github.com/Aitum/obs-aitum-multistream
# - https://github.com/Aitum/obs-vertical-canvas

AITUM_MULTISTREAM_VERSION="1.0.7"
AITUM_VERTICAL_VERSION="1.6.1"

restart_obs() {
    echo "Restarting OBS..."
    if pgrep -x "obs" > /dev/null; then
        pkill -x "obs"
        sleep 2
    fi
    nohup obs > /dev/null 2>&1 &
    echo "✓ OBS restarted"
}

install_aitum_plugins() {
    echo "Installing Aitum OBS Plugins..."
    echo "These plugins are licensed under GPL-2.0"
    echo ""
    
    # Create temp directory
    TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"
    
    # Install obs-aitum-multistream
    echo "Downloading obs-aitum-multistream v${AITUM_MULTISTREAM_VERSION}..."
    curl -L -o multistream.deb \
        "https://github.com/Aitum/obs-aitum-multistream/releases/download/${AITUM_MULTISTREAM_VERSION}/aitum-multistream-linux-gnu.deb"
    
    echo "Installing obs-aitum-multistream..."
    sudo dpkg -i multistream.deb || sudo apt-get install -f -y
    
    # Install obs-vertical-canvas
    echo "Downloading obs-vertical-canvas v${AITUM_VERTICAL_VERSION}..."
    curl -L -o vertical.deb \
        "https://github.com/Aitum/obs-vertical-canvas/releases/download/${AITUM_VERTICAL_VERSION}/vertical-canvas-linux-gnu.deb"
    
    echo "Installing obs-vertical-canvas..."
    sudo dpkg -i vertical.deb || sudo apt-get install -f -y
    
    # Cleanup
    cd - > /dev/null
    rm -rf "$TEMP_DIR"
    
    echo ""
    echo "✓ Aitum plugins installed successfully!"
    echo "  - obs-aitum-multistream v${AITUM_MULTISTREAM_VERSION}"
    echo "  - obs-vertical-canvas v${AITUM_VERTICAL_VERSION}"
    restart_obs
}

uninstall_aitum_plugins() {
    echo "Uninstalling Aitum OBS Plugins..."
    
    # Check for multistream package (could be aitum-multistream or obs-aitum-multistream)
    MULTISTREAM_PKG=$(dpkg -l 2>/dev/null | grep -E "aitum-multistream|multistream" | awk '{print $2}' | head -1)
    if [ -n "$MULTISTREAM_PKG" ]; then
        sudo dpkg --purge "$MULTISTREAM_PKG"
        echo "✓ Removed $MULTISTREAM_PKG"
    else
        echo "  aitum-multistream not found"
    fi
    
    # Check for vertical-canvas package (could be vertical-canvas or obs-vertical-canvas)
    VERTICAL_PKG=$(dpkg -l 2>/dev/null | grep -E "vertical-canvas" | awk '{print $2}' | head -1)
    if [ -n "$VERTICAL_PKG" ]; then
        sudo dpkg --purge "$VERTICAL_PKG"
        echo "✓ Removed $VERTICAL_PKG"
    else
        echo "  vertical-canvas not found"
    fi
    
    echo ""
    echo "Aitum plugins uninstalled."
    restart_obs
}

update_aitum_plugins() {
    echo "Updating Aitum OBS Plugins to latest versions..."
    echo "These plugins are licensed under GPL-2.0"
    echo ""
    
    # Get latest release tags from GitHub API
    LATEST_MULTISTREAM=$(curl -s https://api.github.com/repos/Aitum/obs-aitum-multistream/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    LATEST_VERTICAL=$(curl -s https://api.github.com/repos/Aitum/obs-vertical-canvas/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$LATEST_MULTISTREAM" ] || [ -z "$LATEST_VERTICAL" ]; then
        echo "Error: Could not fetch latest versions from GitHub API"
        exit 1
    fi
    
    echo "Latest versions found:"
    echo "  - obs-aitum-multistream: $LATEST_MULTISTREAM"
    echo "  - obs-vertical-canvas: $LATEST_VERTICAL"
    echo ""
    
    # Create temp directory
    TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"
    
    # Install obs-aitum-multistream
    echo "Downloading obs-aitum-multistream $LATEST_MULTISTREAM..."
    curl -L -o multistream.deb \
        "https://github.com/Aitum/obs-aitum-multistream/releases/download/${LATEST_MULTISTREAM}/aitum-multistream-linux-gnu.deb"
    
    echo "Installing obs-aitum-multistream..."
    sudo dpkg -i multistream.deb || sudo apt-get install -f -y
    
    # Install obs-vertical-canvas
    echo "Downloading obs-vertical-canvas $LATEST_VERTICAL..."
    curl -L -o vertical.deb \
        "https://github.com/Aitum/obs-vertical-canvas/releases/download/${LATEST_VERTICAL}/vertical-canvas-linux-gnu.deb"
    
    echo "Installing obs-vertical-canvas..."
    sudo dpkg -i vertical.deb || sudo apt-get install -f -y
    
    # Cleanup
    cd - > /dev/null
    rm -rf "$TEMP_DIR"
    
    echo ""
    echo "✓ Aitum plugins updated successfully!"
    echo "  - obs-aitum-multistream $LATEST_MULTISTREAM"
    echo "  - obs-vertical-canvas $LATEST_VERTICAL"
    restart_obs
}

# Handle Aitum plugin action flags first (these exit after running)
for arg in "$@"; do
    case "$arg" in
        -installaitum)
            install_aitum_plugins
            exit 0
            ;;
        -uninstallaitum)
            uninstall_aitum_plugins
            exit 0
            ;;
        -updateaitum)
            update_aitum_plugins
            exit 0
            ;;
    esac
done

# ===========================================
# Interactive Configuration System
# ===========================================
CONFIG_FILE="/etc/stable-stream/config.env"
FIRST_RUN=false
RUN_CONFIG=false

# Check if this is first run
if [ ! -f "$CONFIG_FILE" ]; then
    FIRST_RUN=true
    RUN_CONFIG=true
    sudo mkdir -p /etc/stable-stream
    
    # Pre-seed default credentials for first run
    # This allows immediate login without running configuration
    echo "Pre-seeding default credentials (admin/password)..."
    DEFAULT_HASH=$(python3 -c "import bcrypt; print(bcrypt.hashpw(b'password', bcrypt.gensalt()).decode())")
    if [ -n "$DEFAULT_HASH" ]; then
        # Save to config file
        sudo tee "$CONFIG_FILE" > /dev/null <<EOF
USERNAME="admin"
PASSWORD_HASH="$DEFAULT_HASH"
MULTISTREAM_ENABLED="y"
MAX_MULTISTREAMS="5"
INGESTS_LOCKED="n"
EOF
        # Sync to database using Python (safe from bash escaping issues)
        python3 -c "
import sqlite3
import sys
conn = sqlite3.connect('data.db')
c = conn.cursor()
c.execute('CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT UNIQUE, password_hash TEXT NOT NULL)')
c.execute('DELETE FROM users')
c.execute('INSERT INTO users (username, password_hash) VALUES (?, ?)', ('admin', '''$DEFAULT_HASH'''))
conn.commit()
conn.close()
print('✓ Default credentials set (admin/password)')
" 2>/dev/null || echo "Warning: Could not pre-seed database"
    fi
fi

# Prompt helper functions
prompt_input() {
    local prompt="$1"
    local default="$2"
    local varname="$3"
    read -p "$prompt [$default]: " input
    eval "$varname=\"\${input:-$default}\""
}

prompt_password() {
    local prompt="$1"
    local varname="$2"
    read -sp "$prompt: " input
    echo ""
    eval "$varname=\"$input\""
}

prompt_yn() {
    local prompt="$1"
    local default="$2"
    local result
    read -p "$prompt (y/n) [$default]: " input
    input="${input:-$default}"
    if [[ "$input" =~ ^[Yy] ]]; then
        echo "y"
    else
        echo "n"
    fi
}

# Hash password using bcrypt via Python (more secure than SHA-256)
hash_password() {
    local password="$1"
    python3 -c "import bcrypt; print(bcrypt.hashpw('$password'.encode(), bcrypt.gensalt()).decode())" 2>/dev/null
}

echo ""
echo "=============================================="
echo "  STABLE STREAM SOLUTIONS"
echo "=============================================="

# On subsequent runs, ask if they want to modify config
if [ "$FIRST_RUN" = false ]; then
    echo ""
    echo "Existing configuration found."
    MODIFY_SETUP=$(prompt_yn "Modify setup?" "n")
    if [ "$MODIFY_SETUP" = "y" ]; then
        RUN_CONFIG=true
    else
        echo "Using saved configuration..."
        source "$CONFIG_FILE"
        
        # Sync saved config to database (in case DB was reset or doesn't exist)
        # IMPORTANT: Use Python with proper escaping to handle bcrypt hashes containing $
        DB_PATH="data.db"
        if [ -n "$USERNAME" ] && [ -n "$PASSWORD_HASH" ]; then
            # Base64 encode the hash to safely pass it through bash
            ENCODED_HASH=$(echo -n "$PASSWORD_HASH" | base64)
            python3 -c "
import sqlite3
import base64
import sys

username = '$USERNAME'
encoded_hash = '$ENCODED_HASH'
password_hash = base64.b64decode(encoded_hash).decode('utf-8')

conn = sqlite3.connect('$DB_PATH')
c = conn.cursor()
c.execute('CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT UNIQUE, password_hash TEXT NOT NULL)')
c.execute('DELETE FROM users')
c.execute('INSERT INTO users (username, password_hash) VALUES (?, ?)', (username, password_hash))
conn.commit()
conn.close()
print('✓ Credentials synced to database')
" || echo "Warning: Failed to sync credentials to database"
        fi
        
        # Sync ingests_locked setting
        if [ -f "$DB_PATH" ]; then
            INGESTS_LOCK_VALUE="false"
            [ "${INGESTS_LOCKED:-n}" = "y" ] && INGESTS_LOCK_VALUE="true"
            python3 -c "
import sqlite3
conn = sqlite3.connect('$DB_PATH')
c = conn.cursor()
c.execute('CREATE TABLE IF NOT EXISTS config (key TEXT PRIMARY KEY, value TEXT)')
c.execute('INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)', ('ingests_locked', '$INGESTS_LOCK_VALUE'))
conn.commit()
conn.close()
" 2>/dev/null || true
        fi
    fi
fi

# Run interactive configuration if needed
if [ "$RUN_CONFIG" = true ]; then
    echo ""
    echo "--- Configuration ---"
    echo ""
    
    # Load existing values as defaults (if config exists)
    if [ -f "$CONFIG_FILE" ]; then
        source "$CONFIG_FILE"
    fi
    
    # Username setup (was previously email)
    if [ -n "${USERNAME:-}" ]; then
        echo "Current username: $USERNAME"
        CHANGE_USERNAME=$(prompt_yn "Change username?" "n")
        if [ "$CHANGE_USERNAME" = "y" ]; then
            prompt_input "New username" "" USERNAME
        fi
    else
        prompt_input "Username" "${USERNAME:-}" USERNAME
    fi
    
    # Password setup
    if [ -n "${PASSWORD_HASH:-}" ]; then
        echo "Password is already set."
        CHANGE_PASSWORD=$(prompt_yn "Change password?" "n")
        if [ "$CHANGE_PASSWORD" = "y" ]; then
            prompt_password "New password" PASSWORD
            if [ -n "$PASSWORD" ]; then
                echo "Hashing password..."
                PASSWORD_HASH=$(hash_password "$PASSWORD")
                unset PASSWORD
            fi
        fi
    else
        prompt_password "Set password" PASSWORD
        if [ -n "$PASSWORD" ]; then
            echo "Hashing password..."
            PASSWORD_HASH=$(hash_password "$PASSWORD")
            unset PASSWORD
        fi
    fi
    
    # Multi-stream configuration
    DEFAULT_MS="n"
    [ "${MULTISTREAM_ENABLED:-n}" = "y" ] && DEFAULT_MS="y"
    MULTISTREAM_ENABLED=$(prompt_yn "Enable Multi-Streaming?" "$DEFAULT_MS")
    
    if [ "$MULTISTREAM_ENABLED" = "y" ]; then
        read -p "Maximum stream destinations (1-5) [${MAX_MULTISTREAMS:-3}]: " input
        MAX_MULTISTREAMS="${input:-${MAX_MULTISTREAMS:-3}}"
        # Clamp to 1-5
        if [ "$MAX_MULTISTREAMS" -lt 1 ] 2>/dev/null; then MAX_MULTISTREAMS=1; fi
        if [ "$MAX_MULTISTREAMS" -gt 5 ] 2>/dev/null; then MAX_MULTISTREAMS=5; fi
    else
        MAX_MULTISTREAMS=0
    fi
    
    # Ingest lock configuration
    echo ""
    echo "--- Ingest Management ---"
    DEFAULT_INGESTS_LOCKED="n"
    [ "${INGESTS_LOCKED:-n}" = "y" ] && DEFAULT_INGESTS_LOCKED="y"
    INGESTS_LOCKED=$(prompt_yn "Lock ingest creation? (Prevents adding new ingests)" "$DEFAULT_INGESTS_LOCKED")
    
    # Save configuration
    sudo tee "$CONFIG_FILE" > /dev/null <<EOF
USERNAME="$USERNAME"
PASSWORD_HASH="$PASSWORD_HASH"
MULTISTREAM_ENABLED="$MULTISTREAM_ENABLED"
MAX_MULTISTREAMS="$MAX_MULTISTREAMS"
INGESTS_LOCKED="$INGESTS_LOCKED"
EOF
    
    echo ""
    echo "✓ Configuration saved!"
    
    # Update/Create server's local SQLite database with the username/password
    DB_PATH="data.db"
    if [ -n "$USERNAME" ] && [ -n "$PASSWORD_HASH" ]; then
        echo "Setting up user database..."
        python3 -c "
import sqlite3
conn = sqlite3.connect('$DB_PATH')
c = conn.cursor()
c.execute('CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT UNIQUE, password_hash TEXT NOT NULL)')
c.execute('DELETE FROM users')
c.execute('INSERT INTO users (username, password_hash) VALUES (?, ?)', ('$USERNAME', '$PASSWORD_HASH'))
conn.commit()
conn.close()
print('✓ User credentials saved to database')
" || echo "Error: Failed to save credentials"
    fi
    
    # Sync ingests_locked setting to database
    INGESTS_LOCK_VALUE="false"
    [ "$INGESTS_LOCKED" = "y" ] && INGESTS_LOCK_VALUE="true"
    
    python3 -c "
import sqlite3
conn = sqlite3.connect('$DB_PATH')
c = conn.cursor()
c.execute('CREATE TABLE IF NOT EXISTS config (key TEXT PRIMARY KEY, value TEXT)')
c.execute('INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)', ('ingests_locked', '$INGESTS_LOCK_VALUE'))
conn.commit()
conn.close()
" 2>/dev/null || true
    echo "✓ Ingest lock setting synced to database: $INGESTS_LOCK_VALUE"
fi

echo ""

# ===========================================
# Fix Multistream Permissions (if enabled)
# ===========================================
if [ "${MULTISTREAM_ENABLED:-n}" = "y" ]; then
    echo "Fixing multistream nginx permissions..."
    # Ensure rtmp.conf is writable by backend
    if [ -f /etc/nginx/rtmp.conf ]; then
        sudo chown $USER:$USER /etc/nginx/rtmp.conf 2>/dev/null || true
    fi
    # Ensure rtmp.d directory and destinations.conf exist and are writable
    sudo mkdir -p /etc/nginx/rtmp.d 2>/dev/null || true
    sudo touch /etc/nginx/rtmp.d/destinations.conf 2>/dev/null || true
    sudo chown -R $USER:$USER /etc/nginx/rtmp.d 2>/dev/null || true
    # Ensure ffmpeg log file exists and is writable
    sudo touch /var/log/nginx/ffmpeg_push.log 2>/dev/null || true
    sudo chown $USER:$USER /var/log/nginx/ffmpeg_push.log 2>/dev/null || true
    echo "✓ Multistream permissions configured"
fi

# Ensure scripts are executable
chmod +x scripts/install_dependencies.sh

# Security: Ensure Public Key exists for SSO verification
# It is safe to embed the PUBLIC key (it acts like a lock, not a key)
if [ ! -f "public.pem" ]; then
    echo "Creating public.pem for SSO verification..."
    cat <<EOF > public.pem
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAwxoAQtHoMvrsyiyN4p6l
Xz6W62TkLdKGUV0xV8OXiI8FULHPIXYMr/Rs8GjihRPqSYEVqtVOlIr1nVkhKYm5
tFo+q8piP/nJJ79DZm9do+1uMtbLJDtSZh+QQiVUSh0gkaFoL7hDMpNMXMb8pUcm
bMS6CnxinYaqSz6CL9fKBJJ55ALu5grQ/8QYgZugh/08z8u6IaopWhMcp3CQfzO5
MN+rA7TmPTWTwFWi0eAfiAu9lMsiRmW6Az1ckBlD+sv//Qel0GSOE6Rmbul0PE6i
CODSW/Xv6nIE97bMaBQ52hg71RNlbtY4qiaz/sb06ZbjYmdw0/KnywVbt/6iqCES
yQIDAQAB
-----END PUBLIC KEY-----
EOF
fi

echo "Checking System Dependencies..."
./scripts/install_dependencies.sh

# Ensure Go is in PATH for this session (Linux especially)
if [ -d "/usr/local/go/bin" ]; then
    export PATH=$PATH:/usr/local/go/bin
fi

echo "Building Backend..."
cd backend
export GO111MODULE=on

# Debug Info
echo "Go Version:"
go version
echo "Go Env:"
go env

go mod tidy
go build -o ../bin/server main.go
cd ..

echo "Building Frontend..."
cd frontend
npm install
npm run build
cd ..

echo "Starting Server..."
# Ensure bin directory exists for data.db
mkdir -p bin

# Build server arguments
SERVER_ARGS=""
if [ -n "$JWT_SECRET" ]; then
    SERVER_ARGS="$SERVER_ARGS -jwt $JWT_SECRET"
fi
if [ -n "$JWT_PUBLIC_KEY" ]; then
    SERVER_ARGS="$SERVER_ARGS -public-key $JWT_PUBLIC_KEY"
fi
if [ "$MULTISTREAM_ENABLED" = "y" ]; then
    SERVER_ARGS="$SERVER_ARGS -multistream -max-streams $MAX_MULTISTREAMS"
    echo "🔀 Multi-streaming enabled (max $MAX_MULTISTREAMS destinations)"
fi

# Add ingest lock flag if enabled
if [ "$INGESTS_LOCKED" = "y" ]; then
    SERVER_ARGS="$SERVER_ARGS -ingests-locked"
    echo "🔒 Ingest creation is LOCKED"
fi

# Start server with arguments
eval ./bin/server $SERVER_ARGS


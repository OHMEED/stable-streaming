#!/bin/bash

# Ensure bin directory exists
mkdir -p bin

# 1. Download MediaMTX (v1.9.3 is a recent stable version example, check for latest)
# Using v1.9.3 for stability.
echo "Downloading MediaMTX..."
# Detect OS/Arch for MediaMTX
OS="$(uname -s)"
ARCH="$(uname -m)"

MMTX_VER="v1.9.3"
MMTX_ASSET=""

if [ "$OS" == "Darwin" ]; then
    if [ "$ARCH" == "arm64" ]; then
        MMTX_ASSET="mediamtx_${MMTX_VER}_darwin_arm64.tar.gz"
    else
        MMTX_ASSET="mediamtx_${MMTX_VER}_darwin_amd64.tar.gz"
    fi
elif [ "$OS" == "Linux" ]; then
    if [ "$ARCH" == "aarch64" ]; then
        MMTX_ASSET="mediamtx_${MMTX_VER}_linux_arm64v8.tar.gz"
    else
        MMTX_ASSET="mediamtx_${MMTX_VER}_linux_amd64.tar.gz"
    fi
fi

if [ -z "$MMTX_ASSET" ]; then
    echo "Unsupported OS/Arch ($OS/$ARCH) for auto-downloading MediaMTX."
else
    curl -L "https://github.com/bluenviron/mediamtx/releases/download/${MMTX_VER}/${MMTX_ASSET}" -o "bin/mediamtx.tar.gz"
    tar -xzf bin/mediamtx.tar.gz -C bin/
    rm bin/mediamtx.tar.gz
    echo "MediaMTX installed to ./bin/mediamtx"
fi

# 2. Install go-irl
echo "Installing go-irl..."

if command -v go >/dev/null 2>&1; then
    GIT_TMP=$(mktemp -d)
    git clone https://github.com/e04/go-irl.git "$GIT_TMP"
    
    current_dir=$(pwd)
    cd "$GIT_TMP"
    
    # go-irl embeds its frontend, so we need to build it first
    if [ -d "frontend" ] && [ -f "frontend/package.json" ]; then
        echo "Building go-irl frontend..."
        cd frontend
        if command -v npm >/dev/null 2>&1; then
            npm install --silent 2>/dev/null
            npm run build --silent 2>/dev/null
        else
            echo "npm not found, creating stub frontend/dist..."
            # Create stub files so Go embed works
            mkdir -p dist
            echo "<!DOCTYPE html><html><body>go-irl</body></html>" > dist/index.html
        fi
        cd ..
    fi
    
    # Now build go-irl
    echo "Compiling go-irl..."
    go build -o "$current_dir/bin/go-irl" .
    
    cd "$current_dir"
    rm -rf "$GIT_TMP"
    
    if [ -f "bin/go-irl" ]; then
        echo "go-irl installed to ./bin/go-irl"
    else
        echo "Failed to install go-irl. Please install manually."
    fi
else
    echo "Go is not installed. Cannot build go-irl."
fi

chmod +x bin/mediamtx
chmod +x bin/go-irl
echo "Done."

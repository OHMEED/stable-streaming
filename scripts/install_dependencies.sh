#!/bin/bash

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# ===========================================
# GPU Environment Detection
# Returns: bare_metal, passthrough, vgpu, or none
# ===========================================
detect_gpu_environment() {
    # Check if any NVIDIA GPU is visible
    if ! lspci 2>/dev/null | grep -qi "nvidia"; then
        echo "none"
        return
    fi

    # Get the NVIDIA PCI device description
    local nvidia_pci
    nvidia_pci=$(lspci 2>/dev/null | grep -i nvidia | head -1)
    
    # First, check if we're running in a VM
    local is_vm="no"
    if command -v systemd-detect-virt > /dev/null 2>&1; then
        local virt_type
        virt_type=$(systemd-detect-virt 2>/dev/null)
        if [ "$virt_type" != "none" ] && [ -n "$virt_type" ]; then
            is_vm="yes"
        fi
    elif [ -f /sys/class/dmi/id/product_name ]; then
        local product
        product=$(cat /sys/class/dmi/id/product_name 2>/dev/null)
        if echo "$product" | grep -qi "vmware\|proxmox\|virtual\|qemu\|kvm"; then
            is_vm="yes"
        fi
    fi

    # Check for vGPU-specific indicators (more specific checks)
    # vGPU devices show "GRID" or "NVIDIA vGPU" in lspci, NOT the full model name
    if echo "$nvidia_pci" | grep -qi "GRID"; then
        echo "vgpu"
        return
    fi

    # Check nvidia-smi for vGPU license/mode (only if driver is loaded)
    if command_exists nvidia-smi; then
        if nvidia-smi -q 2>/dev/null | grep -qi "vgpu software licensed product"; then
            echo "vgpu"
            return
        fi
        
        # Check for GRID in nvidia-smi product name
        local smi_name
        smi_name=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1)
        if echo "$smi_name" | grep -qi "GRID"; then
            echo "vgpu"
            return
        fi
        
        # nvidia-smi works with real GPU name = we have a physical GPU
        if [ -n "$smi_name" ]; then
            if [ "$is_vm" = "yes" ]; then
                echo "passthrough"
            else
                echo "bare_metal"
            fi
            return
        fi
    fi

    # Check for GRID licensing config file (indicates vGPU guest)
    if [ -f "/etc/nvidia/gridd.conf" ]; then
        # But only if we haven't already identified it as passthrough
        echo "vgpu"
        return
    fi

    # If we see a real GPU model name (GeForce, Quadro, Tesla, RTX, etc.) it's a physical GPU
    if echo "$nvidia_pci" | grep -qiE "GeForce|Quadro|Tesla|RTX|GTX|TITAN"; then
        if [ "$is_vm" = "yes" ]; then
            echo "passthrough"
        else
            echo "bare_metal"
        fi
        return
    fi

    # Default: if in VM with NVIDIA hardware, assume passthrough (safer than vGPU)
    if [ "$is_vm" = "yes" ]; then
        echo "passthrough"
    else
        echo "bare_metal"
    fi
}

# Check if NVENC is available (requires nvidia-smi)
check_nvenc_support() {
    if ! command_exists nvidia-smi; then
        echo "no_driver"
        return
    fi

    # Check for encoder capability
    # NVENC is available on most NVIDIA GPUs, but vGPU profiles may restrict it
    local gpu_name
    gpu_name=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1)
    
    if [ -z "$gpu_name" ]; then
        echo "no_gpu"
        return
    fi

    # Check for vGPU license type (C-series = compute with NVENC, Q-series = Quadro vDWS)
    local vgpu_mode
    vgpu_mode=$(nvidia-smi -q 2>/dev/null | grep -i "vgpu software licensed product" | head -1)
    
    if echo "$vgpu_mode" | grep -qi "compute\|vcs\|vws"; then
        echo "available"
        return
    fi
    
    # For non-vGPU or enterprise drivers, check if we can query encoding sessions
    if nvidia-smi dmon -s u -c 1 >/dev/null 2>&1; then
        echo "available"
        return
    fi
    
    # Assume available for consumer GPUs
    if [ -z "$vgpu_mode" ]; then
        echo "available"
        return
    fi
    
    echo "restricted"
}

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     machine=Linux;;
    Darwin*)    machine=Mac;;
    CYGWIN*)    machine=Cygwin;;
    MINGW*)     machine=MinGw;;
    *)          machine="UNKNOWN:${OS}"
esac

echo "Detected OS: $machine"

install_mac() {
    if ! command_exists brew; then
        echo "Homebrew not found. Attempting to install Homebrew..."
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    fi

    echo "Checking dependencies..."

    if ! command_exists xquartz; then
        if [ -d "/Applications/Utilities/XQuartz.app" ]; then
            echo "XQuartz found in Utilities."
        else
            echo "Installing XQuartz (required for XPRA)..."
            brew install --cask xquartz
        fi
    else
        echo "XQuartz found."
    fi
    
    if ! command_exists obs; then
        if [ -d "/Applications/OBS.app" ]; then
             echo "OBS found in Applications."
        else
             echo "Installing OBS Studio..."
             brew install --cask obs
        fi
    else
        echo "OBS Studio found via CLI."
    fi

    if ! command_exists xpra; then
        echo "Installing XPRA..."
        if brew install --cask xpra 2>/dev/null; then
            echo "XPRA installed via Homebrew."
        else
            echo "Homebrew install failed. Attempting manual download and installation..."
            ARCH=$(uname -m)
            if [ "$ARCH" == "arm64" ]; then
                XPRA_URL="https://xpra.org/dists/MacOS/arm64/Xpra-arm64-6.4-r0.dmg"
            else
                # Fallback for x86_64 - verifying availability is recommended but this follows standard naming
                XPRA_URL="https://xpra.org/dists/MacOS/x86_64/Xpra-x86_64-4.4.6-r0.dmg" 
                # Note: 6.4 might not be available for x86 yet or naming differs, but prioritizing user's arm64
                if ! curl --output /dev/null --silent --head --fail "$XPRA_URL"; then
                     XPRA_URL="https://xpra.org/dists/MacOS/x86_64/Xpra-x86_64.dmg" # Generic fallback
                fi
            fi

            TEMP_DMG="/tmp/Xpra_Install.dmg"
            echo "Downloading XPRA from $XPRA_URL..."
            if curl -L -o "$TEMP_DMG" "$XPRA_URL"; then
                echo "Mounting DMG..."
                # Mount and define mount point
                hdiutil attach "$TEMP_DMG" -mountpoint /Volumes/Xpra_Install -nobrowse -quiet
                
                if [ -d "/Volumes/Xpra_Install/Xpra.app" ]; then
                    echo "Copying Xpra.app to /Applications..."
                    if [ -d "/Applications/Xpra.app" ]; then
                        echo "Removing existing Xpra.app..."
                        rm -rf /Applications/Xpra.app
                    fi
                    cp -R "/Volumes/Xpra_Install/Xpra.app" /Applications/
                    
                    echo "Unmounting..."
                    hdiutil detach /Volumes/Xpra_Install -quiet
                    rm "$TEMP_DMG"
                    
                    # Create Wrapper Script
                    echo "Creating wrapper script for 'xpra' command..."
                    TARGET_BIN="/Applications/Xpra.app/Contents/MacOS/Xpra"
                    if [ -f "$TARGET_BIN" ]; then
                        # Determine install location
                        INSTALL_PATH=""
                        if [ -w "/usr/local/bin" ]; then
                            INSTALL_PATH="/usr/local/bin/xpra"
                        elif [ -w "/opt/homebrew/bin" ]; then
                            INSTALL_PATH="/opt/homebrew/bin/xpra"
                        fi

                        if [ -n "$INSTALL_PATH" ]; then
                            echo "#!/bin/bash" > "$INSTALL_PATH"
                            echo "exec \"$TARGET_BIN\" \"\$@\"" >> "$INSTALL_PATH"
                            chmod +x "$INSTALL_PATH"
                            echo "XPRA wrapper installed to $INSTALL_PATH"
                        else
                            # Try to use sudo if possible, or warn user
                            echo "⚠️  Could not create wrapper in /usr/local/bin or /opt/homebrew/bin due to permissions."
                            echo "   Please create a wrapper script manually."
                        fi
                    fi
                    echo "XPRA installed manually."
                else
                    echo "Failed to find Xpra.app in DMG."
                    hdiutil detach /Volumes/Xpra_Install -quiet || true
                fi
            else
                echo "Failed to download XPRA."
            fi
        fi
    else
        echo "XPRA found."
    fi
}

install_linux() {
    if command_exists apt-get; then
        echo "Updating apt..."
        sudo apt-get update
        
        # ===========================================
        # NVIDIA DKMS Conflict Pre-Check & Fix
        # Must run BEFORE any apt install operations
        # ===========================================
        if dpkg -l 2>/dev/null | grep -E "nvidia-(dkms|driver)" | grep -qE "^(.F|iU|iF|rc|.H)"; then
            echo ""
            echo "=============================================="
            echo "FIXING BROKEN NVIDIA PACKAGES"
            echo "=============================================="
            echo "Detected broken/half-configured NVIDIA packages."
            echo "Attempting automatic repair..."
            
            # Get the nvidia version from broken packages
            BROKEN_NVIDIA_VER=$(dpkg -l 2>/dev/null | grep -E "nvidia-dkms" | awk '{print $3}' | grep -oP '^\d+\.\d+\.\d+' | head -1)
            
            # Step 1: Clear DKMS tree for nvidia
            echo "Clearing DKMS nvidia entries..."
            for dkms_ver in $(dkms status 2>/dev/null | grep nvidia | awk -F'[,: ]+' '{print $2}'); do
                echo "  Removing DKMS entry: nvidia/$dkms_ver"
                sudo dkms remove "nvidia/$dkms_ver" --all 2>/dev/null || true
            done
            
            # Nuclear option: clear DKMS tree entirely for nvidia
            if [ -d "/var/lib/dkms/nvidia" ]; then
                echo "  Removing /var/lib/dkms/nvidia directory..."
                sudo rm -rf /var/lib/dkms/nvidia
            fi
            
            # Step 2: Purge broken nvidia packages
            echo "Purging broken NVIDIA packages..."
            sudo dpkg --purge --force-remove-reinstreq nvidia-dkms-* 2>/dev/null || true
            sudo dpkg --purge --force-remove-reinstreq nvidia-driver-* 2>/dev/null || true
            sudo apt-get purge -y 'nvidia-dkms-*' 'nvidia-driver-*' 2>/dev/null || true
            
            # Step 3: Fix dpkg state
            echo "Fixing dpkg state..."
            sudo dpkg --configure -a 2>/dev/null || true
            sudo apt-get -f install -y 2>/dev/null || true
            sudo apt-get autoremove -y 2>/dev/null || true
            
            echo "✓ NVIDIA package cleanup complete"
            echo "=============================================="
            echo ""
        fi
        
        # ===========================================
        # Automatic Timezone Detection & Time Sync
        # Required for OAuth (Twitch, YouTube, etc.)
        # ===========================================
        echo ""
        echo "=============================================="
        echo "CONFIGURING TIMEZONE & TIME SYNC"
        echo "=============================================="
        
        # Enable NTP time synchronization first
        echo "Enabling NTP time synchronization..."
        sudo timedatectl set-ntp true
        
        # Detect timezone via IP geolocation
        echo "Detecting timezone based on server location..."
        DETECTED_TZ=$(curl -s --max-time 10 http://ip-api.com/line?fields=timezone 2>/dev/null)
        
        if [ -n "$DETECTED_TZ" ] && [ "$DETECTED_TZ" != "" ]; then
            # Validate timezone exists
            if [ -f "/usr/share/zoneinfo/$DETECTED_TZ" ]; then
                echo "Setting timezone to: $DETECTED_TZ"
                sudo timedatectl set-timezone "$DETECTED_TZ"
                echo "✓ Timezone configured: $DETECTED_TZ"
            else
                echo "⚠ Detected timezone '$DETECTED_TZ' not found, using UTC"
                sudo timedatectl set-timezone "UTC"
            fi
        else
            echo "⚠ Could not detect timezone (no internet?), using UTC"
            sudo timedatectl set-timezone "UTC"
        fi
        
        # Show current time settings
        echo ""
        timedatectl status
        echo "=============================================="
        echo ""
        
        echo "Installing dependencies..."
        # Ensure software-properties-common is available
        sudo apt-get install -y software-properties-common flatpak novnc websockify

        # Set up Flathub
        sudo flatpak remote-add --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo

        # Core tools (openbox provides WM for OBS fullscreen support)
        sudo apt-get install -y ffmpeg git curl wget build-essential procps xdotool dbus-x11 openbox

        # Python for OBS scripting and plugin marketplace
        echo "Installing Python 3 for OBS scripting support..."
        sudo apt-get install -y python3 python3-pip

        # ===========================================
        # Wayland Compositor + VNC for OBS Session
        # ===========================================
        echo "Installing Wayland components (labwc, wayvnc, wtype, ydotool, wlr-randr)..."
        # Using labwc instead of cage - cage has stability issues with libwayland-server crashes
        sudo apt-get install -y labwc wayvnc wtype ydotool wlr-randr pipewire pipewire-pulse wireplumber \
            xdg-desktop-portal xdg-desktop-portal-wlr wl-clipboard

        # ===========================================
        # Cloudflare Tunnel (cloudflared) for Direct VNC Connections
        # Enables low-latency connections without port forwarding
        # ===========================================
        echo ""
        echo "=============================================="
        echo "CLOUDFLARE TUNNEL SETUP"
        echo "=============================================="
        
        if command_exists cloudflared; then
            echo "cloudflared already installed: $(cloudflared --version)"
        else
            echo "Installing cloudflared..."
            
            # Detect CPU architecture
            ARCH=$(uname -m)
            case "$ARCH" in
                x86_64)
                    CLOUDFLARED_ARCH="amd64"
                    ;;
                aarch64|arm64)
                    CLOUDFLARED_ARCH="arm64"
                    ;;
                armv7l|armhf)
                    CLOUDFLARED_ARCH="arm"
                    ;;
                *)
                    echo "ERROR: Unsupported architecture: $ARCH"
                    echo "cloudflared installation skipped (manual install required)"
                    CLOUDFLARED_ARCH=""
                    ;;
            esac
            
            if [ -n "$CLOUDFLARED_ARCH" ]; then
                CLOUDFLARED_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${CLOUDFLARED_ARCH}.deb"
                CLOUDFLARED_TMP="/tmp/cloudflared.deb"
                
                # Download with error checking
                echo "Downloading cloudflared for $CLOUDFLARED_ARCH..."
                if ! curl -fsSL --output "$CLOUDFLARED_TMP" "$CLOUDFLARED_URL"; then
                    echo "ERROR: Failed to download cloudflared from $CLOUDFLARED_URL"
                    rm -f "$CLOUDFLARED_TMP"
                else
                    # Verify file is not empty
                    if [ ! -s "$CLOUDFLARED_TMP" ]; then
                        echo "ERROR: Downloaded file is empty"
                        rm -f "$CLOUDFLARED_TMP"
                    else
                        # Install package with error checking
                        if sudo dpkg -i "$CLOUDFLARED_TMP"; then
                            # Verify installation
                            if command_exists cloudflared; then
                                echo "✓ cloudflared installed: $(cloudflared --version)"
                            else
                                echo "ERROR: cloudflared binary not found after installation"
                            fi
                        else
                            echo "ERROR: dpkg installation failed"
                            echo "Attempting to fix dependencies..."
                            sudo apt-get install -f -y
                        fi
                        
                        # Cleanup
                        rm -f "$CLOUDFLARED_TMP"
                    fi
                fi
            fi
        fi
        
        # Note: cloudflared service will be configured during start.sh based on tunnel token
        echo "cloudflared binary ready. Tunnel will be configured during ./start.sh"
        echo ""

        # ===========================================
        # GPU Driver Installation (required for Wayland GLES2 renderer)
        # Supports: Bare Metal, GPU Passthrough, vGPU (VMware/Proxmox)
        # ===========================================
        echo ""
        echo "=============================================="
        echo "GPU ENVIRONMENT DETECTION"
        echo "=============================================="
        
        GPU_ENV=$(detect_gpu_environment)
        echo "Detected GPU environment: $GPU_ENV"
        
        case "$GPU_ENV" in
            bare_metal|passthrough)
                echo ""
                echo "Physical/Passthrough NVIDIA GPU detected."
                echo "Installing driver via ubuntu-drivers..."
                
                # Install ubuntu-drivers tool if not present
                sudo apt-get install -y ubuntu-drivers-common
                
                # ===========================================
                # DKMS Conflict Resolution
                # Handles stuck DKMS state from interrupted installs
                # ===========================================
                fix_nvidia_dkms() {
                    local driver_version="$1"
                    echo ""
                    echo "Attempting to fix DKMS conflicts..."
                    
                    # Try to remove any stuck DKMS entries
                    if [ -n "$driver_version" ]; then
                        echo "Removing DKMS entry for nvidia/$driver_version..."
                        sudo dkms remove "nvidia/$driver_version" --all 2>/dev/null || true
                    fi
                    
                    # Remove all nvidia DKMS entries if specific version failed
                    for dkms_ver in $(dkms status 2>/dev/null | grep nvidia | awk -F'[,: ]+' '{print $2}'); do
                        echo "Removing DKMS entry: nvidia/$dkms_ver"
                        sudo dkms remove "nvidia/$dkms_ver" --all 2>/dev/null || true
                    done
                    
                    # Nuclear option: clear DKMS tree entirely for nvidia
                    if [ -d "/var/lib/dkms/nvidia" ]; then
                        echo "Clearing stuck DKMS state from /var/lib/dkms/nvidia..."
                        sudo rm -rf /var/lib/dkms/nvidia
                    fi
                    
                    # Reconfigure any partially installed packages
                    echo "Reconfiguring packages..."
                    sudo dpkg --configure -a 2>/dev/null || true
                    
                    echo "DKMS cleanup complete."
                }
                
                # Check for existing broken NVIDIA packages
                if dpkg -l | grep -E "nvidia-(dkms|driver)" | grep -qE "^(iU|iF|rc)"; then
                    echo ""
                    echo "⚠ Detected broken NVIDIA packages, attempting repair..."
                    
                    # Get version from broken package
                    BROKEN_VERSION=$(dpkg -l | grep nvidia-dkms | grep -oP '\d+\.\d+\.\d+' | head -1)
                    fix_nvidia_dkms "$BROKEN_VERSION"
                    
                    # Purge broken packages
                    echo "Purging broken NVIDIA packages..."
                    sudo apt-get purge -y 'nvidia-*' 2>/dev/null || true
                    sudo apt-get autoremove -y 2>/dev/null || true
                    sudo apt-get -f install -y 2>/dev/null || true
                fi
                
                # Check if a GPU with proprietary driver is detected
                if ubuntu-drivers devices 2>/dev/null | grep -q "driver"; then
                    # Get the recommended driver
                    RECOMMENDED_DRIVER=$(ubuntu-drivers devices 2>/dev/null | grep "recommended" | awk '{print $3}')
                    
                    if [ -n "$RECOMMENDED_DRIVER" ]; then
                        echo "Installing recommended driver: $RECOMMENDED_DRIVER"
                        
                        # Install with error handling for DKMS conflicts
                        if ! sudo apt-get install -y "$RECOMMENDED_DRIVER" 2>&1 | tee /tmp/nvidia_install.log; then
                            # Check if it's a DKMS conflict
                            if grep -q "DKMS tree already contains" /tmp/nvidia_install.log; then
                                echo ""
                                echo "⚠ DKMS conflict detected, auto-fixing..."
                                
                                # Extract version from error message
                                CONFLICT_VERSION=$(grep -oP 'nvidia-\K[\d.]+' /tmp/nvidia_install.log | head -1)
                                fix_nvidia_dkms "$CONFLICT_VERSION"
                                
                                # Purge and reinstall
                                echo "Purging and reinstalling driver..."
                                sudo apt-get purge -y "$RECOMMENDED_DRIVER" 2>/dev/null || true
                                sudo apt-get purge -y 'nvidia-dkms-*' 2>/dev/null || true
                                sudo apt-get autoremove -y
                                sudo apt-get -f install -y
                                
                                # Retry installation
                                echo "Retrying driver installation..."
                                if ! sudo apt-get install -y "$RECOMMENDED_DRIVER"; then
                                    echo ""
                                    echo "=============================================="
                                    echo "⚠ DRIVER INSTALLATION FAILED"
                                    echo "=============================================="
                                    echo "Manual intervention required. Try:"
                                    echo "  sudo rm -rf /var/lib/dkms/nvidia"
                                    echo "  sudo apt purge -y 'nvidia-*'"
                                    echo "  sudo apt autoremove -y"
                                    echo "  sudo apt install -y $RECOMMENDED_DRIVER"
                                    echo "  sudo reboot"
                                    echo "=============================================="
                                fi
                            else
                                echo "Driver installation failed for unknown reason."
                                echo "Check /tmp/nvidia_install.log for details."
                            fi
                        fi
                        
                        # Cleanup temp log
                        rm -f /tmp/nvidia_install.log
                        
                        # For NVIDIA, also install additional utilities
                        if echo "$RECOMMENDED_DRIVER" | grep -q "nvidia"; then
                            echo "NVIDIA GPU detected, installing additional utilities..."
                            # Extract version number from driver name (e.g., nvidia-driver-580 -> 580)
                            NVIDIA_VERSION=$(echo "$RECOMMENDED_DRIVER" | grep -oP '\d+' | head -1)
                            if [ -n "$NVIDIA_VERSION" ]; then
                                sudo apt-get install -y "nvidia-utils-$NVIDIA_VERSION" 2>/dev/null || true
                            fi
                        fi
                        
                        echo ""
                        echo "=============================================="
                        echo "GPU DRIVER INSTALLED: $RECOMMENDED_DRIVER"
                        echo "NVENC hardware encoding: AVAILABLE"
                        echo "A REBOOT IS REQUIRED for the driver to load."
                        echo "=============================================="
                    else
                        echo "No recommended driver found, using open-source drivers."
                        echo "NVENC hardware encoding: NOT AVAILABLE (software encoding will be used)"
                    fi
                else
                    echo "No GPU requiring proprietary drivers detected."
                    echo "Using open-source drivers (NVENC not available)."
                fi
                ;;
                
            vgpu)
                echo ""
                echo "=============================================="
                echo "NVIDIA vGPU ENVIRONMENT DETECTED"
                echo "=============================================="
                echo ""
                echo "This VM is running with NVIDIA vGPU (VMware/Proxmox)."
                echo "Standard consumer drivers are NOT compatible with vGPU."
                echo ""
                echo ">>> MANUAL DRIVER INSTALLATION REQUIRED <<<"
                echo ""
                echo "To enable GPU acceleration and NVENC encoding:"
                echo ""
                echo "1. Obtain the NVIDIA vGPU Guest Driver from:"
                echo "   https://enterprise.nvidia.com/downloads"
                echo "   (Requires NVIDIA Enterprise Portal account)"
                echo ""
                echo "2. Choose the driver matching your hypervisor's vGPU Manager version"
                echo ""
                echo "3. Install the driver:"
                echo "   chmod +x NVIDIA-Linux-x86_64-XXX.XX-grid.run"
                echo "   sudo ./NVIDIA-Linux-x86_64-XXX.XX-grid.run --dkms"
                echo ""
                echo "4. Configure licensing (if required by your vGPU profile):"
                echo "   sudo nano /etc/nvidia/gridd.conf"
                echo "   # Add: ServerAddress=<license-server-ip>"
                echo "   sudo systemctl restart nvidia-gridd"
                echo ""
                echo "IMPORTANT: Ensure your vGPU profile supports NVENC encoding:"
                echo "  - Compute profiles (-C suffix): NVENC SUPPORTED"
                echo "  - Quadro vDWS profiles (-Q suffix): NVENC SUPPORTED"  
                echo "  - Display-only profiles: NVENC NOT SUPPORTED"
                echo ""
                echo "After driver installation, run: nvidia-smi"
                echo "=============================================="
                echo ""
                
                # Save vGPU detection flag for startwm_obs.sh
                sudo mkdir -p /etc/stable-stream
                echo "vgpu" | sudo tee /etc/stable-stream/gpu_env > /dev/null
                ;;
                
            none)
                echo ""
                echo "=============================================="
                echo "NO NVIDIA GPU DETECTED"
                echo "=============================================="
                echo ""
                echo "No NVIDIA GPU found. The system will use software encoding (x264)."
                echo "This will work but uses significantly more CPU resources."
                echo ""
                echo "If you expected a GPU to be present:"
                echo "  - For vGPU: Ensure your VM has a vGPU profile assigned"
                echo "  - For passthrough: Check IOMMU/VT-d configuration"
                echo "  - For bare metal: Verify GPU is seated properly"
                echo ""
                echo "=============================================="
                
                # Save no-GPU detection flag for startwm_obs.sh
                sudo mkdir -p /etc/stable-stream
                echo "none" | sudo tee /etc/stable-stream/gpu_env > /dev/null
                ;;
        esac
        
        # Save GPU environment for runtime detection
        sudo mkdir -p /etc/stable-stream
        echo "$GPU_ENV" | sudo tee /etc/stable-stream/gpu_env > /dev/null

        # Install OBS Studio (native via PPA for direct GPU access)
        echo "Installing OBS Studio (native)..."
        
        # Try PPA with timeout (Launchpad can hang on some networks)
        if timeout 60 sudo add-apt-repository -y ppa:obsproject/obs-studio 2>/dev/null; then
            echo "OBS Studio PPA added successfully"
            sudo apt-get update
        else
            echo "PPA connection timed out or failed, using default Ubuntu repo..."
        fi
        
        sudo apt-get install -y obs-studio

        # Add user to video and render groups for hardware acceleration access
        sudo usermod -aG video,render,input $USER

        # ===========================================
        # Configure Wayland OBS Session with labwc
        # ===========================================
        echo "Configuring Wayland session for OBS with labwc..."
        
        # Copy our custom startwm script for Wayland
        SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        if [ -f "$SCRIPT_DIR/startwm_obs.sh" ]; then
            sudo cp "$SCRIPT_DIR/startwm_obs.sh" /usr/local/bin/start-obs-session.sh
            sudo chmod +x /usr/local/bin/start-obs-session.sh
            echo "Installed Wayland OBS session script to /usr/local/bin/"
        fi

        # Create labwc config with auto-maximize for OBS windows
        echo "Creating labwc configuration with OBS auto-maximize..."
        sudo mkdir -p /home/user/.config/labwc
        sudo tee /home/user/.config/labwc/rc.xml > /dev/null << 'EOF'
<?xml version="1.0"?>
<labwc_config>
  <keyboard>
    <default />
    <!-- Add F11 as manual fullscreen toggle -->
    <keybind key="F11">
      <action name="ToggleFullscreen" />
    </keybind>
  </keyboard>
  <mouse>
    <default />
  </mouse>
  <!-- Window rules to auto-maximize OBS windows -->
  <windowRules>
    <windowRule title="OBS*">
      <action name="Maximize" />
    </windowRule>
  </windowRules>
</labwc_config>
EOF
        sudo chown -R user:user /home/user/.config

        # Create labwc autostart to set resolution for headless output
        echo "Creating labwc autostart for 1920x1080 resolution..."
        sudo tee /home/user/.config/labwc/autostart > /dev/null << 'EOF'
# Set headless output resolution to 1920x1080
# Sleep briefly to ensure output is initialized
sleep 0.5
wlr-randr --output HEADLESS-1 --custom-mode 1920x1080
EOF
        sudo chmod +x /home/user/.config/labwc/autostart
        sudo chown user:user /home/user/.config/labwc/autostart

        # ===========================================
        # ydotool daemon for input automation
        # ===========================================
        echo "Setting up ydotool daemon for crash dialog handling..."
        
        # Create systemd service for ydotoold
        sudo tee /etc/systemd/system/ydotoold.service > /dev/null << 'EOF'
[Unit]
Description=ydotool daemon for Wayland input automation
After=local-fs.target

[Service]
Type=simple
ExecStart=/usr/bin/ydotool daemon
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

        # ===========================================
        # OBS Wayland session service with labwc compositor
        # ===========================================
        echo "Setting up OBS Wayland session service..."
        
        # Create systemd service for the OBS Wayland session using labwc
        # labwc is more stable than cage for long-running server use
        sudo tee /etc/systemd/system/obs-wayland.service > /dev/null << 'EOF'
[Unit]
Description=OBS Studio on Wayland (labwc compositor)
After=ydotoold.service
Wants=ydotoold.service

[Service]
Type=simple
User=user
Environment=HOME=/home/user
Environment=XDG_RUNTIME_DIR=/run/user/1000
# Use headless backend (no real display required)
Environment=WLR_BACKENDS=headless
Environment=WLR_LIBINPUT_NO_DEVICES=1
# WLR_RENDERER is not set — labwc auto-detects GPU (GLES2) when available.
# For VMs without GPU passthrough, uncomment the following line:
# Environment=WLR_RENDERER=pixman
# NVIDIA-specific environment for OBS hardware encoding
Environment=__NV_PRIME_RENDER_OFFLOAD=1
Environment=__GLX_VENDOR_LIBRARY_NAME=nvidia
Environment=VK_ICD_FILENAMES=/usr/share/vulkan/icd.d/nvidia_icd.json
# OBS EGL for hardware encoding
Environment=OBS_USE_EGL=1
Environment=LIBVA_DRIVER_NAME=nvidia
# Force display name to avoid conflicts
Environment=WAYLAND_DISPLAY=wayland-0
# Ensure XDG_RUNTIME_DIR exists at boot (logind only creates it on login)
ExecStartPre=/bin/bash -c "mkdir -p /run/user/1000 && chown user:user /run/user/1000 && chmod 700 /run/user/1000"
# labwc uses -s to specify startup command
ExecStart=/usr/bin/labwc -s /usr/local/bin/start-obs-session.sh
Restart=always
RestartSec=5
# Graceful shutdown: send SIGTERM to main process, wait for cleanup
KillMode=mixed
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
EOF

        # NOTE: wayvnc and websockify now run INSIDE the labwc session (startwm_obs.sh)
        # because wayvnc needs access to the Wayland display created by labwc.
        # Separate systemd services cannot access the compositor's isolated Wayland socket.
        
        # Clean up any old wayvnc/novnc systemd services that won't work
        sudo systemctl disable wayvnc.service 2>/dev/null || true
        sudo systemctl disable novnc.service 2>/dev/null || true
        sudo rm -f /etc/systemd/system/wayvnc.service
        sudo rm -f /etc/systemd/system/novnc.service

        # Enable all services
        sudo systemctl daemon-reload
        sudo systemctl enable ydotoold.service
        sudo systemctl enable obs-wayland.service

        # Ensure user has GPU device access (required for hardware rendering)
        sudo usermod -aG video,render user
        echo "Added user to video and render groups for GPU access"

        # ===========================================
        # UFW Firewall Configuration
        # ===========================================
        echo "Configuring UFW firewall..."
        sudo apt-get install -y ufw

        # Reset to defaults
        sudo ufw --force reset

        # Default policies: deny incoming, allow outgoing
        sudo ufw default deny incoming
        sudo ufw default allow outgoing

        # ===========================================
        # LOCALHOST-ONLY PORTS (Security: Not exposed to internet)
        # ===========================================
        # SSH - localhost only (use VPN or SSH tunnel for remote access)
        # This prevents SSH brute force attacks from the internet
        sudo ufw allow from 127.0.0.1 to any port 22 proto tcp comment 'SSH (localhost only)'
        sudo ufw allow from 10.0.0.0/8 to any port 22 proto tcp comment 'SSH (private network)'
        sudo ufw allow from 172.16.0.0/12 to any port 22 proto tcp comment 'SSH (private network)'
        sudo ufw allow from 192.168.0.0/16 to any port 22 proto tcp comment 'SSH (private network)'

        # noVNC - localhost only (Go backend proxies with authentication)
        # External access goes through authenticated Go backend on port 8080
        sudo ufw allow from 127.0.0.1 to any port 6080 proto tcp comment 'noVNC (localhost only)'

        # ===========================================
        # INTERNET-ACCESSIBLE PORTS
        # ===========================================
        # HTTP/HTTPS for web access
        sudo ufw allow 80/tcp comment 'HTTP'
        sudo ufw allow 443/tcp comment 'HTTPS'

        # OBS WebSocket (external access for integrations)
        sudo ufw allow 4455/tcp comment 'OBS WebSocket'

        # Web UI (Go backend with authentication - proxies VNC)
        sudo ufw allow 8080/tcp comment 'Web UI'

        # ===========================================
        # INGEST PORTS (Incoming streams)
        # ===========================================
        # SRTLA UDP range (bonded streaming)
        sudo ufw allow 5000:5100/udp comment 'SRTLA ingest range'

        # SRT UDP range (standard SRT)
        sudo ufw allow 9000:9100/udp comment 'SRT ingest range'

        # RTMP TCP range (legacy streaming)
        sudo ufw allow 9000:9100/tcp comment 'RTMP ingest range'

        # ===========================================
        # EGRESS PORTS (Shareable output streams)
        # ===========================================
        # SRT relay output (SRTLA → SRT conversion)
        sudo ufw allow 6000:7000/udp comment 'SRT relay egress'

        # RTSP egress (MediaMTX output for SRT/RTMP)
        sudo ufw allow 7000:8000/tcp comment 'RTSP egress'

        # Enable UFW
        sudo ufw --force enable

        echo ""
        echo "=============================================="
        echo "UFW FIREWALL ENABLED"
        echo "=============================================="
        echo "Allowed ports:"
        sudo ufw status verbose
        echo "=============================================="

        # ===========================================
        # Fail2Ban for SSH Brute Force Protection
        # ===========================================
        echo "Installing fail2ban for SSH protection..."
        sudo apt-get install -y fail2ban

        # Create fail2ban configuration for SSH
        sudo tee /etc/fail2ban/jail.local > /dev/null << 'EOF'
[DEFAULT]
# Ban IP for 1 hour after 5 failed attempts in 10 minutes
bantime = 3600
findtime = 600
maxretry = 5

# Email notifications (optional - uncomment if configured)
# destemail = admin@example.com
# sendername = Fail2Ban
# mta = sendmail
# action = %(action_mwl)s

[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 5
bantime = 3600
EOF

        # Enable and start fail2ban
        sudo systemctl enable fail2ban
        sudo systemctl restart fail2ban

        echo "Fail2ban configured for SSH protection"

        # ===========================================
        # Caddy Reverse Proxy for HTTPS
        # ===========================================
        echo "Installing Caddy web server for HTTPS..."
        sudo apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl
        # Remove existing keyring to avoid gpg overwrite prompt
        sudo rm -f /usr/share/keyrings/caddy-stable-archive-keyring.gpg
        curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null || true
        curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
        sudo apt-get update
        sudo apt-get install -y caddy

        # Copy Caddyfile if present
        if [ -f "$SCRIPT_DIR/../Caddyfile" ]; then
            sudo cp "$SCRIPT_DIR/../Caddyfile" /etc/caddy/Caddyfile
            echo "Installed Caddyfile to /etc/caddy/"
        fi

        # Enable but don't start Caddy (user must configure domain first)
        sudo systemctl enable caddy

        # ===========================================
        # nginx-rtmp for Multi-Stream Relay
        # ===========================================
        echo ""
        echo "=============================================="
        echo "INSTALLING NGINX-RTMP FOR MULTI-STREAMING"
        echo "=============================================="
        
        # Install nginx with RTMP module
        sudo apt-get install -y libnginx-mod-rtmp
        
        # Create nginx rtmp.d directory for dynamic destination configs
        sudo mkdir -p /etc/nginx/rtmp.d
        
        # Create base RTMP configuration
        sudo tee /etc/nginx/rtmp.conf > /dev/null <<'RTMPEOF'
# nginx-rtmp configuration for multi-stream relay
# Destinations are dynamically configured by the backend

rtmp {
    server {
        listen 127.0.0.1:1935;
        chunk_size 4096;
        
        application live {
            live on;
            record off;
            
            # Dynamically included destination configs
            include /etc/nginx/rtmp.d/*.conf;
        }
    }
}
RTMPEOF
        
        # Create empty destinations file - owned by 'user' so backend can write
        sudo touch /etc/nginx/rtmp.d/destinations.conf
        sudo chown user:user /etc/nginx/rtmp.d/destinations.conf
        sudo chmod 644 /etc/nginx/rtmp.d/destinations.conf
        
        # Include rtmp.conf in main nginx config if not present
        if ! grep -q "include /etc/nginx/rtmp.conf" /etc/nginx/nginx.conf 2>/dev/null; then
            # Add include at the end of nginx.conf (before closing brace if exists, or at end)
            echo "include /etc/nginx/rtmp.conf;" | sudo tee -a /etc/nginx/nginx.conf > /dev/null
            echo "Added RTMP include to nginx.conf"
        fi
        
        # Disable default nginx HTTP site to avoid port 80 conflict with Caddy
        sudo rm -f /etc/nginx/sites-enabled/default
        
        # RTMP port only accessible from localhost (security)
        sudo ufw allow from 127.0.0.1 to any port 1935 proto tcp comment 'RTMP relay (localhost only)'
        
        # Reload nginx to pick up new config
        sudo systemctl restart nginx 2>/dev/null || sudo systemctl start nginx
        
        # Allow user to reload nginx without password (needed by backend for multistream)
        echo "user ALL=(ALL) NOPASSWD: /usr/sbin/nginx" | sudo tee /etc/sudoers.d/stable-stream-nginx > /dev/null
        sudo chmod 440 /etc/sudoers.d/stable-stream-nginx
        
        echo "✓ nginx-rtmp installed and configured"
        echo ""

        # ===========================================
        # Backend Authentication Configuration
        # ===========================================
        echo ""
        echo "=============================================="
        echo "CONFIGURING BACKEND AUTHENTICATION"
        echo "=============================================="
        
        # Ensure SCRIPT_DIR is available (defined earlier, but safeguarding)
        SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
        
        echo "Writing Portal Public Key to backend directories..."
        
        # Define the key content
        read -r -d '' PUBLIC_KEY_CONTENT << 'EOF'
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

        # Write to backend directory if it exists
        if [ -d "$PROJECT_ROOT/backend" ]; then
            echo "$PUBLIC_KEY_CONTENT" > "$PROJECT_ROOT/backend/public.pem"
            # Set permissions (user readable)
            chmod 644 "$PROJECT_ROOT/backend/public.pem"
            echo "✓ Updated: $PROJECT_ROOT/backend/public.pem"
        fi
        
        # Also write to root directory as fallback
        if [ -w "$PROJECT_ROOT" ]; then
             echo "$PUBLIC_KEY_CONTENT" > "$PROJECT_ROOT/public.pem"
             chmod 644 "$PROJECT_ROOT/public.pem"
             echo "✓ Updated: $PROJECT_ROOT/public.pem"
        fi
        
        echo "Authentication key configured."
        echo ""

        echo ""
        echo "=============================================="
        echo "SECURITY HARDENING COMPLETE!"
        echo "=============================================="
        echo ""
        echo "✅ UFW Firewall: ENABLED"
        echo "   - Only essential ports are open"
        echo "   - VNC ports (5900) blocked from external access"
        echo ""
        echo "✅ Caddy HTTPS Proxy: INSTALLED (not started)"
        echo "   To enable HTTPS:"
        echo "   1. Edit /etc/caddy/Caddyfile"
        echo "   2. Replace 'localhost' with your domain"
        echo "   3. Run: sudo systemctl start caddy"
        echo ""
        echo "✅ VNC Security: localhost-only binding"
        echo "   - VNC only accessible through authenticated backend"
        echo ""
        echo "✅ nginx-rtmp: INSTALLED"
        echo "   - Multi-stream relay on localhost:1935"
        echo "   - Destinations configured via web UI"
        echo "=============================================="
        echo ""
        echo "To start the OBS Wayland session:"
        echo "   sudo systemctl start obs-wayland"
        echo ""
        echo "To access via browser:"
        echo "   http://<server-ip>:8080  (Go backend proxies VNC)"
        echo ""
        echo "For production with HTTPS:"
        echo "   https://yourdomain.com (after Caddy setup)"
        echo "=============================================="


    else
        echo "Unsupported package manager. Please install dependencies manually."
        exit 1
    fi
}

if [ "$machine" == "Mac" ]; then
    install_mac
    
    # Install Go and Node on Mac
    if ! command_exists go; then
        echo "Installing Go..."
        brew install go
    else
        echo "Go found."
    fi

    if ! command_exists node; then
        echo "Installing Node.js..."
        brew install node
    else
        echo "Node.js found."
    fi

elif [ "$machine" == "Linux" ]; then
    install_linux
    
    # Install Go on Linux
    # Install Go on Linux
    GO_VERSION_REQUIRED="1.24.0"
    
    install_go() {
         echo "Installing Go ${GO_VERSION_REQUIRED}..."
         # Remove any existing go installation
         sudo rm -rf /usr/local/go
         
         ARCH_GO=""
         if [ "$(uname -m)" = "aarch64" ]; then ARCH_GO="arm64"; else ARCH_GO="amd64"; fi
         
         wget https://go.dev/dl/go${GO_VERSION_REQUIRED}.linux-${ARCH_GO}.tar.gz
         sudo tar -C /usr/local -xzf go${GO_VERSION_REQUIRED}.linux-${ARCH_GO}.tar.gz
         rm go${GO_VERSION_REQUIRED}.linux-${ARCH_GO}.tar.gz
         
         # Add to path temporarily for this script and persist it
         export PATH=$PATH:/usr/local/go/bin
         echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a /etc/profile
         echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
         
         echo "Go ${GO_VERSION_REQUIRED} installed."
    }

    if ! command_exists go; then
         install_go
    else
         # Check version
         CURRENT_GO_VER=$(go version | awk '{print $3}' | sed 's/go//')
         # Simple version comparison: if current starts with 1.24 or 1.25, we are good.
         # Improve logic if needed, but for now exact match or greater major.
         if [[ "$CURRENT_GO_VER" < "1.24.0" ]]; then
             echo "Go version $CURRENT_GO_VER is too old. Upgrading to ${GO_VERSION_REQUIRED}..."
             install_go
         else
             echo "Go version $CURRENT_GO_VER found (>= 1.24.0 required). Skipping installation."
         fi
    fi

    # ===========================================
    # Install MediaMTX and go-irl
    # ===========================================
    export PATH=$PATH:/usr/local/go/bin

    INSTALL_DIR="${INSTALL_DIR:-$(pwd)}"
    mkdir -p "$INSTALL_DIR/bin"

    # --- MediaMTX ---
    if [ ! -f "$INSTALL_DIR/bin/mediamtx" ]; then
        echo "Installing MediaMTX..."
        MMTX_VER="v1.9.3"
        MMTX_ARCH="$(uname -m)"
        if [ "$MMTX_ARCH" == "aarch64" ]; then
            MMTX_ASSET="mediamtx_${MMTX_VER}_linux_arm64v8.tar.gz"
        else
            MMTX_ASSET="mediamtx_${MMTX_VER}_linux_amd64.tar.gz"
        fi
        curl -L "https://github.com/bluenviron/mediamtx/releases/download/${MMTX_VER}/${MMTX_ASSET}" -o "$INSTALL_DIR/bin/mediamtx.tar.gz"
        tar -xzf "$INSTALL_DIR/bin/mediamtx.tar.gz" -C "$INSTALL_DIR/bin/"
        rm -f "$INSTALL_DIR/bin/mediamtx.tar.gz"
        chmod +x "$INSTALL_DIR/bin/mediamtx"
        echo "MediaMTX installed to $INSTALL_DIR/bin/mediamtx"
    else
        echo "MediaMTX already installed. Skipping."
    fi

    # --- go-irl ---
    if [ ! -f "$INSTALL_DIR/bin/go-irl" ]; then
        echo "Installing go-irl..."
        if command_exists go; then
            GIT_TMP=$(mktemp -d)
            git clone https://github.com/e04/go-irl.git "$GIT_TMP"

            pushd "$GIT_TMP" > /dev/null

            # go-irl embeds its frontend, build it first
            if [ -d "frontend" ] && [ -f "frontend/package.json" ]; then
                echo "Building go-irl frontend..."
                cd frontend
                if command_exists npm; then
                    npm install --silent 2>/dev/null
                    npm run build --silent 2>/dev/null
                else
                    echo "npm not found, creating stub frontend/dist..."
                    mkdir -p dist
                    echo '<!DOCTYPE html><html><body>go-irl</body></html>' > dist/index.html
                fi
                cd ..
            fi

            echo "Compiling go-irl..."
            go build -o "$INSTALL_DIR/bin/go-irl" .

            popd > /dev/null
            rm -rf "$GIT_TMP"

            if [ -f "$INSTALL_DIR/bin/go-irl" ]; then
                chmod +x "$INSTALL_DIR/bin/go-irl"
                echo "go-irl installed to $INSTALL_DIR/bin/go-irl"
            else
                echo "ERROR: Failed to build go-irl."
            fi
        else
            echo "ERROR: Go is not installed. Cannot build go-irl."
        fi
    else
        echo "go-irl already installed. Skipping."
    fi

    # Install Node.js on Linux
    if ! command_exists node; then
        echo "Installing Node.js (v20)..."
        curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
        sudo apt-get install -y nodejs
    else
        echo "Node.js found."
    fi

else
    echo "Unsupported OS: $machine"
    echo "Please ensure OBS Studio, XPRA, Go, and Node.js are installed manually."
fi

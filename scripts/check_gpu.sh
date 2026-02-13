#!/bin/bash
# ===========================================
# GPU Environment Diagnostic Script
# For Stable Stream Solutions
# ===========================================
# Run this script to verify your GPU setup and NVENC availability

echo ""
echo "=============================================="
echo "        STABLE STREAM - GPU DIAGNOSTIC"
echo "=============================================="
echo ""

# ===========================================
# System Information
# ===========================================
echo "[SYSTEM INFO]"
echo "  Hostname:     $(hostname)"
echo "  OS:           $(uname -s) $(uname -r)"

# Check if running in VM
VM_TYPE="Bare Metal"
if command -v systemd-detect-virt > /dev/null 2>&1; then
    VIRT=$(systemd-detect-virt 2>/dev/null)
    if [ "$VIRT" != "none" ] && [ -n "$VIRT" ]; then
        VM_TYPE="VM ($VIRT)"
    fi
elif [ -f /sys/class/dmi/id/product_name ]; then
    PRODUCT=$(cat /sys/class/dmi/id/product_name 2>/dev/null)
    if echo "$PRODUCT" | grep -qi "vmware\|proxmox\|virtual\|qemu\|kvm"; then
        VM_TYPE="VM ($PRODUCT)"
    fi
fi
echo "  Environment:  $VM_TYPE"
echo ""

# ===========================================
# GPU Detection
# ===========================================
echo "[GPU DETECTION]"

# Check for NVIDIA hardware via lspci
NVIDIA_PCI=$(lspci 2>/dev/null | grep -i nvidia)
if [ -n "$NVIDIA_PCI" ]; then
    echo "  PCI Device:   Found"
    echo "$NVIDIA_PCI" | while read line; do
        echo "                $line"
    done
    
    # Determine GPU type based on lspci output
    # vGPU shows "GRID" in name, passthrough shows real model names
    if echo "$NVIDIA_PCI" | grep -qi "GRID"; then
        GPU_TYPE="vGPU (NVIDIA GRID)"
    elif echo "$NVIDIA_PCI" | grep -qiE "GeForce|Quadro|Tesla|RTX|GTX|TITAN"; then
        if [ "$VM_TYPE" != "Bare Metal" ]; then
            GPU_TYPE="Passthrough (Physical GPU in VM)"
        else
            GPU_TYPE="Physical GPU (Bare Metal)"
        fi
    else
        # Unknown GPU type
        if [ "$VM_TYPE" != "Bare Metal" ]; then
            GPU_TYPE="Passthrough (assumed)"
        else
            GPU_TYPE="Physical GPU"
        fi
    fi
    echo "  GPU Type:     $GPU_TYPE"
else
    echo "  PCI Device:   NOT FOUND"
    echo ""
    echo "  [!] No NVIDIA GPU detected in PCI devices."
    echo "      If using vGPU, ensure a vGPU profile is assigned to this VM."
    echo ""
    exit 1
fi
echo ""

# ===========================================
# Driver Status
# ===========================================
echo "[DRIVER STATUS]"

if command -v nvidia-smi > /dev/null 2>&1; then
    echo "  nvidia-smi:   Installed"
    
    if nvidia-smi > /dev/null 2>&1; then
        echo "  Driver:       LOADED"
        
        # Get detailed info
        GPU_NAME=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1)
        DRIVER_VER=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader 2>/dev/null | head -1)
        GPU_MEM=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader 2>/dev/null | head -1)
        
        echo ""
        echo "  GPU Name:     $GPU_NAME"
        echo "  Driver Ver:   $DRIVER_VER"
        echo "  GPU Memory:   $GPU_MEM"
    else
        echo "  Driver:       NOT LOADED (nvidia-smi failed)"
        echo ""
        echo "  [!] Driver installed but not running. Try rebooting."
    fi
else
    echo "  nvidia-smi:   NOT INSTALLED"
    echo ""
    echo "  [!] NVIDIA driver not installed."
    echo ""
    
    # Check GPU type for install instructions
    if echo "$NVIDIA_PCI" | grep -qi "virtual\|grid"; then
        echo "  >>> vGPU DETECTED - Manual driver required <<<"
        echo ""
        echo "  Install steps:"
        echo "  1. Download vGPU guest driver from https://enterprise.nvidia.com"
        echo "  2. chmod +x NVIDIA-Linux-x86_64-XXX.XX-grid.run"
        echo "  3. sudo ./NVIDIA-Linux-x86_64-XXX.XX-grid.run --dkms"
        echo "  4. Reboot"
    else
        echo "  Install with: sudo apt install nvidia-driver-535"
        echo "  (or use ubuntu-drivers: sudo ubuntu-drivers autoinstall)"
    fi
    exit 1
fi
echo ""

# ===========================================
# vGPU License Status (if applicable)
# ===========================================
echo "[vGPU LICENSE]"

VGPU_INFO=$(nvidia-smi -q 2>/dev/null | grep -i "vgpu software licensed product")
if [ -n "$VGPU_INFO" ]; then
    LICENSE_TYPE=$(echo "$VGPU_INFO" | awk -F: '{print $2}' | xargs)
    echo "  License Type: $LICENSE_TYPE"
    
    # Check gridd.conf
    if [ -f "/etc/nvidia/gridd.conf" ]; then
        SERVER=$(grep -i "ServerAddress" /etc/nvidia/gridd.conf 2>/dev/null | grep -v "^#" | head -1)
        if [ -n "$SERVER" ]; then
            echo "  License Srv:  $(echo $SERVER | awk -F= '{print $2}')"
        fi
    fi
    
    # Check license status
    GRID_STATUS=$(systemctl is-active nvidia-gridd 2>/dev/null)
    echo "  gridd Status: $GRID_STATUS"
else
    echo "  License Type: N/A (physical GPU or unlicensed vGPU)"
fi
echo ""

# ===========================================
# NVENC Availability
# ===========================================
echo "[NVENC STATUS]"

NVENC_AVAILABLE="UNKNOWN"

# Check for vGPU profile restrictions
if [ -n "$VGPU_INFO" ]; then
    if echo "$VGPU_INFO" | grep -qi "compute\|vcs\|vws"; then
        NVENC_AVAILABLE="YES (Compute vGPU)"
    else
        NVENC_AVAILABLE="NO (Display-only vGPU)"
    fi
else
    # Physical GPU - check for encoder
    if nvidia-smi dmon -s u -c 1 > /dev/null 2>&1; then
        NVENC_AVAILABLE="YES (Physical GPU)"
    else
        NVENC_AVAILABLE="YES (Assumed - physical GPU)"
    fi
fi

echo "  NVENC:        $NVENC_AVAILABLE"

if echo "$NVENC_AVAILABLE" | grep -q "NO"; then
    echo ""
    echo "  [!] NVENC encoding not available with current vGPU profile."
    echo "      Request a compute profile (-C suffix) from your admin."
    echo "      OBS will fall back to software encoding (x264)."
fi
echo ""

# ===========================================
# Stable Stream Config
# ===========================================
echo "[STABLE STREAM CONFIG]"

if [ -f "/etc/stable-stream/gpu_env" ]; then
    SAVED_ENV=$(cat /etc/stable-stream/gpu_env)
    echo "  Detected at install: $SAVED_ENV"
else
    echo "  Detected at install: (not yet configured)"
fi
echo ""

# ===========================================
# Summary
# ===========================================
echo "=============================================="
echo "                  SUMMARY"
echo "=============================================="

if echo "$NVENC_AVAILABLE" | grep -qi "YES"; then
    echo "  ✅ GPU:      Ready"
    echo "  ✅ Driver:   Loaded"
    echo "  ✅ NVENC:    Available"
    echo ""
    echo "  Hardware encoding is available. OBS will use NVENC."
elif command -v nvidia-smi > /dev/null 2>&1 && nvidia-smi > /dev/null 2>&1; then
    echo "  ✅ GPU:      Ready"
    echo "  ✅ Driver:   Loaded"
    echo "  ⚠️  NVENC:    Restricted"
    echo ""
    echo "  GPU works for rendering but NVENC encoding is restricted."
    echo "  OBS will use software encoding (x264)."
else
    echo "  ❌ GPU:      Not Ready"
    echo "  ❌ Driver:   Missing"
    echo "  ❌ NVENC:    Not Available"
    echo ""
    echo "  Install the appropriate driver and reboot."
fi
echo ""
echo "=============================================="

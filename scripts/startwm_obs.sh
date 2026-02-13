#!/bin/sh
# Wayland session script for OBS with cage compositor
# cage is a minimal Wayland compositor that runs a single app fullscreen
# wayvnc runs INSIDE cage to capture the Wayland display

LOG="/tmp/obs-session.log"
echo "=== OBS Session Start $(date) ===" >> $LOG

# ===========================================
# Graceful Shutdown Handler
# ===========================================
SHUTDOWN=0

cleanup() {
    SHUTDOWN=1
    echo "Received shutdown signal at $(date), cleaning up..." >> $LOG
    # Kill OBS gracefully first
    if [ -n "$OBS_PID" ] && kill -0 $OBS_PID 2>/dev/null; then
        kill -TERM $OBS_PID 2>/dev/null
        # Give OBS a few seconds to save and exit
        for i in 1 2 3 4 5; do
            kill -0 $OBS_PID 2>/dev/null || break
            sleep 1
        done
        # Force kill if still running
        kill -9 $OBS_PID 2>/dev/null || true
    fi
    # Kill wayvnc and websockify
    kill $WAYVNC_PID 2>/dev/null || true
    kill $WEBSOCKIFY_PID 2>/dev/null || true
    # Kill pipewire/wireplumber we started
    pkill -x pipewire 2>/dev/null || true
    pkill -x wireplumber 2>/dev/null || true
    echo "Graceful shutdown complete at $(date)" >> $LOG
    exit 0
}

trap cleanup SIGTERM SIGINT SIGHUP

# ===========================================
# GPU Environment Detection at Runtime
# ===========================================
GPU_ENV="none"
NVENC_AVAILABLE="no"

# Read GPU environment from install-time detection
if [ -f "/etc/stable-stream/gpu_env" ]; then
    GPU_ENV=$(cat /etc/stable-stream/gpu_env)
fi

# Runtime check for nvidia-smi (driver loaded)
if command -v nvidia-smi > /dev/null 2>&1; then
    if nvidia-smi > /dev/null 2>&1; then
        GPU_NAME=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1)
        GPU_DRIVER=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader 2>/dev/null | head -1)
        echo "GPU detected: $GPU_NAME (driver: $GPU_DRIVER)" >> $LOG
        
        # Check if NVENC is likely available
        # For vGPU, check if it's a compute-capable profile
        VGPU_LICENSE=$(nvidia-smi -q 2>/dev/null | grep -i "vgpu software licensed product" | head -1)
        if [ -n "$VGPU_LICENSE" ]; then
            echo "vGPU license: $VGPU_LICENSE" >> $LOG
            if echo "$VGPU_LICENSE" | grep -qi "compute\|vcs\|vws"; then
                NVENC_AVAILABLE="yes"
                echo "NVENC: Available (compute vGPU profile)" >> $LOG
            else
                echo "NVENC: Restricted (display-only vGPU profile)" >> $LOG
            fi
        else
            # Consumer/datacenter GPU - NVENC always available
            NVENC_AVAILABLE="yes"
            echo "NVENC: Available (physical GPU)" >> $LOG
        fi
    else
        echo "nvidia-smi failed - driver not loaded properly" >> $LOG
    fi
else
    echo "No NVIDIA driver installed - using software encoding" >> $LOG
fi

echo "GPU Environment: $GPU_ENV, NVENC Available: $NVENC_AVAILABLE" >> $LOG

# ===========================================
# Wayland Environment Setup
# ===========================================
export XDG_SESSION_TYPE=wayland
# Force XWayland for CEF/browser plugin compatibility (NVENC still works via EGL)
export QT_QPA_PLATFORM=xcb
export OBS_USE_EGL=1

# NVIDIA GPU environment variables for hardware rendering
if [ "$NVENC_AVAILABLE" = "yes" ]; then
    export __NV_PRIME_RENDER_OFFLOAD=1
    export __GLX_VENDOR_LIBRARY_NAME=nvidia
    export LIBVA_DRIVER_NAME=nvidia
    export WLR_NO_HARDWARE_CURSORS=1
    export GBM_BACKEND=nvidia-drm
    # For vGPU, ensure we're using the right Vulkan ICD
    if [ "$GPU_ENV" = "vgpu" ]; then
        export VK_ICD_FILENAMES=/usr/share/vulkan/icd.d/nvidia_icd.json
    fi
    echo "Hardware encoding environment configured" >> $LOG
else
    # Software encoding fallback - disable GPU-specific settings
    unset __NV_PRIME_RENDER_OFFLOAD
    unset __GLX_VENDOR_LIBRARY_NAME
    unset LIBVA_DRIVER_NAME
    unset GBM_BACKEND
    # Use llvmpipe for software rendering
    export LIBGL_ALWAYS_SOFTWARE=1
    export WLR_RENDERER=pixman
    echo "Software encoding environment configured (llvmpipe)" >> $LOG
fi

# PipeWire for screen capture
export PIPEWIRE_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

echo "Environment set, WAYLAND_DISPLAY=$WAYLAND_DISPLAY XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR" >> $LOG

# ===========================================
# Start PipeWire if not running
# ===========================================
if ! pgrep -x pipewire > /dev/null; then
    pipewire &
    sleep 0.5
    echo "Started pipewire" >> $LOG
fi

if ! pgrep -x wireplumber > /dev/null; then
    wireplumber &
    sleep 0.5
    echo "Started wireplumber" >> $LOG
fi

# ===========================================
# Launch OBS Studio in background
# ===========================================
# Clear unclean shutdown sentinel to prevent safe mode popup
rm -rf "$HOME/.config/obs-studio/.sentinel" 2>/dev/null || true
echo "Launching OBS..." >> $LOG
obs --disable-shutdown-check --disable-updater --disable-missing-files-check >> $LOG 2>&1 &
OBS_PID=$!
echo "OBS started with PID $OBS_PID" >> $LOG
sleep 3
# Dismiss any safe mode / unclean shutdown dialog
ydotool key 28:1 28:0 >> $LOG 2>&1 || true

# ===========================================
# Start wayvnc INSIDE cage (critical - it needs access to WAYLAND_DISPLAY)
# ===========================================
echo "Starting wayvnc inside cage session..." >> $LOG

# Kill any stale wayvnc processes first
pkill -9 wayvnc 2>/dev/null || true

# wayvnc needs to run inside cage to access the Wayland display
# Listen on localhost only (127.0.0.1) - external access through authenticated Go backend proxy
# --max-fps=15 reduces bandwidth (15fps is sufficient for monitoring OBS)
wayvnc --log-level=info --max-fps=15 127.0.0.1 5900 >> /tmp/wayvnc.log 2>&1 &
WAYVNC_PID=$!
echo "wayvnc started with PID $WAYVNC_PID" >> $LOG
sleep 1

# Verify wayvnc started successfully
if kill -0 $WAYVNC_PID 2>/dev/null; then
    echo "wayvnc is running on port 5900" >> $LOG
else
    echo "WARNING: wayvnc failed to start, check /tmp/wayvnc.log" >> $LOG
fi

# ===========================================
# Start websockify for noVNC browser access
# ===========================================
echo "Starting websockify for noVNC..." >> $LOG

# Kill any existing websockify on port 6080
fuser -k 6080/tcp 2>/dev/null || true
sleep 0.5

# websockify proxies WebSocket (6080) to VNC (5900)
# Bind to localhost only - Go backend handles external auth
websockify 127.0.0.1:6080 localhost:5900 >> /tmp/websockify.log 2>&1 &
WEBSOCKIFY_PID=$!
echo "websockify started with PID $WEBSOCKIFY_PID" >> $LOG

# ===========================================
# OBS Auto-Restart Loop (watchdog)
# Keep OBS running - restart if user closes it
# Stops on SIGTERM (systemd stop) via trap above
# ===========================================
echo "Starting OBS watchdog loop..." >> $LOG

while [ "$SHUTDOWN" -eq 0 ]; do
    echo "Waiting for OBS (PID $OBS_PID) to exit..." >> $LOG
    wait $OBS_PID
    EXIT_CODE=$?
    echo "OBS exited with code $EXIT_CODE at $(date)" >> $LOG
    
    # If shutdown flag set (via signal), don't restart
    if [ "$SHUTDOWN" -ne 0 ]; then
        echo "Shutdown requested, not restarting OBS" >> $LOG
        break
    fi
    
    # Small delay before restart to prevent rapid restart loops
    sleep 2
    
    # Check again after sleep in case signal arrived during sleep
    if [ "$SHUTDOWN" -ne 0 ]; then
        echo "Shutdown requested, not restarting OBS" >> $LOG
        break
    fi
    
    # Clear unclean shutdown sentinel to prevent safe mode popup
    rm -rf "$HOME/.config/obs-studio/.sentinel" 2>/dev/null || true
    echo "Restarting OBS..." >> $LOG
    obs --disable-shutdown-check --disable-updater --disable-missing-files-check >> $LOG 2>&1 &
    OBS_PID=$!
    echo "OBS restarted with PID $OBS_PID" >> $LOG
    sleep 3
    # Dismiss any safe mode / unclean shutdown dialog
    ydotool key 28:1 28:0 >> $LOG 2>&1 || true
done

# Final cleanup
echo "Watchdog loop ended, cleaning up..." >> $LOG
kill $WAYVNC_PID 2>/dev/null || true
kill $WEBSOCKIFY_PID 2>/dev/null || true
echo "Session ended at $(date)" >> $LOG

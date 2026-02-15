# Stable Streaming

**Professional Live Streaming Server** — A self-hosted solution for managing IRL (In Real Life) streams with SRTLA bonding, OBS integration, and multi-destination streaming.

![Version](https://img.shields.io/badge/version-1.0.0-blue)
![License](https://img.shields.io/badge/license-AGPL--3.0-green)
![Platform](https://img.shields.io/badge/platform-Ubuntu%2022.04+-orange)

---

## ✨ Features

- **🎥 SRTLA Ingest** — Reliable SRTLA stream ingestion with automatic bonding via go-irl
- **🖥️ OBS Integration** — Built-in OBS Studio with WebSocket control and remote access via noVNC
- **📡 Multi-Destination Streaming** — Simultaneously stream to multiple platforms (Twitch, YouTube, etc.)
- **🔄 Automatic Scene Switching** — Switch OBS scenes based on connection bitrate thresholds
- **📊 Real-time Monitoring** — Live bitrate graphs and connection status
- **⚙️ Web-Based Management** — Modern React UI for complete system control
- **📱 Mobile Friendly** — Responsive design works on any device

---

## 📋 System Requirements

| Component | Requirement |
|-----------|-------------|
| **OS** | Ubuntu 22.04+ Server |
| **CPU** | 4+ cores recommended |
| **RAM** | 4GB minimum, 8GB recommended |
| **GPU** | Strongly Recommended GPU (hardware encoding) |
| **Network** | Public IP or port forwarding for ingest |

---

## 🚀 Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/OHMEED/stable-streaming.git
cd stable-streaming
```

### 2. Install Prerequisites

```bash
sudo apt install npm
```

### 3. Run the Installer

```bash
chmod +x install.sh
sudo ./install.sh
```
REBOOT AFTER INSTALL FOR GPU DRIVERS TO TAKE EFFECT
(GPU DRIVERS SHOULD INSTALL FINE ON BAREMETAL, BUT IVE ONLY TESTED ON PROXMOX WITH GPU PASSTHROUGH AS ENVIROMENT, IF NO HW ACCELERATION ON OBS IS DETECTED, INSTALL DRIVERS MANUALLY)

The installer will:
- Install all dependencies (Go, Node.js, OBS, nginx-rtmp, etc.)
- Build the backend and frontend
- Create systemd service for auto-start

### 4. Access the Web UI

```
http://<YOUR_SERVER_IP>:8080
```
- Default Username/Password is admin/admin
- Update password with stable-stream configure
---

## 🔧 CLI Commands

After installation, use the `stable-stream` CLI to manage the service:

```bash
stable-stream start      # Start the service
stable-stream stop       # Stop the service
stable-stream restart    # Restart the service
stable-stream status     # Check service status
stable-stream configure  # Re-run interactive configuration
stable-stream logs       # View service logs
stable-stream update     # Update from GitHub
stable-stream version    # Show version info
```

---

## 📖 Configuration

### First-Time Setup

The installer creates a default configuration at `/etc/stable-stream/config.env`:

| Setting | Default | Description |
|---------|---------|-------------|
| `USERNAME` | admin | Login username |
| `PASSWORD_HASH` | (hashed) | Bcrypt-hashed password |
| `MULTISTREAM_ENABLED` | y | Enable multi-destination streaming |
| `MAX_MULTISTREAMS` | 5 | Maximum stream destinations (1-5) |
| `INGESTS_LOCKED` | n | Prevent adding new ingests |

To reconfigure:
```bash
sudo stable-stream configure
```

### OBS WebSocket Settings

Access **Settings → OBS Websocket** in the web UI to get:
- WebSocket password (auto-generated)
- Connection URL for external OBS clients

---

## 📡 Creating an Ingest

1. Go to **Settings → Ingests** in the web UI
2. Click **Add Ingest**
3. Configure:
   - **Name**: Descriptive label
   - **Input Port**: SRTLA listening port
   - **Output Port**: Local UDP port for OBS
   - **Passphrase**: (Optional) SRT encryption key

### Connecting from Mobile

Use any SRTLA-compatible app (e.g., Larix Broadcaster):
- **Server**: `<YOUR_SERVER_IP>`
- **Port**: Your ingest input port
- **Passphrase**: If configured

---

## 🔄 Scene Switcher

Automatically switch OBS scenes based on connection quality:

1. Go to **Settings → Scene Switcher**
2. Configure:
   - **Ingest**: Which ingest to monitor
   - **Online Scene**: Scene when bitrate is above threshold
   - **Offline Scene**: Scene when bitrate drops below threshold
   - **Threshold**: Bitrate trigger (Kbps)
3. Enable the switcher

---

## 📺 Multi-Destination Streaming

Stream to multiple platforms simultaneously:

1. Enable in configuration: `sudo stable-stream configure`
2. Go to **Settings → Multistream**
3. Add destinations with RTMP URL and stream key
4. Toggle individual destinations on/off

> **Note**: Multi-streaming uses nginx-rtmp to re-stream. Requires additional bandwidth.

---

## 🛠️ Troubleshooting

### Service not starting
```bash
sudo stable-stream logs
journalctl -u stable-stream.service -f
```

### OBS not connecting
- Check that OBS Wayland service is running: `systemctl status obs-wayland`
- Verify WebSocket password in Settings → OBS Websocket

### Can't access web UI
- Verify firewall allows port 8080
- Check service status: `stable-stream status`

### Ingest not receiving stream
- Verify port forwarding for ingest ports
- Check firewall rules: `sudo ufw status`

---

## 📁 Directory Structure

```
stable-streaming/
├── backend/           # Go backend server
├── frontend/          # React frontend
├── bin/               # Compiled binaries
├── scripts/           # CLI and utility scripts
├── systemd/           # Service definitions
├── logs/              # Application logs
└── data.db            # SQLite database
```

---

## 📄 License

This project is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)**.

See [LICENSE](LICENSE) for details.

### Third-Party Components

This project uses several open-source components. See [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for details.

---


## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/OHMEED/stable-streaming/issues)
- **Documentation**: This README and in-app help

---


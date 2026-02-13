package obs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Manager struct {
	conn                *websocket.Conn
	host                string
	port                int
	pw                  string
	pendingRequests     sync.Map // map[string]chan []byte
	watchdogCancel      context.CancelFunc
	watchdogMu          sync.Mutex
	writeMu             sync.Mutex
	obsLaunchTime       time.Time // Track when OBS was last launched for startup grace period
	crashDialogAttempts int       // Track failed crash dialog dismissal attempts
	needsFullscreen     bool      // Track if OBS needs to be fullscreened after connecting
}

func NewManager() *Manager {
	return &Manager{
		host: "localhost",
		port: 4455, // Default OBS WebSocket port (v5)
		pw:   "",   // Password will be set from database
	}
}

// SetPassword updates the WebSocket password used for OBS connection
func (m *Manager) SetPassword(pw string) {
	m.pw = pw
}

// GetPassword returns the current WebSocket password
func (m *Manager) GetPassword() string {
	return m.pw
}

// GetPort returns the WebSocket port
func (m *Manager) GetPort() int {
	return m.port
}
func (m *Manager) EnsureRunning() error {
	// Check if OBS is running
	obsRunning := m.isOBSRunning()
	if obsRunning {
		log.Println("OBS is already running.")
		// Set launch time to now so grace period applies even when backend restarts
		// This prevents crash dialog handler from firing immediately
		m.obsLaunchTime = time.Now()
		m.startWatchdog()
		return nil
	}

	log.Println("OBS not running, attempting to launch...")

	// Configure WebSocket before launch
	if err := m.ConfigureWebSocket(); err != nil {
		log.Printf("Failed to configure OBS WebSocket: %v", err)
	}

	// Launch OBS based on platform
	switch runtime.GOOS {
	case "windows":
		// Windows: Launch OBS directly, start VNC server for remote access
		log.Println("Windows: Launching OBS with VNC support...")

		// Start TightVNC server if not running
		m.ensureVNCRunning()

		// Start websockify for noVNC browser access
		m.ensureWebsockifyRunning()

		// Launch OBS
		m.launchOBS()

	case "darwin":
		// macOS: Check for XPRA (shadow mode for existing display)
		if _, err := exec.LookPath("xpra"); err != nil {
			// No XPRA, launch OBS directly
			log.Println("Launching OBS directly on macOS...")
			cmd := exec.Command("open", "-a", "OBS", "--args", "--disable-shutdown-check", "--disable-updater", "--disable-missing-files-check")
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to launch OBS: %w", err)
			}
		} else {
			// XPRA shadow mode for macOS
			log.Println("Launching OBS via XPRA shadow on macOS...")
			if err := exec.Command("open", "-a", "OBS", "--args", "--disable-shutdown-check", "--disable-updater", "--disable-missing-files-check").Run(); err != nil {
				log.Printf("Failed to open OBS app: %v", err)
			}
			time.Sleep(2 * time.Second)

			cmd := exec.Command("xpra", "shadow", ":0",
				"--bind-tcp=0.0.0.0:10000",
				"--html=on",
				"--daemon=yes",
			)

			os.MkdirAll("logs", 0755)
			logFile, _ := os.Create("logs/xpra_launch.log")
			if logFile != nil {
				cmd.Stdout = logFile
				cmd.Stderr = logFile
			}

			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to launch XPRA: %w", err)
			}
			log.Println("Launched XPRA (Shadow) on port 10000")
		}

	default:
		// Linux: OBS is managed by systemd (obs-wayland.service)
		// The service is started automatically via WantedBy dependency
		// We just need to wait for it to become available
		log.Println("Linux: Waiting for OBS Wayland session (managed by systemd)...")

		// Wait up to 30 seconds for OBS to start
		for i := 0; i < 30; i++ {
			if m.isOBSRunning() {
				log.Println("OBS is now running")
				break
			}
			log.Printf("Waiting for OBS... (%d/30)", i+1)
			time.Sleep(1 * time.Second)
		}

		// Don't try to restart OBS via sudo - let systemd handle it
		// The watchdog will detect if OBS crashes and the service will auto-restart
	}

	// Start Polling Watchdog (Background)
	m.startWatchdog()

	return nil
}

func (m *Manager) startWatchdog() {
	m.watchdogMu.Lock()
	defer m.watchdogMu.Unlock()

	// Cancel existing watchdog if any
	if m.watchdogCancel != nil {
		m.watchdogCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.watchdogCancel = cancel

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !m.isOBSRunning() {
					log.Println("Watchdog: OBS process not found. Restarting...")
					m.launchOBS()
				} else {
					// OBS is running, check for crash dialog and dismiss if present
					m.handleCrashDialog()
				}
			}
		}
	}()
}

func (m *Manager) Stop() {
	log.Println("Manager: Shutting down OBS gracefully...")

	// Cancel watchdog first
	m.watchdogMu.Lock()
	if m.watchdogCancel != nil {
		m.watchdogCancel()
	}
	m.watchdogMu.Unlock()

	switch runtime.GOOS {
	case "windows":
		log.Println("Stopping OBS on Windows...")
		exec.Command("taskkill", "/IM", "obs64.exe", "/F").Run()
		exec.Command("taskkill", "/IM", "obs32.exe", "/F").Run()
		// Note: We leave VNC and websockify running for reconnection
	case "linux":
		m.gracefulStopLinux()
	case "darwin":
		log.Println("Stopping OBS on macOS...")
		exec.Command("pkill", "-TERM", "obs").Run()
		time.Sleep(2 * time.Second)
		exec.Command("pkill", "-KILL", "obs").Run()
		exec.Command("pkill", "xpra").Run()
	}

	log.Println("Manager: Shutdown sequence complete.")
}

// gracefulStopLinux performs a clean shutdown of OBS on Linux to prevent crash dialogs
func (m *Manager) gracefulStopLinux() {
	log.Println("Stopping OBS gracefully on Linux...")

	// Step 1: Try to stop via OBS WebSocket (cleanest method)
	if m.conn != nil {
		log.Println("Attempting graceful shutdown via WebSocket...")
		// This would be ideal but OBS doesn't have a "quit" command in websocket
		// So we proceed with signal-based shutdown
	}

	// Step 2: Send SIGTERM to OBS for graceful shutdown
	log.Println("Sending SIGTERM to OBS...")
	exec.Command("pkill", "-TERM", "-x", "obs").Run()

	// Step 3: Wait up to 5 seconds for OBS to exit cleanly
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if exec.Command("pgrep", "-x", "obs").Run() != nil {
			log.Println("OBS exited cleanly")
			break
		}
		if i == 9 {
			log.Println("OBS didn't exit gracefully, sending SIGKILL...")
			exec.Command("pkill", "-KILL", "-x", "obs").Run()
		}
	}

	// Step 4: Clean up sentinel files to prevent crash recovery dialog
	m.cleanupCrashState()

	// Step 5: Stop the Wayland session services
	log.Println("Stopping Wayland session...")
	exec.Command("systemctl", "stop", "obs-wayland").Run()
}

// cleanupCrashState removes OBS crash/sentinel files to prevent crash recovery dialog on next launch
func (m *Manager) cleanupCrashState() {
	log.Println("Cleaning up OBS crash state...")

	// Get home directory for user
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/home/user"
	}

	obsConfigDir := filepath.Join(homeDir, ".config", "obs-studio")

	// Remove sentinel directory (OBS uses this to detect crashes)
	sentinelDir := filepath.Join(obsConfigDir, ".sentinel")
	if err := os.RemoveAll(sentinelDir); err != nil {
		log.Printf("Failed to remove sentinel dir: %v", err)
	} else {
		log.Println("Removed OBS sentinel directory")
	}

	// Remove any crash-related files
	crashFiles := []string{
		filepath.Join(obsConfigDir, ".crash"),
		filepath.Join(obsConfigDir, "crashes"),
	}
	for _, f := range crashFiles {
		os.RemoveAll(f)
	}

	// Remove backup files that might trigger recovery
	files, _ := filepath.Glob(filepath.Join(obsConfigDir, "*.bak"))
	for _, f := range files {
		os.Remove(f)
	}

	log.Println("OBS crash state cleanup complete")
}

func (m *Manager) launchOBS() {
	log.Println("Launching OBS process (native)...")

	var obsCmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Windows: Try common OBS install paths
		obsPaths := []string{
			`C:\Program Files\obs-studio\bin\64bit\obs64.exe`,
			`C:\Program Files (x86)\obs-studio\bin\64bit\obs64.exe`,
			`C:\Program Files (x86)\obs-studio\bin\32bit\obs32.exe`,
		}

		var obsPath string
		for _, p := range obsPaths {
			if _, err := os.Stat(p); err == nil {
				obsPath = p
				break
			}
		}

		if obsPath == "" {
			log.Printf("ERROR: OBS not found in standard locations")
			return
		}

		log.Printf("Launching OBS from: %s", obsPath)
		obsCmd = exec.Command(obsPath, "--disable-shutdown-check", "--disable-updater", "--disable-missing-files-check")

	case "darwin":
		obsCmd = exec.Command("open", "-a", "OBS", "--args", "--disable-shutdown-check", "--disable-updater", "--disable-missing-files-check")

	default:
		// Linux: OBS is managed by systemd. If it's not running, restart the service.
		log.Println("Linux: OBS not running - forcing systemctl restart obs-wayland...")
		exec.Command("sudo", "systemctl", "restart", "obs-wayland").Run()
		m.obsLaunchTime = time.Now()
		m.needsFullscreen = true
		return
	}

	// Log OBS output
	os.MkdirAll("logs", 0755)
	obsLog, logErr := os.Create("logs/obs_process.log")
	if logErr == nil {
		obsCmd.Stdout = obsLog
		obsCmd.Stderr = obsLog
	}

	if err := obsCmd.Start(); err != nil {
		log.Printf("Failed to start OBS process: %v", err)
	} else {
		m.obsLaunchTime = time.Now() // Track launch time for startup grace period
		m.needsFullscreen = true     // Fullscreen OBS when WebSocket connects
		log.Printf("Launched OBS process on %s", runtime.GOOS)
	}
}

func (m *Manager) isOBSRunning() bool {
	switch runtime.GOOS {
	case "windows":
		// Check for obs64.exe or obs32.exe using tasklist
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq obs64.exe", "/NH").Output()
		if err == nil && strings.Contains(string(out), "obs64.exe") {
			return true
		}
		out, err = exec.Command("tasklist", "/FI", "IMAGENAME eq obs32.exe", "/NH").Output()
		if err == nil && strings.Contains(string(out), "obs32.exe") {
			return true
		}
		return false
	case "linux":
		// On Linux, we check if the service is active AND the process exists
		serviceActive := exec.Command("systemctl", "is-active", "--quiet", "obs-wayland").Run() == nil
		if !serviceActive {
			log.Println("Watchdog: obs-wayland service is not active")
			return false
		}
		processExists := exec.Command("pgrep", "-x", "obs").Run() == nil
		if !processExists {
			log.Println("Watchdog: obs-wayland service is active, but obs process is missing")
			return false
		}
		return true
	case "darwin":
		return exec.Command("pgrep", "-x", "OBS").Run() == nil
	}
	return false
}

// handleCrashDialog detects OBS crash recovery dialog and auto-dismisses it
// by pressing Enter using ydotool. Once OBS WebSocket connects, presses F11
// to fullscreen the main window.
// NOTE: wtype was removed as it caused compositor instability.
func (m *Manager) handleCrashDialog() {
	if runtime.GOOS != "linux" {
		return
	}

	// Only attempt crash dialog dismissal if:
	// 1. OBS process is confirmed running (isOBSRunning() == true)
	// 2. But WebSocket is not connected (m.conn == nil)
	// 3. AND we've waited at least 30 seconds since OBS launch (startup grace period)
	if !m.isOBSRunning() {
		return
	}

	// Grace period: Wait 60 seconds after OBS launch before trying to dismiss dialogs
	if time.Since(m.obsLaunchTime) < 60*time.Second {
		return
	}

	// OBS process exists but WebSocket is not connected after grace period
	// This could mean a crash dialog is blocking OBS from fully starting
	if m.conn == nil {
		m.crashDialogAttempts++

		if m.crashDialogAttempts >= 6 {
			// After 6 failed attempts, force restart OBS
			log.Printf("Watchdog: OBS unresponsive after %d crash dialog attempts. Force restarting...", m.crashDialogAttempts)
			m.crashDialogAttempts = 0
			m.forceRestartOBS()
			return
		}

		// Only press Enter if we aren't even connected yet.
		// If we are connected but not identified yet, don't press anything.
		log.Printf("Watchdog: OBS process running but WebSocket not connected after 60s (attempt %d/6), pressing Enter to dismiss crash dialog...", m.crashDialogAttempts)
		m.pressKey("28") // Enter key to dismiss crash dialog
	} else {
		// WebSocket connected - OBS is now running and connected
		// Note: labwc window rules now auto-fullscreen OBS, no keypress needed
		if m.needsFullscreen {
			log.Println("OBS WebSocket connected successfully")
			m.needsFullscreen = false
		}
		// Reset crash dialog attempt counter
		m.crashDialogAttempts = 0
	}
}

// pressKey sends a keypress using ydotool (uinput-based, doesn't interfere with Wayland compositor)
// keyCode is the Linux input event code (e.g., 28 = Enter, 87 = F11)
func (m *Manager) pressKey(keyCode string) {
	if _, err := exec.LookPath("ydotool"); err != nil {
		log.Println("Warning: ydotool not found, cannot send keypress")
		return
	}

	// ydotool key syntax: keycode:1 (press) keycode:0 (release)
	cmd := exec.Command("ydotool", "key", keyCode+":1", keyCode+":0")
	if err := cmd.Run(); err != nil {
		log.Printf("ydotool key %s failed: %v", keyCode, err)
	} else {
		log.Printf("Sent keypress via ydotool: keycode %s", keyCode)
	}
}

// fullscreenOBSWindow focuses the OBS window using Alt+Tab, then presses F11 to fullscreen
func (m *Manager) fullscreenOBSWindow() {
	if _, err := exec.LookPath("ydotool"); err != nil {
		log.Println("Warning: ydotool not found, cannot fullscreen OBS window")
		return
	}

	// Alt+Tab to focus the OBS window (assuming it's the most recent/only window)
	// Alt is keycode 56, Tab is keycode 15
	log.Println("Pressing Alt+Tab to focus OBS window...")
	cmd := exec.Command("ydotool", "key", "56:1", "15:1", "15:0", "56:0")
	if err := cmd.Run(); err != nil {
		log.Printf("ydotool Alt+Tab failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Press F11 to fullscreen (now works because we added F11 keybinding to labwc config)
	// F11 is keycode 87
	log.Println("Pressing F11 to fullscreen OBS...")
	cmd = exec.Command("ydotool", "key", "87:1", "87:0")
	if err := cmd.Run(); err != nil {
		log.Printf("ydotool F11 failed: %v", err)
	}
}

// forceRestartOBS kills OBS and relaunches it
func (m *Manager) forceRestartOBS() {
	log.Println("Force restarting OBS...")

	// Kill OBS process
	exec.Command("pkill", "-9", "-x", "obs").Run()
	time.Sleep(2 * time.Second)

	// Clean up crash state to prevent crash dialog on restart
	m.cleanupCrashState()

	// Relaunch OBS
	m.launchOBS()
}

// ensureVNCRunning starts TightVNC server on Windows if not already running
func (m *Manager) ensureVNCRunning() {
	if runtime.GOOS != "windows" {
		return
	}

	// Check if TightVNC server is running
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq tvnserver.exe", "/NH").Output()
	if err == nil && strings.Contains(string(out), "tvnserver.exe") {
		log.Println("TightVNC server already running")
		return
	}

	log.Println("Starting TightVNC server...")

	// Try to start TightVNC service
	if err := exec.Command("net", "start", "tvnserver").Run(); err != nil {
		log.Printf("Failed to start TightVNC service: %v", err)

		// Try direct execution as fallback
		vncPaths := []string{
			`C:\Program Files\TightVNC\tvnserver.exe`,
			`C:\Program Files (x86)\TightVNC\tvnserver.exe`,
		}

		for _, p := range vncPaths {
			if _, err := os.Stat(p); err == nil {
				cmd := exec.Command(p, "-run")
				if err := cmd.Start(); err != nil {
					log.Printf("Failed to start TightVNC directly: %v", err)
				} else {
					log.Println("TightVNC server started")
				}
				return
			}
		}
		log.Println("ERROR: TightVNC not found. Please install TightVNC.")
	} else {
		log.Println("TightVNC service started")
	}
}

// ensureWebsockifyRunning starts websockify for noVNC browser access on Windows
func (m *Manager) ensureWebsockifyRunning() {
	if runtime.GOOS != "windows" {
		return
	}

	// Check if websockify is already running
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq python.exe", "/NH").Output()
	if err == nil && strings.Contains(string(out), "python.exe") {
		// Could be websockify or another python process - check for port 6080
		portCheck, _ := exec.Command("netstat", "-ano").Output()
		if strings.Contains(string(portCheck), ":6080") {
			log.Println("Websockify already running on port 6080")
			return
		}
	}

	log.Println("Starting websockify for noVNC...")

	// Start websockify: websockify 6080 localhost:5900
	cmd := exec.Command("python", "-m", "websockify", "--web", getNoVNCPath(), "6080", "localhost:5900")

	os.MkdirAll("logs", 0755)
	logFile, _ := os.Create("logs/websockify.log")
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start websockify: %v", err)
	} else {
		log.Println("Websockify started on port 6080")
	}
}

// getNoVNCPath returns the path to noVNC web files
func getNoVNCPath() string {
	// Check common locations for noVNC
	paths := []string{
		"./novnc",
		"./noVNC",
		fmt.Sprintf("%s/AppData/Local/Programs/Python/Python312/Lib/site-packages/websockify/web", os.Getenv("USERPROFILE")),
		fmt.Sprintf("%s/AppData/Local/Programs/Python/Python311/Lib/site-packages/websockify/web", os.Getenv("USERPROFILE")),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Return empty - websockify will use its built-in web interface
	return ""
}

// Connect establishes connection to OBS WebSocket with retry logic
func (m *Manager) Connect() error {
	go m.maintainConnection()
	return nil
}

func (m *Manager) maintainConnection() {
	url := fmt.Sprintf("ws://%s:%d", m.host, m.port)
	const fallbackPassword = "password"

	for {
		log.Printf("Connecting to OBS WebSocket at %s...", url)

		err := m.connectOnce(url)
		if err == nil {
			log.Println("Connected to OBS WebSocket")
			m.monitorConnection()
		} else {
			log.Printf("Failed to connect to OBS WebSocket: %v. Retrying in 5s...", err)
		}

		time.Sleep(5 * time.Second)
	}
}

// generateRecoveryPassword creates a new password when recovering from fallback
func (m *Manager) generateRecoveryPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 20)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond) // Ensure different values
	}
	return string(b)
}

func (m *Manager) connectOnce(url string) error {
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}

	// OBS v5 auth logic ... (same as before)
	// 1. Read Hello (OpCode 0)
	_, message, err := c.ReadMessage()
	if err != nil {
		c.Close()
		return fmt.Errorf("failed to read hello: %w", err)
	}

	var hello struct {
		Op int `json:"op"`
		D  struct {
			RpcVersion     int `json:"rpcVersion"`
			Authentication *struct {
				Challenge string `json:"challenge"`
				Salt      string `json:"salt"`
			} `json:"authentication"`
		} `json:"d"`
	}

	if err := json.Unmarshal(message, &hello); err != nil {
		c.Close()
		return fmt.Errorf("failed to parse hello: %w", err)
	}

	if hello.Op != 0 {
		c.Close()
		return fmt.Errorf("expected hello op 0, got %d", hello.Op)
	}

	// 2. Compute Auth if needed
	authString := ""
	if hello.D.Authentication != nil {
		secret := sha256.Sum256([]byte(m.pw + hello.D.Authentication.Salt))
		secretB64 := base64.StdEncoding.EncodeToString(secret[:])

		auth := sha256.Sum256([]byte(secretB64 + hello.D.Authentication.Challenge))
		authString = base64.StdEncoding.EncodeToString(auth[:])
	}

	// 3. Send Identify (OpCode 1)
	identify := map[string]interface{}{
		"op": 1,
		"d": map[string]interface{}{
			"rpcVersion": 1,
		},
	}

	if authString != "" {
		identify["d"].(map[string]interface{})["authentication"] = authString
	}

	m.writeMu.Lock()
	err = c.WriteJSON(identify)
	m.writeMu.Unlock()
	if err != nil {
		c.Close()
		m.conn = nil
		return fmt.Errorf("failed to send identify: %w", err)
	}

	// 4. Wait for Identified (OpCode 2)
	_, message, err = c.ReadMessage()
	if err != nil {
		c.Close()
		return fmt.Errorf("failed to read identified: %w", err)
	}

	var identified struct {
		Op int `json:"op"`
	}
	if err := json.Unmarshal(message, &identified); err != nil {
		c.Close()
		return fmt.Errorf("failed to parse identified: %w", err)
	}

	if identified.Op != 2 {
		c.Close()
		return fmt.Errorf("expected identified op 2, got %d (might be auth failure)", identified.Op)
	}

	// Success
	m.conn = c

	// Ensure required scenes exist
	go m.EnsureScenes()

	return nil
}

func (m *Manager) monitorConnection() {
	if m.conn == nil {
		return
	}
	defer func() {
		if m.conn != nil {
			m.conn.Close()
			m.conn = nil
		}
		// Close all pending request channels to unblock waiters
		m.pendingRequests.Range(func(key, value interface{}) bool {
			if ch, ok := value.(chan []byte); ok {
				close(ch)
			}
			return true
		})
	}()

	// Loop reading messages to detect disconnect
	for {
		_, message, err := m.conn.ReadMessage()
		if err != nil {
			log.Printf("OBS WebSocket disconnected: %v", err)
			return
		}

		// Parse OpCode to route message
		var base struct {
			Op int `json:"op"`
			D  struct {
				RequestId string `json:"requestId"`
			} `json:"d"`
		}
		if err := json.Unmarshal(message, &base); err != nil {
			log.Printf("Failed to parse message base: %v", err)
			continue
		}

		// Op 7 is RequestResponse
		if base.Op == 7 {
			if ch, ok := m.pendingRequests.Load(base.D.RequestId); ok {
				select {
				case ch.(chan []byte) <- message:
				default:
					log.Printf("Warning: Dropped response for %s, channel full", base.D.RequestId)
				}
			}
		}
		// We could process events (Op 5) here
	}
}

// sendRequestAndWait sends a request and waits for the response with the same requestId
func (m *Manager) sendRequestAndWait(reqType string, reqData map[string]interface{}) ([]byte, error) {
	if m.conn == nil {
		return nil, fmt.Errorf("not connected to OBS")
	}

	reqID := fmt.Sprintf("%d", time.Now().UnixNano())
	req := map[string]interface{}{
		"op": 6,
		"d": map[string]interface{}{
			"requestType": reqType,
			"requestId":   reqID,
			"requestData": reqData,
		},
	}

	responseChan := make(chan []byte, 1)
	m.pendingRequests.Store(reqID, responseChan)
	defer m.pendingRequests.Delete(reqID)

	m.writeMu.Lock()
	err := m.conn.WriteJSON(req)
	m.writeMu.Unlock()
	if err != nil {
		return nil, err
	}

	select {
	case res, ok := <-responseChan:
		if !ok {
			return nil, fmt.Errorf("connection closed while waiting for response")
		}
		return res, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

func (m *Manager) EnsureScenes() {
	requiredScenes := []string{"Starting Soon", "BRB", "Ingest 1", "Ingest 2", "Ingest Offline"}

	// Get current scenes to avoid duplicates (though CreateScene might just fail or ignore)
	currentScenes, err := m.GetSceneList()
	if err != nil {
		log.Printf("Failed to get scene list: %v", err)
		return
	}

	existing := make(map[string]bool)
	for _, s := range currentScenes {
		existing[s] = true
	}

	for _, scene := range requiredScenes {
		if !existing[scene] {
			log.Printf("Creating missing scene: %s", scene)
			if err := m.CreateScene(scene); err != nil {
				log.Printf("Failed to create scene %s: %v", scene, err)
			}
		}
	}
}

func (m *Manager) CreateScene(sceneName string) error {
	data := map[string]interface{}{
		"sceneName": sceneName,
	}
	_, err := m.sendRequestAndWait("CreateScene", data)
	return err
}

func (m *Manager) GetSceneList() ([]string, error) {
	respBytes, err := m.sendRequestAndWait("GetSceneList", nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Op int `json:"op"`
		D  struct {
			ResponseData struct {
				Scenes []struct {
					SceneName string `json:"sceneName"`
				} `json:"scenes"`
			} `json:"responseData"`
		} `json:"d"`
	}

	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, fmt.Errorf("failed to parse scene list: %v", err)
	}

	var names []string
	for _, s := range res.D.ResponseData.Scenes {
		names = append(names, s.SceneName)
	}

	return names, nil
}

func (m *Manager) GetSceneItemList(sceneName string) ([]string, error) {
	data := map[string]interface{}{
		"sceneName": sceneName,
	}
	respBytes, err := m.sendRequestAndWait("GetSceneItemList", data)
	if err != nil {
		return nil, err
	}

	var res struct {
		D struct {
			ResponseData struct {
				SceneItems []struct {
					SourceName string `json:"sourceName"`
				} `json:"sceneItems"`
			} `json:"responseData"`
		} `json:"d"`
	}

	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, err
	}

	var items []string
	for _, item := range res.D.ResponseData.SceneItems {
		items = append(items, item.SourceName)
	}
	return items, nil
}

// GetVideoSettings returns the base canvas dimensions (width, height)
func (m *Manager) GetVideoSettings() (int, int, error) {
	respBytes, err := m.sendRequestAndWait("GetVideoSettings", nil)
	if err != nil {
		return 0, 0, err
	}

	var res struct {
		D struct {
			ResponseData struct {
				BaseWidth  int `json:"baseWidth"`
				BaseHeight int `json:"baseHeight"`
			} `json:"responseData"`
		} `json:"d"`
	}

	if err := json.Unmarshal(respBytes, &res); err != nil {
		return 0, 0, fmt.Errorf("failed to parse video settings: %v", err)
	}

	return res.D.ResponseData.BaseWidth, res.D.ResponseData.BaseHeight, nil
}

// GetSceneItemId returns the scene item ID for a source in a scene
func (m *Manager) GetSceneItemId(sceneName, sourceName string) (int, error) {
	data := map[string]interface{}{
		"sceneName":  sceneName,
		"sourceName": sourceName,
	}
	respBytes, err := m.sendRequestAndWait("GetSceneItemId", data)
	if err != nil {
		return 0, err
	}

	var res struct {
		D struct {
			ResponseData struct {
				SceneItemId int `json:"sceneItemId"`
			} `json:"responseData"`
		} `json:"d"`
	}

	if err := json.Unmarshal(respBytes, &res); err != nil {
		return 0, fmt.Errorf("failed to parse scene item id: %v", err)
	}

	return res.D.ResponseData.SceneItemId, nil
}

// SetSceneItemTransform applies a transform to a scene item
// boundsType can be: OBS_BOUNDS_NONE, OBS_BOUNDS_STRETCH, OBS_BOUNDS_SCALE_INNER (fit), OBS_BOUNDS_SCALE_OUTER (fill), etc.
func (m *Manager) SetSceneItemTransform(sceneName string, sceneItemId int, boundsType string, boundsWidth, boundsHeight float64) error {
	data := map[string]interface{}{
		"sceneName":   sceneName,
		"sceneItemId": sceneItemId,
		"sceneItemTransform": map[string]interface{}{
			"boundsType":      boundsType,
			"boundsWidth":     boundsWidth,
			"boundsHeight":    boundsHeight,
			"boundsAlignment": 0, // Center alignment
		},
	}
	_, err := m.sendRequestAndWait("SetSceneItemTransform", data)
	return err
}

func (m *Manager) AddMediaSource(sceneName, sourceName, protocol, url, inputFormat string) error {
	// 1. Check if source already exists in the scene
	items, err := m.GetSceneItemList(sceneName)
	if err != nil {
		return fmt.Errorf("failed to get scene item list: %w", err)
	}

	for _, item := range items {
		if item == sourceName {
			return fmt.Errorf("source %s already exists in scene %s", sourceName, sceneName)
		}
	}

	// 2. Prepare Input Settings based on protocol
	inputSettings := map[string]interface{}{
		"is_local_file":       false,
		"input":               url,
		"buffering_mb":        1,    // Network buffering 1MB
		"reconnect_delay_sec": 1,    // Reconnect delay 1s
		"hw_decode":           true, // Use hardware decoding
		"close_when_inactive": false,
		"restart_on_activate": false,
		"seekable":            false,
		"clear_on_media_end":  false,
	}

	// Protocol specific overrides
	// SRTLA uses UDP mpegts output from go-irl, others use RTSP
	if inputFormat != "" {
		inputSettings["input_format"] = inputFormat
	} else {
		inputSettings["input_format"] = ""
	}

	createData := map[string]interface{}{
		"sceneName":        sceneName,
		"inputName":        sourceName,
		"inputKind":        "ffmpeg_source",
		"inputSettings":    inputSettings,
		"sceneItemEnabled": true,
	}

	_, err = m.sendRequestAndWait("CreateInput", createData)
	if err != nil {
		return fmt.Errorf("failed to create input: %w", err)
	}

	// 3. Apply "Fit to Screen" transform
	// Get canvas dimensions
	canvasWidth, canvasHeight, err := m.GetVideoSettings()
	if err != nil {
		log.Printf("Warning: Failed to get video settings for fit-to-screen: %v", err)
		return nil // Source was created, just couldn't apply transform
	}

	// Get the scene item ID for the newly created source
	sceneItemId, err := m.GetSceneItemId(sceneName, sourceName)
	if err != nil {
		log.Printf("Warning: Failed to get scene item ID for fit-to-screen: %v", err)
		return nil // Source was created, just couldn't apply transform
	}

	// Apply fit-to-screen transform using OBS_BOUNDS_SCALE_INNER (like "Fit to Screen")
	err = m.SetSceneItemTransform(sceneName, sceneItemId, "OBS_BOUNDS_SCALE_INNER", float64(canvasWidth), float64(canvasHeight))
	if err != nil {
		log.Printf("Warning: Failed to apply fit-to-screen transform: %v", err)
		return nil // Source was created, just couldn't apply transform
	}

	log.Printf("Successfully added source %s to scene %s with fit-to-screen transform", sourceName, sceneName)
	return nil
}

// Scene selection
func (m *Manager) GetCurrentProgramScene() (string, error) {
	respBytes, err := m.sendRequestAndWait("GetCurrentProgramScene", nil)
	if err != nil {
		return "", err
	}

	var res struct {
		D struct {
			ResponseData struct {
				CurrentProgramSceneName string `json:"currentProgramSceneName"`
			} `json:"responseData"`
		} `json:"d"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", err
	}

	return res.D.ResponseData.CurrentProgramSceneName, nil
}

func (m *Manager) SetCurrentProgramScene(sceneName string) error {
	data := map[string]interface{}{
		"sceneName": sceneName,
	}
	_, err := m.sendRequestAndWait("SetCurrentProgramScene", data)
	return err
}

func (m *Manager) SetScene(sceneName string) error {
	data := map[string]interface{}{
		"sceneName": sceneName,
	}
	_, err := m.sendRequestAndWait("SetCurrentProgramScene", data)
	return err
}

func (m *Manager) IsConnected() bool {
	return m.conn != nil
}

// Streaming
func (m *Manager) GetStreamStatus() (bool, error) {
	respBytes, err := m.sendRequestAndWait("GetStreamStatus", nil)
	if err != nil {
		return false, err
	}

	var res struct {
		D struct {
			ResponseData struct {
				OutputActive bool `json:"outputActive"`
			} `json:"responseData"`
		} `json:"d"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return false, err
	}
	return res.D.ResponseData.OutputActive, nil
}

func (m *Manager) ToggleStream() error {
	_, err := m.sendRequestAndWait("ToggleStream", nil)
	return err
}

// Recording
func (m *Manager) GetRecordStatus() (bool, error) {
	respBytes, err := m.sendRequestAndWait("GetRecordStatus", nil)
	if err != nil {
		return false, err
	}

	var res struct {
		D struct {
			ResponseData struct {
				OutputActive bool `json:"outputActive"`
			} `json:"responseData"`
		} `json:"d"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return false, err
	}
	return res.D.ResponseData.OutputActive, nil
}

func (m *Manager) ToggleRecord() error {
	_, err := m.sendRequestAndWait("ToggleRecord", nil)
	return err
}

// Preview (Screenshot)
func (m *Manager) GetScreenshot() (string, error) {
	sceneName, err := m.GetCurrentProgramScene()
	if err != nil {
		return "", fmt.Errorf("failed to get current scene for screenshot: %w", err)
	}

	data := map[string]interface{}{
		"sourceName":              sceneName,
		"imageFormat":             "jpg",
		"imageWidth":              800, // Optimized for 0.5s frequency
		"imageHeight":             450,
		"imageCompressionQuality": 70, // Slightly lower for faster transmission
	}

	respBytes, err := m.sendRequestAndWait("GetSourceScreenshot", data)
	if err != nil {
		return "", err
	}

	var res struct {
		D struct {
			ResponseData struct {
				ImageData string `json:"imageData"`
			} `json:"responseData"`
		} `json:"d"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", err
	}

	imgData := res.D.ResponseData.ImageData
	if strings.HasPrefix(imgData, "data:") {
		if parts := strings.SplitN(imgData, ",", 2); len(parts) > 1 {
			imgData = parts[1]
		}
	}

	return imgData, nil
}

func (m *Manager) GetRecordDirectory() (string, error) {
	respBytes, err := m.sendRequestAndWait("GetRecordDirectory", nil)
	if err != nil {
		return "", err
	}

	var res struct {
		D struct {
			ResponseData struct {
				RecordDirectory string `json:"recordDirectory"`
			} `json:"responseData"`
		} `json:"d"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", err
	}

	return res.D.ResponseData.RecordDirectory, nil
}

func (m *Manager) ConfigureWebSocket() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	var configPath string
	switch runtime.GOOS {
	case "windows":
		// Windows: Use APPDATA environment variable
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		configPath = filepath.Join(appData, "obs-studio", "plugin_config", "obs-websocket", "config.json")
	case "darwin":
		configPath = filepath.Join(home, "Library", "Application Support", "obs-studio", "plugin_config", "obs-websocket", "config.json")
	default:
		// Linux: OBS runs as 'user', but backend may run as root/service
		// Always use /home/user for OBS config
		obsHome := "/home/user"
		if h := os.Getenv("OBS_USER_HOME"); h != "" {
			obsHome = h
		}
		configPath = filepath.Join(obsHome, ".config", "obs-studio", "plugin_config", "obs-websocket", "config.json")
	}

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(configPath), 0755)

	// Build OBS WebSocket plugin config JSON
	config := map[string]interface{}{
		"alerts_enabled":  false,
		"auth_required":   true,
		"first_load":      false,
		"server_enabled":  true,
		"server_password": m.pw,
		"server_port":     m.port,
	}

	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	log.Printf("Configured OBS WebSocket in %s", configPath)
	return nil
}

// GetStreamServiceSettings returns the current OBS stream service configuration
// Returns: serviceType (e.g., "rtmp_custom", "rtmp_common"), settings map, error
func (m *Manager) GetStreamServiceSettings() (string, map[string]interface{}, error) {
	respBytes, err := m.sendRequestAndWait("GetStreamServiceSettings", nil)
	if err != nil {
		return "", nil, err
	}

	var res struct {
		D struct {
			ResponseData struct {
				StreamServiceType     string                 `json:"streamServiceType"`
				StreamServiceSettings map[string]interface{} `json:"streamServiceSettings"`
			} `json:"responseData"`
		} `json:"d"`
	}

	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", nil, fmt.Errorf("failed to parse stream service settings: %w", err)
	}

	return res.D.ResponseData.StreamServiceType, res.D.ResponseData.StreamServiceSettings, nil
}

// SetStreamServiceSettings updates the OBS stream service configuration
// serviceType: "rtmp_custom" for custom RTMP, "rtmp_common" for preset services
// settings: map containing "server" and "key" for rtmp_custom, or service-specific fields
func (m *Manager) SetStreamServiceSettings(serviceType string, settings map[string]interface{}) error {
	data := map[string]interface{}{
		"streamServiceType":     serviceType,
		"streamServiceSettings": settings,
	}

	_, err := m.sendRequestAndWait("SetStreamServiceSettings", data)
	if err != nil {
		return fmt.Errorf("failed to set stream service settings: %w", err)
	}

	log.Printf("Set stream service to %s", serviceType)
	return nil
}

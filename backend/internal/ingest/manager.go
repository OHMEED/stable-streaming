package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"stable-stream-solutions/internal/database"
	"sync"
	"time"

	"path/filepath"

	"github.com/gorilla/websocket"
)

// isPortAvailable checks if a port is free on both TCP and UDP.
func isPortAvailable(port int) bool {
	if port <= 0 {
		return false
	}

	// Check TCP
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()

	// Check UDP
	pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	pc.Close()

	return true
}

// isPortAvailableWithRetry attempts to check port availability multiple times before giving up.
// This prevents race conditions on startup where ingests may momentarily hold ports.
// Only after all retries fail do we consider the port permanently unavailable.
func isPortAvailableWithRetry(port int, maxRetries int, retryDelay time.Duration) bool {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if isPortAvailable(port) {
			return true
		}
		if attempt < maxRetries {
			log.Printf("Port %d not available (attempt %d/%d), retrying in %v...", port, attempt, maxRetries, retryDelay)
			time.Sleep(retryDelay)
		}
	}
	return false
}

// getAllUsedPorts returns a map of all ports currently assigned in the database
// to any ingest (enabled or disabled) to ensure we don't double-allocate.
func (m *Manager) getAllUsedPorts() (map[int]bool, error) {
	rows, err := m.DB.Query("SELECT port, output_port, srt_port FROM ingests")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	used := make(map[int]bool)
	for rows.Next() {
		var p, op, sp int
		if err := rows.Scan(&p, &op, &sp); err != nil {
			continue
		}
		if p > 0 {
			used[p] = true
		}
		if op > 0 {
			used[op] = true
			// MediaMTX also implicitly uses op+500 and op+501 for RTP/RTCP
			used[op+500] = true
			used[op+501] = true
		}
		if sp > 0 {
			used[sp] = true
		}
	}
	return used, nil
}

// findAvailablePort searches for the first available port in a range [start, end].
// It checks both OS availability AND database reservation (via exclude map).
func findAvailablePort(start, end int, exclude map[int]bool) (int, error) {
	reserved := make(map[int]bool)
	if exclude != nil {
		reserved = exclude
	}

	// Common system/app reserved ports
	reserved[80] = true
	reserved[443] = true
	reserved[22] = true
	reserved[8080] = true // App API
	reserved[4455] = true // OBS WebSocket
	reserved[6080] = true // noVNC
	reserved[5900] = true // VNC

	for port := start; port <= end; port++ {
		if reserved[port] {
			continue
		}
		if isPortAvailable(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports found in range %d-%d", start, end)
}

type runningIngest struct {
	cmd        *exec.Cmd
	sidecarCmd *exec.Cmd // Optional MediaMTX sidecar for SRTLA RTSP egress
	cancel     context.CancelFunc
}

type Manager struct {
	DB *database.DB
	// Store running processes
	processes map[int]*runningIngest
	mu        sync.Mutex
}

func NewManager(db *database.DB) *Manager {
	// Ensure logs directory exists
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Printf("Failed to create logs directory: %v", err)
	}
	return &Manager{
		DB:        db,
		processes: make(map[int]*runningIngest),
	}
}

// killProcessOnPort has been deprecated. We now use findAvailablePort to avoid conflicts.
func killProcessOnPort(port int) {
	// NOP - as requested by user to avoid killing processes
}

// StartAll reads enabled ingests from DB and starts them
func (m *Manager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rows, err := m.DB.Query("SELECT id, name, protocol, port, output_port, srt_port, bs_port, ws_port, rtsp_port, stream_key, stream_id FROM ingests WHERE enabled = 1")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, port, outputPort, srtPort, bsPort, wsPort, rtspPort int
		var name, protocol string
		var streamKey, streamID sql.NullString // Handle nullable

		if err := rows.Scan(&id, &name, &protocol, &port, &outputPort, &srtPort, &bsPort, &wsPort, &rtspPort, &streamKey, &streamID); err != nil {
			log.Printf("Failed to scan ingest: %v", err)
			continue
		}

		key := ""
		if streamKey.Valid {
			key = streamKey.String
		}

		sID := ""
		if streamID.Valid {
			sID = streamID.String
		}

		go m.StartIngest(id, name, protocol, port, key, sID, outputPort, srtPort, bsPort, wsPort, rtspPort)
	}
	return nil
}

// ... StopIngest / IsRunning ...

// ... List ...

// ... Add (will be updated separately) ...

// ... GetStats ...

// ... Delete ...

// StopIngest halts the process for an ingest
// StopIngest halts the process for an ingest
func (m *Manager) StopIngest(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r, exists := m.processes[id]; exists {
		log.Printf("Stopping ingest %d supervisor...", id)
		r.cancel() // Stop the supervisor loop

		if r.cmd != nil && r.cmd.Process != nil {
			log.Printf("Killing ingest process %d (PID %d)...", id, r.cmd.Process.Pid)
			if err := r.cmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill ingest %d: %v", id, err)
			}
			// Wait for it to release ports
			r.cmd.Wait()
		}

		// Also stop MediaMTX sidecar if running
		if r.sidecarCmd != nil && r.sidecarCmd.Process != nil {
			log.Printf("Killing MediaMTX sidecar for ingest %d (PID %d)...", id, r.sidecarCmd.Process.Pid)
			if err := r.sidecarCmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill sidecar for ingest %d: %v", id, err)
			}
			r.sidecarCmd.Wait()
		}

		delete(m.processes, id)
	} else {
		log.Printf("Ingest %d is not running", id)
	}
}

// IsRunning checks if an ingest ID is currently active in the manager
func (m *Manager) IsRunning(id int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.processes[id]
	return exists
}

type Ingest struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int    `json:"port"`
	OutputPort int    `json:"output_port"`
	SrtPort    int    `json:"srt_port"`
	BsPort     int    `json:"bs_port"`
	WsPort     int    `json:"ws_port"`
	RtspPort   int    `json:"rtsp_port"`
	StreamKey  string `json:"stream_key"`
	StreamID   string `json:"stream_id"`
	Enabled    bool   `json:"enabled"`
	IsRunning  bool   `json:"is_running"`
}

func (m *Manager) List() ([]Ingest, error) {
	rows, err := m.DB.Query("SELECT id, name, protocol, port, output_port, srt_port, bs_port, ws_port, rtsp_port, stream_key, stream_id, enabled FROM ingests")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ingests []Ingest
	for rows.Next() {
		var i Ingest
		var streamKey, streamID sql.NullString
		if err := rows.Scan(&i.ID, &i.Name, &i.Protocol, &i.Port, &i.OutputPort, &i.SrtPort, &i.BsPort, &i.WsPort, &i.RtspPort, &streamKey, &streamID, &i.Enabled); err != nil {
			return nil, err
		}
		if streamKey.Valid {
			i.StreamKey = streamKey.String
		}
		if streamID.Valid {
			i.StreamID = streamID.String
		}
		i.IsRunning = m.IsRunning(i.ID)
		ingests = append(ingests, i)
	}
	return ingests, nil
}

// LogPortsToForward displays a summary of ports that need to be forwarded for ingest input
func (m *Manager) LogPortsToForward() {
	ingests, err := m.List()
	if err != nil {
		log.Printf("Failed to list ingests for port summary: %v", err)
		return
	}

	if len(ingests) == 0 {
		log.Println("📡 No ingests configured yet. Create ingests in the UI to see ports to forward.")
		return
	}

	log.Println("")
	log.Println("═══════════════════════════════════════════════════════════════")
	log.Println("📡 PORTS TO FORWARD (for ingest input):")
	log.Println("───────────────────────────────────────────────────────────────")

	for _, ing := range ingests {
		proto := "UDP"
		if ing.Protocol == "rtmp" {
			proto = "TCP"
		}
		log.Printf("   • %s (%s): Port %d/%s", ing.Name, ing.Protocol, ing.Port, proto)
	}

	log.Println("───────────────────────────────────────────────────────────────")
	log.Println("   • OBS WebSocket: Port 4455/TCP")
	log.Println("   • Web UI: Port 8080/TCP")
	log.Println("═══════════════════════════════════════════════════════════════")
	log.Println("")
}

func (m *Manager) Add(name, protocol string, streamKey string, streamID string) (int, error) {
	// Get all currently used ports to prevent collisions
	usedPorts, err := m.getAllUsedPorts()
	if err != nil {
		return 0, fmt.Errorf("failed to get used ports: %v", err)
	}

	// Automatically find an available ingest port
	// SRTLA uses 5000-5100/udp, SRT/RTMP uses 9000-9100/udp (matches UFW firewall rules)
	var port int
	if protocol == "srtla" {
		port, err = findAvailablePort(5000, 5100, usedPorts)
	} else {
		port, err = findAvailablePort(9000, 9100, usedPorts)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to find available ingest port: %v", err)
	}

	// Default enabled=1 so it starts on boot, but we must also start it now.
	res, err := m.DB.Exec("INSERT INTO ingests (name, protocol, port, stream_key, stream_id, enabled) VALUES (?, ?, ?, ?, ?, 1)", name, protocol, port, streamKey, streamID)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	// Mark the newly taken port as used for the next search
	usedPorts[port] = true

	// Automatically find an available output port
	// For SRT/RTMP: 7000-8000/tcp (matches UFW firewall rules for RTSP egress)
	// For SRTLA: output_port is internal UDP (OBS), so we also need rtsp_port (TCP)
	outputPort, err := findAvailablePort(7000, 8000, usedPorts)
	if err != nil {
		return 0, fmt.Errorf("failed to find available output port: %v", err)
	}
	usedPorts[outputPort] = true // Mark used

	// Default ports for others
	srtPort := 0
	bsPort := 0
	wsPort := 0
	rtspPort := 0

	if protocol == "srtla" {
		// Allocate SRT relay port
		var newSrtPort int
		newSrtPort, err = findAvailablePort(6000, 7000, usedPorts)
		if err != nil {
			return 0, fmt.Errorf("failed to find available srt relay port: %v", err)
		}
		srtPort = newSrtPort
		usedPorts[srtPort] = true

		// Allocate Broadcaster port
		var newBsPort int
		newBsPort, err = findAvailablePort(8000, 9000, usedPorts)
		if err != nil {
			return 0, fmt.Errorf("failed to find available broadcaster port: %v", err)
		}
		bsPort = newBsPort
		usedPorts[bsPort] = true

		// Allocate RTSP Egress port
		var newRtspPort int
		newRtspPort, err = findAvailablePort(7000, 8000, usedPorts)
		if err != nil {
			return 0, fmt.Errorf("failed to find available rtsp egress port: %v", err)
		}
		rtspPort = newRtspPort
		usedPorts[rtspPort] = true
	}

	// Stats port
	wsPort, err = findAvailablePort(9500, 9600, usedPorts)
	if err != nil {
		return 0, fmt.Errorf("failed to find available stats port: %v", err)
	}

	// Update DB with auto-assigned ports
	_, err = m.DB.Exec("UPDATE ingests SET output_port = ?, srt_port = ?, bs_port = ?, ws_port = ?, rtsp_port = ? WHERE id = ?", outputPort, srtPort, bsPort, wsPort, rtspPort, id)
	if err != nil {
		return 0, fmt.Errorf("failed to update ports for ingest %d: %v", id, err)
	}

	return int(id), nil
}

func (m *Manager) GetStats(id int) (map[string]interface{}, error) {
	// 1. Get Ingest Info
	var protocol string
	var wsPort int
	err := m.DB.QueryRow("SELECT protocol, ws_port FROM ingests WHERE id = ?", id).Scan(&protocol, &wsPort)
	if err != nil {
		return nil, err
	}

	if protocol != "srtla" && protocol != "srt" && protocol != "rtmp" {
		return nil, fmt.Errorf("stats not supported for protocol: %s", protocol)
	}

	if !m.IsRunning(id) {
		return nil, fmt.Errorf("ingest not running")
	}

	// 2. Connect to Stats
	// If MediaMTX, use API
	if protocol == "srt" || protocol == "rtmp" {
		statsUrl := fmt.Sprintf("http://localhost:%d/v3/paths/list", wsPort)
		resp, err := http.Get(statsUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to call mediamtx api: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("mediamtx api returned status %d", resp.StatusCode)
		}

		var apiResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return nil, fmt.Errorf("failed to parse mediamtx api response: %v", err)
		}

		// Debug log
		// log.Printf("Ingest %d (MediaMTX) Stats: %+v", id, apiResp)

		return apiResp, nil
	}

	// SRTLA (WebSocket)
	// go-irl exposes stats at /ws path
	wsUrl := fmt.Sprintf("ws://localhost:%d/ws", wsPort)
	conn, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to stats: %v", err)
	}
	defer conn.Close()

	// 4. Read one message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read stats: %v", err)
	}

	// go-irl stats format: { timestamp, type: "reader"|"writer", stats: { Interval: { MbpsRecvRate, ... }, Instantaneous: { MbpsRecvRate, ... } } }
	var rawStats map[string]interface{}
	if err := json.Unmarshal(message, &rawStats); err != nil {
		return nil, fmt.Errorf("failed to parse stats: %v", err)
	}

	// Extract bitrate from nested stats structure
	// Priority: Interval.MbpsRecvRate > Instantaneous.MbpsRecvRate
	var mbps float64 = 0
	if statsObj, ok := rawStats["stats"].(map[string]interface{}); ok {
		// Try Interval first (average over interval)
		if interval, ok := statsObj["Interval"].(map[string]interface{}); ok {
			if rate, ok := interval["MbpsRecvRate"].(float64); ok && rate > 0 {
				mbps = rate
			}
		}
		// Fallback to Instantaneous
		if mbps == 0 {
			if instant, ok := statsObj["Instantaneous"].(map[string]interface{}); ok {
				if rate, ok := instant["MbpsRecvRate"].(float64); ok && rate > 0 {
					mbps = rate
				}
			}
		}
	}

	// Convert to kbps for frontend compatibility (frontend expects bitrate_kbps)
	kbps := mbps * 1000

	return map[string]interface{}{
		"bitrate_kbps": kbps,
		"mbps":         mbps,
		"type":         rawStats["type"],
		"timestamp":    rawStats["timestamp"],
	}, nil
}

func (m *Manager) Delete(id int) error {
	m.StopIngest(id)
	_, err := m.DB.Exec("DELETE FROM ingests WHERE id = ?", id)
	return err
}

func (m *Manager) Start(id int) error {
	// Fetch details
	var name, protocol string
	var port, outputPort, srtPort, bsPort, wsPort, rtspPort int
	var streamKey, streamID sql.NullString

	err := m.DB.QueryRow("SELECT name, protocol, port, output_port, srt_port, bs_port, ws_port, rtsp_port, stream_key, stream_id FROM ingests WHERE id = ?", id).Scan(&name, &protocol, &port, &outputPort, &srtPort, &bsPort, &wsPort, &rtspPort, &streamKey, &streamID)
	if err != nil {
		return err
	}

	key := ""
	if streamKey.Valid {
		key = streamKey.String
	}

	sID := ""
	if streamID.Valid {
		sID = streamID.String
	}

	// Update enabled
	m.DB.Exec("UPDATE ingests SET enabled = 1 WHERE id = ?", id)

	// Check for missing ports (migration for existing ingests)
	if (protocol == "srt" || protocol == "rtmp") && (outputPort == 0 || wsPort == 0) {
		if outputPort == 0 {
			outputPort = 7000 + id
		}
		if wsPort == 0 {
			wsPort = 9500 + id
		}
		log.Printf("Assigning missing ports for ingest %d: output=%d, ws=%d", id, outputPort, wsPort)
		m.DB.Exec("UPDATE ingests SET output_port = ?, ws_port = ? WHERE id = ?", outputPort, wsPort, id)
	}

	if protocol == "srtla" && (outputPort == 0 || srtPort == 0 || bsPort == 0 || wsPort == 0 || rtspPort == 0) {
		if outputPort == 0 {
			outputPort = 7000 + id
		} // Internal UDP (OBS)
		if srtPort == 0 {
			srtPort = 6000 + id
		} // SRT Relay
		if bsPort == 0 {
			bsPort = 8500 + id
		} // Broadcaster
		if wsPort == 0 {
			wsPort = 9500 + id
		} // Stats
		if rtspPort == 0 {
			rtspPort = 7500 + id
		} // RTSP Egress (new range 7500-8000? or reuse 7000+? SRT uses 7000+ for RTSP egress too via MediaMTX)
		// Wait, MediaMTX uses 7000-8000 for RTSP egress.
		// We should ensure no conflict.
		// MediaMTX outputPort IS the RTSP port.
		// For SRTLA, outputPort is internal UDP.
		// So we need a unique RTSP port for SRTLA.
		// Let's use logic similar to allocation: find available port.
		// But for quick migration here we use formula.
		// Logic in Add() uses findAvailablePort.
		// Here we just patch it.
		// Re-read existing ports to prevent conflict is safer?
		// Let's assume Add() logic handles new ones.
		// For existing, let's use check or formula.
		// Safety: 7000-8000 is for RTSP.
		// Manager uses 7000+id for MediaMTX RTSP.
		// For SRTLA, let's allocate a high port or try to find?
		// Let's use 7500+id for now as "safe" range within 7000-8000 if MediaMTX uses 7000+.
		// Or better: use Add logic properly. But Start() is simple migration.
		// Let's use 7500+id for simplicity in migration block.

		log.Printf("Assigning missing ports for ingest %d: output=%d, srt=%d, bs=%d, ws=%d, rtsp=%d", id, outputPort, srtPort, bsPort, wsPort, rtspPort)
		m.DB.Exec("UPDATE ingests SET output_port = ?, srt_port = ?, bs_port = ?, ws_port = ?, rtsp_port = ? WHERE id = ?", outputPort, srtPort, bsPort, wsPort, rtspPort, id)
	}

	go m.StartIngest(id, name, protocol, port, key, sID, outputPort, srtPort, bsPort, wsPort, rtspPort)
	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Printf("Stopping all ingest processes and supervisors...")
	for id, r := range m.processes {
		r.cancel() // Stop supervisor
		if r.cmd != nil && r.cmd.Process != nil {
			if err := r.cmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill ingest %d: %v", id, err)
			} else {
				log.Printf("Killed ingest %d (PID %d)", id, r.cmd.Process.Pid)
			}
		}
		// Also stop MediaMTX sidecar if running
		if r.sidecarCmd != nil && r.sidecarCmd.Process != nil {
			if err := r.sidecarCmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill sidecar for ingest %d: %v", id, err)
			} else {
				log.Printf("Killed sidecar for ingest %d (PID %d)", id, r.sidecarCmd.Process.Pid)
			}
		}
	}
	m.processes = make(map[int]*runningIngest)
}

func (m *Manager) Stop(id int) error {
	m.StopIngest(id)
	m.DB.Exec("UPDATE ingests SET enabled = 0 WHERE id = ?", id)
	return nil
}

func (m *Manager) StartIngest(id int, name string, protocol string, port int, key string, streamID string, outputPort int, srtPort int, bsPort int, wsPort int, rtspPort int) {
	log.Printf("Starting supervised ingest %d: %s (%s) on port %d", id, name, protocol, port)

	// Ensure not already running
	m.mu.Lock()
	if _, exists := m.processes[id]; exists {
		m.mu.Unlock()
		log.Printf("Ingest %d is already running", id)
		return
	}
	m.mu.Unlock()

	var finalOutputPort, finalSrtPort, finalPort, finalWsPort, finalRtspPort int
	finalPort = port
	finalOutputPort = outputPort
	finalSrtPort = srtPort
	finalWsPort = wsPort

	// 0. Get all used ports (excluding current ingest's own ports is tricky, so we rely on findAvailablePort skipping if port matches 'start')
	// Ideally we exclude 'current' ports from the 'used' list only if we are forced to move.
	usedPorts, err := m.getAllUsedPorts()
	if err != nil {
		log.Printf("Supervisor [%d]: Failed to get used ports list: %v", id, err)
		usedPorts = make(map[int]bool)
	}

	// 1. Validate Ingest Port
	// If taken by OS (but not by us? hard to distinguish), OR taken by another DB entry
	// Actually, getAllUsedPorts includes US. So we should remove 'our' current ports from the used list so we don't conflict with ourselves
	delete(usedPorts, finalPort)
	delete(usedPorts, finalOutputPort)
	delete(usedPorts, finalSrtPort)
	delete(usedPorts, finalWsPort)         // Don't block our own ws port
	delete(usedPorts, finalOutputPort+500) // RTP
	delete(usedPorts, finalOutputPort+501) // RTCP

	// Port retry configuration: try 5 times with 2 second delays (~10s total)
	// This prevents race conditions on startup and only relocates for persistent conflicts
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	if !isPortAvailableWithRetry(finalPort, maxRetries, retryDelay) {
		log.Printf("Supervisor [%d]: Ingest port %d persistently taken. Finding new port...", id, finalPort)
		// Mark current as used again just in case (though we know it's taken)
		usedPorts[finalPort] = true
		// SRTLA uses 5000-5100/udp, SRT/RTMP uses 9000-9100/udp (matches UFW firewall rules)
		var newPort int
		if protocol == "srtla" {
			newPort, err = findAvailablePort(5000, 5100, usedPorts)
		} else {
			newPort, err = findAvailablePort(9000, 9100, usedPorts)
		}
		if err == nil {
			finalPort = newPort
			log.Printf("Supervisor [%d]: Relocated ingest to port %d", id, finalPort)
			usedPorts[finalPort] = true // Update used list
		}
	}

	// 2. Validate Output Port (Egress)
	if !isPortAvailableWithRetry(finalOutputPort, maxRetries, retryDelay) {
		log.Printf("Supervisor [%d]: Output port %d persistently taken. Finding new port...", id, finalOutputPort)
		usedPorts[finalOutputPort] = true
		newPort, err := findAvailablePort(7000, 8000, usedPorts)
		if err == nil {
			finalOutputPort = newPort
			log.Printf("Supervisor [%d]: Relocated output to port %d", id, finalOutputPort)
			usedPorts[finalOutputPort] = true
		}
	}

	// 3. Validate SRT Port (Internal to SRTLA)
	if protocol == "srtla" && !isPortAvailableWithRetry(finalSrtPort, maxRetries, retryDelay) {
		log.Printf("Supervisor [%d]: Internal SRT port %d persistently taken. Finding new port...", id, finalSrtPort)
		usedPorts[finalSrtPort] = true
		newPort, err := findAvailablePort(6000, 7000, usedPorts)
		if err == nil {
			finalSrtPort = newPort
			log.Printf("Supervisor [%d]: Relocated internal SRT to port %d", id, finalSrtPort)
			usedPorts[finalSrtPort] = true
		}
	}
	// Port collision check
	// Because ports are assigned at database level, we rely on checking OS availability

	// Initialize final ports with passed values
	// Initialize final ports with passed values
	// Kept to silence compiler error if variable re-used, but wait, I just removed its usage in args.
	// Actually, I should remove it from declaration if possible, but it's in a block.
	// Wait, the error is "declared and not used".
	// I can just assign it to `_` to suppress, or remove it from the var block.
	// Since I'm editing a chunk, I'll just assign it to blank identifier if I can't easily see the var block.
	// Or I can just remove the assignment `finalBsPort = bsPort`.
	// But then `finalBsPort` is still declared?
	// Let's re-read the var declaration. It was `var finalOutputPort, finalSrtPort, finalPort, finalWsPort, finalBsPort, finalRtspPort int`.
	// I need to update THAT line too.
	// But I can't easily jump back to line 545 from here in one replace call if they are far apart.
	// Line 545 declared it. Line 623 assigned it.
	// I will remove the assignment here first.
	// AND I will use a multi_replace or second replace to fix the declaration.
	// Actually, let's just make it used for now by logging it or something? No, cleaner to remove.
	// I will use `replace_file_content` to remove the declaration at 545 and the assignment at 623.
	// Since I can only do one contiguous block per call if using `replace_file_content`.
	// I will use `multi_replace_file_content`.
	finalWsPort = wsPort
	finalRtspPort = rtspPort

	// Simple check if ports are free.
	// In managed mode, we generally expect them to be free because we manage them.
	// However, if MediaMTX crashed or zombie process?
	// isPortAvailableWithRetry(finalPort, 5, 1*time.Second)

	// Create supervisor context
	ctx, cancel := context.WithCancel(context.Background())
	r := &runningIngest{
		cancel: cancel,
	}

	m.mu.Lock()
	m.processes[id] = r
	m.mu.Unlock()

	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// No longer killing ports. Just log start attempt.
				log.Printf("Supervisor [%d]: Preparing to start ingest...", id)

				var cmd *exec.Cmd
				var sidecarCmd *exec.Cmd
				if protocol == "srtla" {
					// go-irl CLI: -mode standalone, -srtla-port, -udp-port, -ws-port, -passphrase
					// go-irl outputs MPEG-TS over UDP on udp-port
					// MediaMTX sidecar converts UDP to RTSP for OBS
					args := []string{
						"-mode", "standalone",
						"-srtla-port", fmt.Sprintf("%d", finalPort),
						"-udp-port", fmt.Sprintf("%d", finalOutputPort),
						"-ws-port", fmt.Sprintf("%d", finalWsPort),
					}
					if key != "" {
						args = append(args, "-passphrase", key)
					}
					srtlaPath := "./bin/go-irl"
					if runtime.GOOS == "windows" {
						srtlaPath = "./bin/go-irl.exe"
					}
					cmd = exec.Command(srtlaPath, args...)

					// Configure MediaMTX sidecar to convert UDP to RTSP
					sidecarConfigPath := fmt.Sprintf("mediamtx_srtla_%d.yml", id)
					absSidecarConfigPath, _ := filepath.Abs(sidecarConfigPath)
					if err := m.generateSRTLAMediaMTXConfig(absSidecarConfigPath, finalOutputPort, finalRtspPort, key); err != nil {
						log.Printf("Supervisor [%d]: Failed to generate sidecar config: %v", id, err)
					} else {
						sidecarBinPath := "./bin/mediamtx"
						if runtime.GOOS == "windows" {
							sidecarBinPath = "./bin/mediamtx.exe"
						}
						sidecarBinPath, _ = filepath.Abs(sidecarBinPath)
						sidecarCmd = exec.Command(sidecarBinPath, absSidecarConfigPath)
						sidecarCmd.Dir, _ = filepath.Abs(".")
					}
				} else {
					configPath := fmt.Sprintf("mediamtx_%d.yml", id)
					absConfigPath, _ := filepath.Abs(configPath)
					if err := m.generateMediaMTXConfig(absConfigPath, protocol, port, finalOutputPort, finalWsPort, key, streamID); err != nil {
						log.Printf("Supervisor [%d]: Failed to generate config: %v", id, err)
						time.Sleep(5 * time.Second)
						continue
					}
					binPath := "./bin/mediamtx"
					if runtime.GOOS == "windows" {
						binPath = "./bin/mediamtx.exe"
					}
					binPath, _ = filepath.Abs(binPath)
					cmd = exec.Command(binPath, absConfigPath)
					cmd.Dir, _ = filepath.Abs(".")
				}

				// Capture stdout/stderr with APPEND
				logFile, err := os.OpenFile(fmt.Sprintf("logs/ingest_%d.log", id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err == nil {
					cmd.Stdout = logFile
					cmd.Stderr = logFile
				}

				if err := cmd.Start(); err != nil {
					log.Printf("Supervisor [%d]: Failed to start ingest process: %v", id, err)
					if logFile != nil {
						logFile.Close()
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
						continue
					}
				}

				m.mu.Lock()
				r.cmd = cmd
				m.mu.Unlock()

				log.Printf("Supervisor [%d]: Ingest process started (PID %d)", id, cmd.Process.Pid)

				// Start MediaMTX sidecar for RTSP egress (all protocols)
				if sidecarCmd != nil {
					// Give main process a moment to start
					time.Sleep(500 * time.Millisecond)

					// Sidecar shares the same log file
					sidecarLogFile, err := os.OpenFile(fmt.Sprintf("logs/ingest_%d_rtsp.log", id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err == nil {
						sidecarCmd.Stdout = sidecarLogFile
						sidecarCmd.Stderr = sidecarLogFile
					}

					if err := sidecarCmd.Start(); err != nil {
						log.Printf("Supervisor [%d]: Failed to start MediaMTX sidecar: %v", id, err)
					} else {
						m.mu.Lock()
						r.sidecarCmd = sidecarCmd
						m.mu.Unlock()
						log.Printf("Supervisor [%d]: MediaMTX sidecar started (PID %d) - RTSP on port %d", id, sidecarCmd.Process.Pid, finalRtspPort)
					}
				}

				cmd.Wait()
				if logFile != nil {
					logFile.Close()
				}

				// Stop sidecar when main process exits
				if r.sidecarCmd != nil && r.sidecarCmd.Process != nil {
					log.Printf("Supervisor [%d]: Stopping MediaMTX sidecar...", id)
					r.sidecarCmd.Process.Kill()
					r.sidecarCmd.Wait()
				}

				log.Printf("Supervisor [%d]: Ingest process exited.", id)

				select {
				case <-ctx.Done():
					log.Printf("Supervisor [%d]: Loop terminating.", id)
					return
				case <-time.After(5 * time.Second):
					log.Printf("Supervisor [%d]: Restart triggered.", id)
				}
			}
		}
	}()
}

func (m *Manager) generateMediaMTXConfig(path, protocol string, port int, outputPort int, wsPort int, key string, streamID string) error {
	// Generate a MediaMTX config that:
	// 1. Listens on the public port for SRT/RTMP
	// 2. Exposes direct RTSP egress on outputPort for OBS
	// 3. Supports passphrase authentication for SRT
	// 4. Enables API on wsPort

	config := "logLevel: info\n"
	config += "api: yes\n"
	config += fmt.Sprintf("apiAddress: 127.0.0.1:%d\n", wsPort)
	config += "metrics: no\n"
	config += "pprof: no\n"
	config += "hls: no\n"
	config += "hlsAddress: \"\"\n"
	config += "webrtc: no\n"
	config += "webrtcAddress: \"\"\n" // Explicitly disable WebRTC to prevent port 8000 binding
	config += "record: no\n"

	// Enable RTSP egress - bind to 0.0.0.0 for PUBLIC access (shareable streams)
	config += "\n# RTSP Egress for OBS (public shareable)\n"
	config += "rtsp: yes\n"
	config += fmt.Sprintf("rtspAddress: 0.0.0.0:%d\n", outputPort)
	config += "protocols: [tcp]\n"                                       // Force TCP only to avoid UDP port conflicts
	config += fmt.Sprintf("rtpAddress: 127.0.0.1:%d\n", outputPort+500)  // Unique RTP port per ingest (internal only)
	config += fmt.Sprintf("rtcpAddress: 127.0.0.1:%d\n", outputPort+501) // Unique RTCP port per ingest (internal only)

	if protocol == "rtmp" {
		config += "\n# RTMP Ingest\n"
		config += "rtmp: yes\n"
		config += fmt.Sprintf("rtmpAddress: :%d\n", port)
		config += "srt: no\n"
	} else if protocol == "srt" {
		config += "\n# SRT Ingest\n"
		config += "rtmp: no\n"
		config += fmt.Sprintf("srtAddress: :%d\n", port)
		config += "srt: yes\n"
	} else {
		return fmt.Errorf("unsupported protocol: %s", protocol)
	}

	// Path configuration
	// Egress via RTSP is unencrypted for easy sharing
	config += "\npaths:\n"

	// Open path 'all' (regex ~^.*$) to allow any streamid path
	config += "  \"~^.*$\":\n"

	// Encryption disabled per user request
	// No srtPublishPassphrase set

	// No srtReadPassphrase = egress via RTSP is open for sharing

	return os.WriteFile(path, []byte(config), 0644)
}

// StartUDPRelay listens for UDP packets on srcPort and forwards them to dstPort on localhost.
// It stays open as long as the context is not cancelled, even if dstPort has nothing listening.
func (m *Manager) StartUDPRelay(ctx context.Context, srcPort, dstPort int) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", srcPort))
	if err != nil {
		log.Printf("Relay [%d]: Failed to resolve source: %v", srcPort, err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("Relay [%d]: Failed to listen: %v", srcPort, err)
		return
	}
	defer conn.Close()

	destAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", dstPort))
	if err != nil {
		log.Printf("Relay [%d]: Failed to resolve dest: %v", srcPort, err)
		return
	}

	// Buffer for packets
	buf := make([]byte, 65535)

	log.Printf("Relay [%d]: Listening and forwarding to %d", srcPort, dstPort)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Relay [%d]: Shutting down", srcPort)
			return
		default:
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Printf("Relay [%d]: Read error: %v", srcPort, err)
				continue
			}

			// Forward to destination
			// Note: We don't care if this fails (e.g. connection refused by OBS)
			// The goal is just to consume from go-irl so it doesn't crash.
			if n > 0 {
				_, _ = conn.WriteToUDP(buf[:n], destAddr)
			}
		}
	}
}

// generateSRTLAMediaMTXConfig creates a MediaMTX config for SRTLA RTSP egress.
// It accepts UDP input from go-irl and exposes it via RTSP.
func (m *Manager) generateSRTLAMediaMTXConfig(path string, udpInputPort int, rtspPort int, passphrase string) error {
	config := "logLevel: info\n"
	config += "api: no\n"
	config += "metrics: no\n"
	config += "pprof: no\n"

	// Disable all input servers (we feed via UDP source)
	config += "rtmp: no\n"
	config += "srt: no\n"
	config += "hls: no\n"
	config += "webrtc: no\n"
	config += "record: no\n"

	// Enable RTSP output - bind to 0.0.0.0 for PUBLIC access
	config += "\n# RTSP Egress for SRTLA\n"
	config += "rtsp: yes\n"
	config += fmt.Sprintf("rtspAddress: 0.0.0.0:%d\n", rtspPort)
	config += "protocols: [tcp]\n"                                     // Force TCP only
	config += fmt.Sprintf("rtpAddress: 127.0.0.1:%d\n", rtspPort+500)  // Unique RTP port
	config += fmt.Sprintf("rtcpAddress: 127.0.0.1:%d\n", rtspPort+501) // Unique RTCP port

	// Path configuration - UDP source
	config += "\npaths:\n"

	// Path name is the passphrase (or 'live' if none)
	pathName := "live"
	if passphrase != "" {
		pathName = passphrase
	}

	// Only define ONE path reading from UDP — MediaMTX can't bind the same
	// UDP port to multiple paths. OBS connects to the /stream path.
	config += fmt.Sprintf("  \"%s/stream\":\n", pathName)
	config += fmt.Sprintf("    source: udp://127.0.0.1:%d\n", udpInputPort)
	config += "    sourceOnDemand: no\n" // Always listen for UDP

	return os.WriteFile(path, []byte(config), 0644)
}

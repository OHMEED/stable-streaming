package applog

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	logFile *os.File
	mu      sync.Mutex
	logPath = "logs/app.log"
)

// Init initializes the application logger
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	// Ensure logs directory exists
	if err := os.MkdirAll("logs", 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	logFile = f

	// Write initial log entry directly (avoid calling Log which would deadlock)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [SYSTEM] Application logger initialized\n", timestamp)
	logFile.WriteString(entry)

	return nil
}

// Log writes a timestamped log entry
func Log(category, message string) {
	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [%s] %s\n", timestamp, category, message)
	logFile.WriteString(entry)
}

// LogAuth logs authentication events
func LogAuth(success bool, ip, reason string) {
	if success {
		Log("AUTH", fmt.Sprintf("Login successful from %s", ip))
	} else {
		Log("AUTH", fmt.Sprintf("Login failed from %s: %s", ip, reason))
	}
}

// LogIngest logs ingest-related events
func LogIngest(action, name string, id int, protocol string) {
	switch action {
	case "create":
		Log("INGEST", fmt.Sprintf("Created: %s (ID: %d, Protocol: %s)", name, id, protocol))
	case "delete":
		Log("INGEST", fmt.Sprintf("Deleted: ID %d", id))
	case "start":
		Log("INGEST", fmt.Sprintf("Started: %s (ID: %d)", name, id))
	case "stop":
		Log("INGEST", fmt.Sprintf("Stopped: %s (ID: %d)", name, id))
	}
}

// LogOBS logs OBS-related events
func LogOBS(action string) {
	switch action {
	case "stream_toggle":
		Log("OBS", "Stream toggled")
	case "record_toggle":
		Log("OBS", "Recording toggled")
	case "connected":
		Log("OBS", "WebSocket connected")
	case "disconnected":
		Log("OBS", "WebSocket disconnected")
	}
}

// LogWS logs WebSocket events
func LogWS(endpoint, event string) {
	Log("WS", fmt.Sprintf("%s: %s", endpoint, event))
}

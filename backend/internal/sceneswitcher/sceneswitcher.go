package sceneswitcher

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"stable-stream-solutions/internal/database"
	"stable-stream-solutions/internal/ingest"
	"stable-stream-solutions/internal/obs"
)

// Config holds the scene switcher configuration
type Config struct {
	IngestID      int    `json:"ingest_id"`
	OnlineScene   string `json:"online_scene"`
	OfflineScene  string `json:"offline_scene"`
	OnlyOnScene   string `json:"only_on_scene"` // Optional: only switch when on this scene
	ThresholdKbps int    `json:"threshold_kbps"`
	Enabled       bool   `json:"enabled"`
}

// SceneSwitcher monitors ingest bitrate and switches OBS scenes
type SceneSwitcher struct {
	db            *database.DB
	ingestManager *ingest.Manager
	obsManager    *obs.Manager

	mu            sync.Mutex
	running       bool
	stopChan      chan struct{}
	currentState  string // "online" or "offline"
	lastSwitch    time.Time
	debounceDelay time.Duration
}

// NewSceneSwitcher creates a new scene switcher instance
func NewSceneSwitcher(db *database.DB, ingestMgr *ingest.Manager, obsMgr *obs.Manager) *SceneSwitcher {
	return &SceneSwitcher{
		db:            db,
		ingestManager: ingestMgr,
		obsManager:    obsMgr,
		debounceDelay: 3 * time.Second, // Prevent rapid switching
	}
}

// GetConfig retrieves the current configuration from the database
func (s *SceneSwitcher) GetConfig() (*Config, error) {
	config := &Config{
		ThresholdKbps: 1000, // Default
	}

	row := s.db.QueryRow(`
		SELECT ingest_id, online_scene, offline_scene, threshold_kbps, enabled, only_on_scene 
		FROM auto_scene_switcher WHERE id = 1
	`)

	var ingestID sql.NullInt64
	var onlineScene, offlineScene, onlyOnScene sql.NullString
	var thresholdKbps sql.NullInt64
	var enabled sql.NullBool

	err := row.Scan(&ingestID, &onlineScene, &offlineScene, &thresholdKbps, &enabled, &onlyOnScene)
	if err == sql.ErrNoRows {
		// No config exists yet, return defaults
		return config, nil
	}
	if err != nil {
		return nil, err
	}

	if ingestID.Valid {
		config.IngestID = int(ingestID.Int64)
	}
	if onlineScene.Valid {
		config.OnlineScene = onlineScene.String
	}
	if offlineScene.Valid {
		config.OfflineScene = offlineScene.String
	}
	if onlyOnScene.Valid {
		config.OnlyOnScene = onlyOnScene.String
	}
	if thresholdKbps.Valid {
		config.ThresholdKbps = int(thresholdKbps.Int64)
	}
	if enabled.Valid {
		config.Enabled = enabled.Bool
	}

	return config, nil
}

// SaveConfig saves the configuration to the database
func (s *SceneSwitcher) SaveConfig(config *Config) error {
	// Use UPSERT pattern (INSERT OR REPLACE)
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO auto_scene_switcher 
		(id, ingest_id, online_scene, offline_scene, threshold_kbps, enabled, only_on_scene)
		VALUES (1, ?, ?, ?, ?, ?, ?)
	`, config.IngestID, config.OnlineScene, config.OfflineScene, config.ThresholdKbps, config.Enabled, config.OnlyOnScene)

	if err != nil {
		return err
	}

	// Restart monitoring if state changed
	s.mu.Lock()
	wasRunning := s.running
	s.mu.Unlock()

	if config.Enabled && !wasRunning {
		s.Start()
	} else if !config.Enabled && wasRunning {
		s.Stop()
	} else if config.Enabled && wasRunning {
		// Config changed while running, restart
		s.Stop()
		s.Start()
	}

	return nil
}

// SetEnabled enables or disables the scene switcher
func (s *SceneSwitcher) SetEnabled(enabled bool) error {
	_, err := s.db.Exec(`
		UPDATE auto_scene_switcher SET enabled = ? WHERE id = 1
	`, enabled)

	if err != nil {
		return err
	}

	if enabled {
		s.Start()
	} else {
		s.Stop()
	}

	return nil
}

// Start begins monitoring the ingest bitrate
func (s *SceneSwitcher) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.currentState = "" // Unknown initial state
	s.mu.Unlock()

	log.Println("[SceneSwitcher] Starting bitrate monitoring")

	go s.monitorLoop()
}

// Stop halts the monitoring goroutine
func (s *SceneSwitcher) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	log.Println("[SceneSwitcher] Stopped bitrate monitoring")
}

// IsRunning returns whether the switcher is actively monitoring
func (s *SceneSwitcher) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// monitorLoop continuously checks bitrate and switches scenes
func (s *SceneSwitcher) monitorLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkAndSwitch()
		}
	}
}

// checkAndSwitch checks bitrate and switches scenes if needed
func (s *SceneSwitcher) checkAndSwitch() {
	config, err := s.GetConfig()
	if err != nil {
		log.Printf("[SceneSwitcher] Error loading config: %v", err)
		return
	}

	if !config.Enabled || config.IngestID == 0 {
		return
	}

	// Check if OBS is connected
	if !s.obsManager.IsConnected() {
		log.Println("[SceneSwitcher] OBS not connected, skipping check")
		return
	}

	// Check "Only when On Scene" constraint
	if config.OnlyOnScene != "" {
		currentScene, err := s.obsManager.GetCurrentProgramScene()
		if err != nil {
			log.Printf("[SceneSwitcher] Error getting current scene: %v", err)
			return
		}
		// Only proceed if we're on the Online or Offline scene (or the specified scene)
		// This allows switching between Online and Offline, but not from other scenes
		if currentScene != config.OnlyOnScene && currentScene != config.OnlineScene && currentScene != config.OfflineScene {
			// User is on a different scene, don't interfere
			return
		}
	}

	// Get ingest stats
	stats, err := s.ingestManager.GetStats(config.IngestID)
	if err != nil {
		// Ingest not running or error - treat as offline
		s.switchToState("offline", config)
		return
	}

	// Extract bitrate in kbps
	bitrateKbps := s.extractBitrateKbps(stats)

	// Determine target state
	var targetState string
	if bitrateKbps > 0 && bitrateKbps >= float64(config.ThresholdKbps) {
		targetState = "online"
	} else {
		targetState = "offline"
	}

	s.switchToState(targetState, config)
}

// extractBitrateKbps extracts bitrate from stats in kbps
func (s *SceneSwitcher) extractBitrateKbps(stats map[string]interface{}) float64 {
	// SRTLA format: bitrate_kbps directly
	if kbps, ok := stats["bitrate_kbps"].(float64); ok {
		return kbps
	}
	if kbps, ok := stats["kbps"].(float64); ok {
		return kbps
	}
	// Some may return as int
	if kbps, ok := stats["bitrate_kbps"].(int); ok {
		return float64(kbps)
	}

	// MediaMTX format: need to calculate from bytesReceived
	// This is handled differently - we get itemCount and items
	if itemCount, ok := stats["itemCount"].(float64); ok && itemCount > 0 {
		if items, ok := stats["items"].([]interface{}); ok && len(items) > 0 {
			if item, ok := items[0].(map[string]interface{}); ok {
				if ready, ok := item["ready"].(bool); ok && ready {
					// MediaMTX is receiving, but we can't calculate exact bitrate here
					// without tracking bytesReceived over time. Return a high value
					// to indicate "receiving" state
					return 10000 // Placeholder: assume receiving = above threshold
				}
			}
		}
	}

	return 0
}

// switchToState switches to the specified state if different from current
func (s *SceneSwitcher) switchToState(targetState string, config *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check debounce
	if time.Since(s.lastSwitch) < s.debounceDelay {
		return
	}

	// Check if state actually changed
	if s.currentState == targetState {
		return
	}

	var sceneName string
	if targetState == "online" {
		sceneName = config.OnlineScene
	} else {
		sceneName = config.OfflineScene
	}

	if sceneName == "" {
		log.Printf("[SceneSwitcher] No scene configured for state: %s", targetState)
		return
	}

	log.Printf("[SceneSwitcher] Switching to %s scene: %s", targetState, sceneName)

	err := s.obsManager.SetScene(sceneName)
	if err != nil {
		log.Printf("[SceneSwitcher] Error switching scene: %v", err)
		return
	}

	s.currentState = targetState
	s.lastSwitch = time.Now()
	log.Printf("[SceneSwitcher] Successfully switched to %s", sceneName)
}

// StartIfEnabled checks config and starts monitoring if enabled
func (s *SceneSwitcher) StartIfEnabled() {
	config, err := s.GetConfig()
	if err != nil {
		log.Printf("[SceneSwitcher] Error loading config on startup: %v", err)
		return
	}

	if config.Enabled {
		log.Println("[SceneSwitcher] Config is enabled, starting monitoring")
		s.Start()
	}
}

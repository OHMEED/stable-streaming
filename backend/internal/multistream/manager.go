package multistream

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"stable-stream-solutions/internal/database"
	"stable-stream-solutions/internal/obs"
	"strings"
	"sync"
)

// Manager handles multi-streaming configuration and nginx-rtmp control
type Manager struct {
	db         *database.DB
	obsManager *obs.Manager
	maxDests   int
	available  bool
	mu         sync.RWMutex
}

// Destination represents a streaming destination
type Destination struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	RTMPURL   string `json:"rtmp_url"`
	StreamKey string `json:"stream_key"`
	Enabled   bool   `json:"enabled"`
}

// Config represents the multistream configuration
type Config struct {
	Enabled                 bool   `json:"enabled"`
	MaxDestinations         int    `json:"max_destinations"`
	OriginalServiceType     string `json:"original_service_type,omitempty"`
	OriginalServiceSettings string `json:"original_service_settings,omitempty"`
	Available               bool   `json:"available"` // Set by startup flag
}

// NewManager creates a new multistream manager
// maxDests of 0 means multistream is not available
func NewManager(db *database.DB, obsManager *obs.Manager, maxDests int) *Manager {
	m := &Manager{
		db:         db,
		obsManager: obsManager,
		maxDests:   maxDests,
		available:  maxDests > 0,
	}

	// Initialize config row if multistream is available
	if m.available {
		m.initConfig()
	}

	return m
}

// initConfig ensures the singleton config row exists
func (m *Manager) initConfig() {
	var count int
	m.db.QueryRow("SELECT COUNT(*) FROM multistream_config WHERE id = 1").Scan(&count)
	if count == 0 {
		_, err := m.db.Exec(`INSERT INTO multistream_config (id, max_destinations) VALUES (1, ?)`, m.maxDests)
		if err != nil {
			log.Printf("Failed to initialize multistream config: %v", err)
		}
	}
}

// IsAvailable returns true if multistream was enabled at startup
func (m *Manager) IsAvailable() bool {
	return m.available
}

// GetConfig returns the current multistream configuration
func (m *Manager) GetConfig() (*Config, error) {
	config := &Config{
		Available:       m.available,
		MaxDestinations: m.maxDests,
	}

	if !m.available {
		return config, nil
	}

	row := m.db.QueryRow(`SELECT enabled, max_destinations, original_service_type, original_service_settings 
                          FROM multistream_config WHERE id = 1`)
	err := row.Scan(&config.Enabled, &config.MaxDestinations,
		&config.OriginalServiceType, &config.OriginalServiceSettings)
	if err == sql.ErrNoRows {
		return config, nil
	}
	if err != nil {
		return nil, err
	}
	return config, nil
}

// Enable activates multistream mode, saving original OBS settings
func (m *Manager) Enable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.available {
		return fmt.Errorf("multistream is not available")
	}

	// Get current OBS stream service settings
	serviceType, settings, err := m.obsManager.GetStreamServiceSettings()
	if err != nil {
		return fmt.Errorf("failed to get OBS stream settings: %w", err)
	}

	settingsJSON, _ := json.Marshal(settings)

	// Save original settings
	_, err = m.db.Exec(`UPDATE multistream_config SET 
        enabled = 1, 
        original_service_type = ?, 
        original_service_settings = ? 
        WHERE id = 1`, serviceType, string(settingsJSON))
	if err != nil {
		return fmt.Errorf("failed to save original settings: %w", err)
	}

	// Set OBS to use our internal RTMP relay
	err = m.obsManager.SetStreamServiceSettings("rtmp_custom", map[string]interface{}{
		"server": "rtmp://127.0.0.1:1935/live",
		"key":    "stream",
	})
	if err != nil {
		return fmt.Errorf("failed to set OBS stream settings: %w", err)
	}

	// Regenerate nginx config and reload
	if err := m.reloadNginxConfig(); err != nil {
		log.Printf("Warning: Failed to reload nginx config: %v", err)
	}

	log.Println("Multi-streaming enabled")
	return nil
}

// Disable deactivates multistream mode and restores original OBS settings
func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.available {
		return fmt.Errorf("multistream is not available")
	}

	// Get saved original settings
	var originalType, originalSettingsJSON string
	err := m.db.QueryRow(`SELECT original_service_type, original_service_settings 
                          FROM multistream_config WHERE id = 1`).Scan(&originalType, &originalSettingsJSON)
	if err != nil {
		return fmt.Errorf("failed to get original settings: %w", err)
	}

	// Restore original OBS settings
	var originalSettings map[string]interface{}
	if originalSettingsJSON != "" {
		json.Unmarshal([]byte(originalSettingsJSON), &originalSettings)
	}

	if originalType != "" {
		err = m.obsManager.SetStreamServiceSettings(originalType, originalSettings)
		if err != nil {
			return fmt.Errorf("failed to restore OBS stream settings: %w", err)
		}
	}

	// Mark as disabled
	_, err = m.db.Exec(`UPDATE multistream_config SET enabled = 0 WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	log.Println("Multi-streaming disabled")
	return nil
}

// ListDestinations returns all configured destinations
func (m *Manager) ListDestinations() ([]Destination, error) {
	rows, err := m.db.Query(`SELECT id, name, rtmp_url, stream_key, enabled 
                             FROM multistream_destinations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dests []Destination
	for rows.Next() {
		var d Destination
		if err := rows.Scan(&d.ID, &d.Name, &d.RTMPURL, &d.StreamKey, &d.Enabled); err != nil {
			continue
		}
		dests = append(dests, d)
	}
	return dests, nil
}

// AddDestination adds a new streaming destination
func (m *Manager) AddDestination(name, rtmpURL, streamKey string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check max destinations
	dests, _ := m.ListDestinations()
	if len(dests) >= m.maxDests {
		return 0, fmt.Errorf("maximum destinations (%d) reached", m.maxDests)
	}

	result, err := m.db.Exec(`INSERT INTO multistream_destinations (name, rtmp_url, stream_key) VALUES (?, ?, ?)`,
		name, rtmpURL, streamKey)
	if err != nil {
		return 0, err
	}

	id, _ := result.LastInsertId()

	// Regenerate nginx config
	if err := m.reloadNginxConfig(); err != nil {
		log.Printf("Warning: Failed to reload nginx config: %v", err)
	}

	return id, nil
}

// RemoveDestination removes a streaming destination
func (m *Manager) RemoveDestination(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM multistream_destinations WHERE id = ?`, id)
	if err != nil {
		return err
	}

	return m.reloadNginxConfig()
}

// UpdateDestination updates an existing destination
func (m *Manager) UpdateDestination(id int, name, rtmpURL, streamKey string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`UPDATE multistream_destinations 
                         SET name = ?, rtmp_url = ?, stream_key = ?, enabled = ? 
                         WHERE id = ?`, name, rtmpURL, streamKey, enabled, id)
	if err != nil {
		return err
	}

	return m.reloadNginxConfig()
}

// reloadNginxConfig regenerates the nginx RTMP destinations config and reloads nginx
func (m *Manager) reloadNginxConfig() error {
	dests, err := m.ListDestinations()
	if err != nil {
		return err
	}

	// Generate native push directives for each enabled destination
	// Native push is more reliable than exec_push with ffmpeg
	var pushLines string
	enabledCount := 0
	for _, d := range dests {
		if d.Enabled {
			// Trim trailing slash from URL to avoid double slashes
			rtmpURL := strings.TrimSuffix(d.RTMPURL, "/")
			pushLines += fmt.Sprintf("            push %s/%s;\n", rtmpURL, d.StreamKey)
			enabledCount++
		}
	}

	// Write to /etc/nginx/rtmp.d/destinations.conf
	configContent := "# Auto-generated by Stable Stream Solutions\n# Do not edit manually - changes will be overwritten\n\n" + pushLines

	configPath := "/etc/nginx/rtmp.d/destinations.conf"
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		// Fall back to /etc/nginx/rtmp.conf for older setups
		log.Printf("Could not write to %s, trying /etc/nginx/rtmp.conf: %v", configPath, err)
		configPath = "/etc/nginx/rtmp.conf"
		fullConfig := "# Auto-generated by Stable Stream Solutions\n# Do not edit manually - changes will be overwritten\n\nrtmp {\n    server {\n        listen 127.0.0.1:1935;\n        chunk_size 4096;\n        \n        application live {\n            live on;\n            record off;\n            \n" + pushLines + "        }\n    }\n}\n"
		err = os.WriteFile(configPath, []byte(fullConfig), 0644)
		if err != nil {
			return fmt.Errorf("failed to write nginx config: %w\n\nTry running: sudo chown user:user /etc/nginx/rtmp.d/destinations.conf", err)
		}
	}

	// Reload nginx
	cmd := exec.Command("sudo", "nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("nginx reload output: %s", string(output))
		return fmt.Errorf("failed to reload nginx: %w", err)
	}

	log.Printf("nginx config reloaded with %d enabled destinations (using native push)", enabledCount)
	return nil
}

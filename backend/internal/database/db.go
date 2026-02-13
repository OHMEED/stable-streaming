package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func InitDB() (*DB, error) {
	// Ensure data directory exists
	// For simplicity, we store it in the current directory or a 'data' folder
	// In production, might want /var/lib/... or similar
	dbPath := "data.db"

	// Create database file if not exists
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	file.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	log.Println("Database connected successfully")

	wrappedDB := &DB{db}
	if err := wrappedDB.createTables(); err != nil {
		return nil, err
	}

	return wrappedDB, nil
}

func (db *DB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS ingests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			protocol TEXT NOT NULL, -- 'rtmp', 'srt', 'srtla'
			port INTEGER NOT NULL,
			output_port INTEGER DEFAULT 0, -- Port where the ingest outputs locally (e.g. UDP for OBS)
			srt_port INTEGER DEFAULT 0, -- SRT relay egress port
			ws_port INTEGER DEFAULT 0, -- WebSocket stats port
			rtsp_port INTEGER DEFAULT 0, -- RTSP egress port for SRTLA
			stream_key TEXT, -- passphrase
            stream_id TEXT, -- SRT Stream ID
			enabled BOOLEAN DEFAULT 1
		);`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS auto_scene_switcher (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			ingest_id INTEGER,
			online_scene TEXT DEFAULT '',
			offline_scene TEXT DEFAULT '',
			threshold_kbps INTEGER DEFAULT 1000,
			enabled BOOLEAN DEFAULT 0,
			FOREIGN KEY (ingest_id) REFERENCES ingests(id)
		);`,
		// Multistream configuration (singleton table)
		`CREATE TABLE IF NOT EXISTS multistream_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			enabled BOOLEAN DEFAULT 0,
			max_destinations INTEGER DEFAULT 3,
			original_service_type TEXT DEFAULT '',
			original_service_settings TEXT DEFAULT ''
		);`,
		// Multistream destinations
		`CREATE TABLE IF NOT EXISTS multistream_destinations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			rtmp_url TEXT NOT NULL,
			stream_key TEXT NOT NULL,
			enabled BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			return err
		}
	}

	// Migration: Check if output_port, stream_id, etc exist
	// Integer columns
	var integerColumnsToCheck = []string{"output_port", "srt_port", "bs_port", "ws_port", "rtsp_port"}
	for _, col := range integerColumnsToCheck {
		var checkCount int
		row := db.QueryRow("SELECT count(*) FROM pragma_table_info('ingests') WHERE name=?", col)
		if err := row.Scan(&checkCount); err == nil && checkCount == 0 {
			log.Printf("Migrating database: adding %s column", col)
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE ingests ADD COLUMN %s INTEGER DEFAULT 0", col)); err != nil {
				return fmt.Errorf("failed to add column %s: %v", col, err)
			}
		}
	}

	// Text columns
	var textColumnsToCheck = []string{"stream_id"}
	for _, col := range textColumnsToCheck {
		var checkCount int
		row := db.QueryRow("SELECT count(*) FROM pragma_table_info('ingests') WHERE name=?", col)
		if err := row.Scan(&checkCount); err == nil && checkCount == 0 {
			log.Printf("Migrating database: adding %s column", col)
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE ingests ADD COLUMN %s TEXT DEFAULT ''", col)); err != nil {
				return fmt.Errorf("failed to add column %s: %v", col, err)
			}
		}
	}

	// Migration: Add only_on_scene column to auto_scene_switcher if it doesn't exist
	var onlyOnSceneCount int
	row := db.QueryRow("SELECT count(*) FROM pragma_table_info('auto_scene_switcher') WHERE name='only_on_scene'")
	if err := row.Scan(&onlyOnSceneCount); err == nil && onlyOnSceneCount == 0 {
		log.Println("Migrating database: adding only_on_scene column to auto_scene_switcher")
		if _, err := db.Exec("ALTER TABLE auto_scene_switcher ADD COLUMN only_on_scene TEXT DEFAULT ''"); err != nil {
			return fmt.Errorf("failed to add only_on_scene column: %v", err)
		}
	}

	// Check if default user exists, if not create 'admin/password' (temporary)
	// In a real app we would force change on first login or set via env var
	// For this task verify default credentials requirement: "A simple login page"
	// and "password: a way to set a new password".
	// We will seed a default user "admin"

	var count int
	// We reuse 'row' from above if we want, or just query again.
	// But 'err' from migration block might shadow if we redeclare or not careful.
	// Let's use clean variables.
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Default password 'admin'
		hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", "admin", string(hash))
		if err != nil {
			return err
		}
		log.Println("Seeded default admin user")
	}

	return nil
}

// GetConfig retrieves a value from the config table by key
func (db *DB) GetConfig(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetConfig sets a value in the config table (upsert)
func (db *DB) SetConfig(key, value string) error {
	_, err := db.Exec(`
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

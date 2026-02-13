package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"stable-stream-solutions/internal/database"
	"stable-stream-solutions/internal/ingest"
	"stable-stream-solutions/internal/multistream"
	"stable-stream-solutions/internal/obs"
	"stable-stream-solutions/internal/ratelimit"
	"stable-stream-solutions/internal/sceneswitcher"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
)

type Server struct {
	router             *chi.Mux
	db                 *database.DB
	ingestManager      *ingest.Manager
	obsManager         *obs.Manager
	multistreamManager *multistream.Manager
	sceneSwitcher      *sceneswitcher.SceneSwitcher
	rateLimiter        *ratelimit.IPTracker
	jwtSecret          []byte
	// Ingest creation lock
	ingestsLocked bool
}

func NewServer(db *database.DB, ingestManager *ingest.Manager, obsManager *obs.Manager, multistreamManager *multistream.Manager) *Server {
	// Create scene switcher
	ss := sceneswitcher.NewSceneSwitcher(db, ingestManager, obsManager)

	// Create rate limiter: 500 requests per minute, 5 minute block after exceeding
	// Higher limit needed because SPA makes many API calls on page load
	rl := ratelimit.NewIPTracker(500, 1*time.Minute, 5*time.Minute)

	// Initialize JWT Secret
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		// Generate random secret
		jwtSecret = make([]byte, 32)
		if _, err := rand.Read(jwtSecret); err != nil {
			log.Fatal("Failed to generate random JWT secret")
		}
		log.Printf("WARNING: JWT_SECRET not set. Generated random secret: %s", hex.EncodeToString(jwtSecret))
	}

	s := &Server{
		router:             chi.NewRouter(),
		db:                 db,
		ingestManager:      ingestManager,
		obsManager:         obsManager,
		multistreamManager: multistreamManager,
		sceneSwitcher:      ss,
		rateLimiter:        rl,
		jwtSecret:          jwtSecret,
	}

	// Initialize OBS WebSocket password from database
	s.initOBSPassword()

	// Initialize VNC password from database and write wayvnc config
	s.initVNCPassword()

	s.setupRoutes()

	// Start scene switcher if enabled in config
	ss.StartIfEnabled()

	return s
}

func (s *Server) setupRoutes() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.StripSlashes)

	// CORS for dev
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // Restrict in production
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API Routes (Mounted at both /api and /server/api for compatibility)
	s.setupAPIRoutes("/api")
	s.setupAPIRoutes("/server/api")

	// Serve Frontend (SPA)
	workDir, _ := os.Getwd()
	// Running from project root via start.sh
	filesDir := filepath.Join(workDir, "frontend/dist")

	// Verify it exists
	if info, err := os.Stat(filesDir); err != nil || !info.IsDir() {
		log.Printf("WARNING: Frontend files not found at %s. UI may return 404.", filesDir)
	} else {
		log.Printf("Serving frontend from %s", filesDir)
	}

	// Handle /server/* prefix (for direct IP access compabitility with base='/server/')
	// This strips '/server' and looks for files in dist root
	s.FileServer(s.router, "/server", http.Dir(filesDir))

	// Handle root requests (catch-all)
	s.FileServer(s.router, "/", http.Dir(filesDir))
}

func (s *Server) setupAPIRoutes(mountPath string) {
	s.router.Route(mountPath, func(r chi.Router) {
		// Rate limiting for API endpoints only
		r.Use(s.rateLimiter.Middleware)

		// Standard API Timeout (exclude persistent connections like VNC)
		r.Use(middleware.Timeout(60 * time.Second))

		r.Get("/status", s.handleStatus)
		r.Post("/login", s.handleLogin)
		r.Get("/system/version", s.handleGetSystemVersion)

		// Protected Routes
		r.Group(func(r chi.Router) {
			r.Use(s.AuthMiddleware)
			r.Get("/verify", s.handleVerify)

			r.Get("/ingests", s.handleListIngests)
			r.Post("/ingests", s.handleAddIngest)
			r.Delete("/ingests/{id}", s.handleRemoveIngest)
			r.Patch("/ingests/{id}/name", s.handleRenameIngest)
			r.Post("/ingests/{id}/start", s.handleStartIngest)
			r.Post("/ingests/{id}/stop", s.handleStopIngest)
			r.Get("/ingests/{id}/stats", s.handleIngestStats)
			r.Get("/ingests/{id}/logs", s.handleIngestLogs)
			r.Get("/ingests/{id}/obs-connection", s.handleGetIngestOBSConnection)

			// Settings routes
			r.Get("/settings/ingests-locked", s.handleGetIngestsLocked)
			r.Put("/settings/ingests-locked", s.handleSetIngestsLocked)

			r.Get("/obs/status", s.handleOBSStatus)
			r.Get("/obs/scenes", s.handleGetScenes)
			r.Post("/obs/scene", s.handleSetScene)
			r.Post("/obs/source", s.handleAddMediaSource)
			r.Get("/obs/scene/{sceneName}/items", s.handleGetSceneItems)
			r.Post("/obs/stream/toggle", s.handleOBSToggleStream)
			r.Post("/obs/record/toggle", s.handleOBSToggleRecord)
			r.Get("/obs/preview", s.handleOBSPreview)
			r.Get("/recordings", s.handleListRecordings)
			r.Get("/recordings/download/{filename}", s.handleDownloadRecording)
			r.Get("/recordings/thumbnail/{filename}", s.handleRecordingThumbnail)
			r.Delete("/recordings/{filename}", s.handleDeleteRecording)
			r.Get("/system/storage", s.handleGetStorage)
			r.Post("/user/password", s.handleUpdatePassword)
			r.Post("/upload", s.handleUploadFile)

			// Scene Switcher routes
			r.Get("/scene-switcher/config", s.handleGetSceneSwitcherConfig)
			r.Post("/scene-switcher/config", s.handleSaveSceneSwitcherConfig)
			r.Post("/scene-switcher/enable", s.handleSetSceneSwitcherEnabled)

			// OBS Settings routes
			r.Get("/obs/settings", s.handleGetOBSSettings)
			r.Post("/obs/settings/password", s.handleGenerateOBSPassword)

			// Server info route (for public IP, etc.)
			r.Get("/server/info", s.handleGetServerInfo)

			// Multistream routes
			r.Get("/multistream/config", s.handleGetMultistreamConfig)
			r.Post("/multistream/enable", s.handleEnableMultistream)
			r.Post("/multistream/disable", s.handleDisableMultistream)
			r.Get("/multistream/destinations", s.handleListDestinations)
			r.Post("/multistream/destinations", s.handleAddDestination)
			r.Put("/multistream/destinations/{id}", s.handleUpdateDestination)
			r.Delete("/multistream/destinations/{id}", s.handleRemoveDestination)
			r.Get("/multistream/logs", s.handleMultistreamLogs)

			// Application logs
			r.Get("/logs/app", s.handleAppLogs)

			// System update routes
			r.Post("/system/update", s.handleSystemUpdate)

			// noVNC Proxy
			// VNC password endpoint must be defined BEFORE the catch-all VNC mount
			r.Get("/vnc/password", s.handleGetVNCPassword)
			// VNC token endpoint returns the auth token for WebSocket connections
			r.Get("/vnc/token", s.handleGetVNCToken)
			// VNC direct URL endpoint returns Cloudflare hostname for low-latency connection
			r.Get("/vnc/direct-url", s.handleGetVNCDirectURL)
			// Websockify must be defined BEFORE the catch-all VNC mount
			r.Get("/vnc/websockify", s.handleVNCWebSocket)

			r.Mount("/vnc", http.HandlerFunc(s.handleVNCProxy))
		})
	})
}

func (s *Server) initOBSPassword() {
	var password string

	// Check environment variable first (from config.env)
	envPassword := os.Getenv("OBS_WEBSOCKET_PASSWORD")
	if envPassword != "" {
		password = envPassword
		// Update DB to match env
		s.db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES ('obs_ws_password', ?)", password)
		log.Printf("OBS WebSocket password initialized from environment variable")
	} else {
		// Fallback to database or generate new
		err := s.db.QueryRow("SELECT value FROM config WHERE key = 'obs_ws_password'").Scan(&password)
		if err != nil || password == "" {
			// No password stored yet - generate one on first run
			password = generateSecurePassword(20)
			s.db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES ('obs_ws_password', ?)", password)
			log.Printf("Generated new OBS WebSocket password")
		}
	}

	s.obsManager.SetPassword(password)
	if err := s.obsManager.ConfigureWebSocket(); err != nil {
		log.Printf("Warning: Failed to configure OBS WebSocket: %v", err)
	}
}

func (s *Server) initVNCPassword() {
	var password string
	err := s.db.QueryRow("SELECT value FROM config WHERE key = 'vnc_password'").Scan(&password)
	if err != nil || password == "" {
		// No password stored yet - generate one on first run
		password = generateSecurePassword(20)
		s.db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES ('vnc_password', ?)", password)
		log.Printf("Generated new VNC password")
	}

	// Write wayvnc config file
	if err := s.writeWayvncConfig(password); err != nil {
		log.Printf("Warning: Failed to write wayvnc config: %v", err)
	}

	log.Printf("VNC password initialized from database")
}

func (s *Server) writeWayvncConfig(password string) error {
	// Get user's home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Join(home, ".config", "wayvnc")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	configPath := filepath.Join(configDir, "config")
	configContent := fmt.Sprintf(`address=127.0.0.1
port=5900
password=%s
`, password)

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Printf("Wrote wayvnc config to %s", configPath)
	return nil
}

func (s *Server) getVNCPassword() string {
	var password string
	s.db.QueryRow("SELECT value FROM config WHERE key = 'vnc_password'").Scan(&password)
	return password
}

// getPublicIP attempts to get the server's public IP address
func getPublicIP() string {
	// Try ipify API first
	if ip := fetchIP("https://api.ipify.org"); ip != "" {
		return ip
	}
	// Fallback to icanhazip
	if ip := fetchIP("https://icanhazip.com"); ip != "" {
		return ip
	}
	return ""
}

func fetchIP(url string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	return strings.TrimSpace(string(buf[:n]))
}

func (s *Server) Start() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	return http.ListenAndServe(":"+port, s.router)
}

func (s *Server) FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")

		// Custom SPA handling
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))

		rootDir := string(root.(http.Dir))

		// Clean path to prevent directory traversal? http.FileServer handles it but we check manually for fallback
		reqPath := strings.TrimPrefix(r.URL.Path, pathPrefix)
		fullPath := filepath.Join(rootDir, reqPath)

		info, err := os.Stat(fullPath)
		if os.IsNotExist(err) || (err == nil && info.IsDir()) {
			// If file doesn't exist, serve index.html
			indexFile := filepath.Join(rootDir, "index.html")
			http.ServeFile(w, r, indexFile)
			return
		}

		fs.ServeHTTP(w, r)
	})
}

func (s *Server) handleVNCProxy(w http.ResponseWriter, r *http.Request) {
	// Check if this is a WebSocket upgrade request (for the actual VNC connection)
	if websocket.IsWebSocketUpgrade(r) {
		s.handleVNCWebSocket(w, r)
		return
	}

	// For non-WebSocket requests, serve noVNC static files
	// noVNC is installed at /usr/share/novnc/ on Linux
	novncPath := "/usr/share/novnc"

	// Check common locations for noVNC
	possiblePaths := []string{
		"/usr/share/novnc",
		"/usr/share/novnc-snapd", // snap version
		"./novnc",                // local fallback
	}

	for _, p := range possiblePaths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			novncPath = p
			log.Printf("Using noVNC static files from: %s", novncPath)
			break
		}
	}

	// Determine the path to serve
	path := r.URL.Path
	// Robustly strip known prefixes if present (handling various router/proxy scenarios)
	knownPrefixes := []string{"/server/api/vnc", "/api/vnc", "/vnc"}
	for _, prefix := range knownPrefixes {
		if path == prefix {
			path = "/"
			break
		}
		if strings.HasPrefix(path, prefix+"/") {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}
	// Ensure leading slash
	if path == "" || !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Serve static files from noVNC directory
	log.Printf("VNC Proxy: Original: %s | Serving: %s from %s", r.URL.Path, path, novncPath)

	// Temporarily modify request path for the file server
	r.URL.Path = path
	fs := http.FileServer(http.Dir(novncPath))
	fs.ServeHTTP(w, r)
}

// WebSocket upgrader for client connections
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (already behind JWT auth)
	},
	// Accept any subprotocol requested by the client
	Subprotocols: []string{"binary", "base64"},
}

// handleVNCWebSocket proxies WebSocket connections between browser and websockify
func (s *Server) handleVNCWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade client connection
	clientConn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade client WebSocket: %v", err)
		return
	}
	defer clientConn.Close()

	// Connect to websockify backend
	// Note: websockify doesn't care about the path after the port usually,
	// but we'll use /websockify as standard for noVNC
	backendURL := "ws://127.0.0.1:6080/websockify"

	// Request the same subprotocols from the backend as the client did
	dialer := websocket.Dialer{
		Subprotocols: r.Header["Sec-Websocket-Protocol"],
	}

	backendConn, resp, err := dialer.Dial(backendURL, nil)
	if err != nil {
		log.Printf("Failed to connect to websockify backend: %v", err)
		clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Backend connection failed"))
		return
	}
	defer backendConn.Close()

	// Log selected subprotocol
	if resp != nil {
		log.Printf("VNC WebSocket: Subprotocol selected: %s", resp.Header.Get("Sec-Websocket-Protocol"))
	}

	// Bidirectional proxy
	errChan := make(chan error, 2)

	// Client -> Backend
	go func() {
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := backendConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Backend -> Client
	go func() {
		for {
			messageType, message, err := backendConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := clientConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Wait for either direction to fail
	<-errChan
}

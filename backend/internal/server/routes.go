package server

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"stable-stream-solutions/internal/applog"
	"stable-stream-solutions/internal/ratelimit"
	"stable-stream-solutions/internal/sceneswitcher"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

//go:embed VERSION
var versionFile embed.FS

// Handlers

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleGetServerInfo returns server info including public IP for external connections
func (s *Server) handleGetServerInfo(w http.ResponseWriter, r *http.Request) {
	publicIP := getPublicIP()
	if publicIP == "" {
		publicIP = getLocalIP()
	}

	response := map[string]interface{}{
		"public_ip": publicIP,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Get client IP for rate limiting
	ip := ratelimit.GetClientIP(r)

	// Check if IP is blocked due to too many failed attempts
	if s.rateLimiter.IsBlocked(ip) {
		http.Error(w, "Too many failed login attempts. Please try again later.", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify user
	var id int
	var hash string
	err := s.db.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", req.Username).Scan(&id, &hash)
	if err != nil {
		// User not found - record failed login attempt
		if s.rateLimiter.RecordFailedLogin(ip) {
			http.Error(w, "Too many failed login attempts. Please try again later.", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		// Wrong password - record failed login attempt
		applog.LogAuth(false, ip, "invalid password")
		if s.rateLimiter.RecordFailedLogin(ip) {
			http.Error(w, "Too many failed login attempts. Please try again later.", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate JWT
	// Generate JWT
	secret := s.jwtSecret

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": id,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString(secret)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Set Cookie for iframe access
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt_token",
		Value:    tokenString,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	applog.LogAuth(true, ip, "")
	json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
}

// AuthMiddleware verifies the JWT token
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		var tokenStr string

		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		// Fallback to cookie
		// Check 'jwt_token' (local) and 'auth_token' (portal)
		if tokenStr == "" {
			if cookie, err := r.Cookie("jwt_token"); err == nil {
				tokenStr = cookie.Value
			} else if cookie, err := r.Cookie("auth_token"); err == nil {
				tokenStr = cookie.Value
			}
		}

		// Fallback to query parameter (for WebSocket connections like noVNC)
		// WebSocket clients can't send custom headers, so we accept token from URL
		if tokenStr == "" {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse token without validation first to check 'alg' and 'claims'
		token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
		if err != nil {
			http.Error(w, "Invalid token format", http.StatusUnauthorized)
			return
		}

		// Log Claims to identify token origin
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Optional: keep minimal logging or remove entirely
			_ = claims
		}

		// Determine verification method based on 'alg'
		var verificationKey interface{}
		alg := token.Method.Alg()

		if alg == "RS256" {
			// Portal Token (SSO) -> Verify with Public Key
			publicKeyPEM := []byte(os.Getenv("JWT_PUBLIC_KEY"))
			if len(publicKeyPEM) == 0 {
				if content, err := os.ReadFile("public.pem"); err == nil {
					publicKeyPEM = content
				}
			}
			if len(publicKeyPEM) == 0 {
				log.Println("Error: JWT_PUBLIC_KEY missing for RS256 token verification")
				http.Error(w, "Server configuration error", http.StatusInternalServerError)
				return
			}
			publicKey, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyPEM)
			if err != nil {
				log.Printf("Error parsing public key: %v", err)
				http.Error(w, "Server configuration error", http.StatusInternalServerError)
				return
			}
			verificationKey = publicKey

		} else if alg == "HS256" {
			// Local Token -> Verify with Local Secret
			verificationKey = s.jwtSecret
		} else {
			http.Error(w, fmt.Sprintf("Unsupported signing method: %v", alg), http.StatusUnauthorized)
			return
		}

		// Verify signature
		verifiedToken, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return verificationKey, nil
		})

		if err != nil || !verifiedToken.Valid {
			http.Error(w, "Invalid token signature", http.StatusUnauthorized)
			return
		}
		if claims, ok := verifiedToken.Claims.(jwt.MapClaims); ok {
			// Extract User ID: try 'sub' (standard) then 'userId' (portal)
			var userID interface{}
			if sub, ok := claims["sub"]; ok {
				userID = sub
			} else if uid, ok := claims["userId"]; ok {
				userID = uid
			}

			if userID == nil {
				http.Error(w, "Invalid token claims", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), "user_id", userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
		}
	})
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	// If it passes AuthMiddleware, it's valid.
	json.NewEncoder(w).Encode(map[string]string{"valid": "true"})
}

func (s *Server) handleListIngests(w http.ResponseWriter, r *http.Request) {
	ingests, err := s.ingestManager.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(ingests)
}

func (s *Server) handleAddIngest(w http.ResponseWriter, r *http.Request) {
	// Check if ingest creation is locked
	if s.ingestsLocked {
		http.Error(w, "Ingest creation is locked", http.StatusForbidden)
		return
	}

	var req struct {
		Name      string `json:"name"`
		Protocol  string `json:"protocol"`
		StreamKey string `json:"stream_key"`
		StreamID  string `json:"stream_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Server-side validation: require password (stream_key) of at least 10 characters
	// This matches frontend validation and prevents API bypass
	if len(req.StreamKey) < 10 {
		http.Error(w, "Password (stream_key) must be at least 10 characters", http.StatusBadRequest)
		return
	}

	id, err := s.ingestManager.Add(req.Name, req.Protocol, req.StreamKey, req.StreamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Auto-start the new ingest
	go s.ingestManager.Start(id)

	applog.LogIngest("create", req.Name, id, req.Protocol)

	// Return the created object (port will be auto-assigned and visible on list refresh)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         id,
		"name":       req.Name,
		"protocol":   req.Protocol,
		"stream_key": req.StreamKey,
		"stream_id":  req.StreamID,
		"enabled":    true,
	})
}

func (s *Server) handleRemoveIngest(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	if err := s.ingestManager.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	applog.LogIngest("delete", "", id, "")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRenameIngest(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Update the name in the database
	_, err := s.db.Exec("UPDATE ingests SET name = ? WHERE id = ?", req.Name, id)
	if err != nil {
		http.Error(w, "Failed to update ingest name", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Ingest renamed successfully"})
}

func (s *Server) handleGetIngestsLocked(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"locked": s.ingestsLocked})
}

func (s *Server) handleSetIngestsLocked(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Locked bool `json:"locked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	s.ingestsLocked = req.Locked

	// Persist to database config table
	value := "false"
	if req.Locked {
		value = "true"
	}
	if err := s.db.SetConfig("ingests_locked", value); err != nil {
		log.Printf("Warning: Failed to persist ingests_locked setting: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"locked": s.ingestsLocked})
}

// SetIngestsLocked sets the ingest lock state (called from main.go on startup)
func (s *Server) SetIngestsLocked(locked bool) {
	s.ingestsLocked = locked
	// Persist to database
	value := "false"
	if locked {
		value = "true"
	}
	if err := s.db.SetConfig("ingests_locked", value); err != nil {
		log.Printf("Warning: Failed to persist ingests_locked setting: %v", err)
	}
}

// InitIngestsLocked loads the ingests_locked setting from database
func (s *Server) InitIngestsLocked() {
	value, err := s.db.GetConfig("ingests_locked")
	if err == nil && value == "true" {
		s.ingestsLocked = true
		log.Printf("Ingest creation is LOCKED")
	}
}

func (s *Server) handleStartIngest(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	if err := s.ingestManager.Start(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStopIngest(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	if err := s.ingestManager.Stop(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleIngestStats(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	stats, err := s.ingestManager.GetStats(id)
	if err != nil {
		// log.Printf("Error fetching stats for ingest %d: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleIngestLogs(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	// Security check: ensure access to this ingest? (Assuming auth middleware covers it for now)

	logPath := fmt.Sprintf("logs/ingest_%d.log", id)
	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		http.Error(w, "Log file not found (process might not have started yet)", http.StatusNotFound)
		return
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		log.Printf("Error reading log for ingest %d: %v", id, err)
		http.Error(w, "Failed to read logs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"content": string(content)})
}

// handleGetIngestOBSConnection returns the correct URL and input format for adding an ingest to OBS
// For SRTLA: returns UDP URL with mpegts format (go-irl output)
// For SRT/RTMP: returns RTSP URL with no special format (MediaMTX output)
func (s *Server) handleGetIngestOBSConnection(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ingest ID", http.StatusBadRequest)
		return
	}

	var protocol string
	var outputPort int
	var streamKey string
	err = s.db.QueryRow("SELECT protocol, output_port, stream_key FROM ingests WHERE id = ?", id).Scan(&protocol, &outputPort, &streamKey)
	if err != nil {
		http.Error(w, "Ingest not found", http.StatusNotFound)
		return
	}

	var connectionURL string
	var inputFormat string

	if protocol == "srtla" {
		// SRTLA uses UDP mpegts output from go-irl
		connectionURL = fmt.Sprintf("udp://127.0.0.1:%d", outputPort)
		inputFormat = "mpegts"
	} else {
		// SRT/RTMP uses RTSP output from MediaMTX
		// Path is the stream_key if set, otherwise 'live'
		path := "live"
		if streamKey != "" {
			path = streamKey
		}
		connectionURL = fmt.Sprintf("rtsp://localhost:%d/%s", outputPort, path)
		inputFormat = ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"url":          connectionURL,
		"input_format": inputFormat,
		"protocol":     protocol,
	})
}

func (s *Server) handleOBSStatus(w http.ResponseWriter, r *http.Request) {
	streaming, _ := s.obsManager.GetStreamStatus()
	recording, _ := s.obsManager.GetRecordStatus()
	currentScene, _ := s.obsManager.GetCurrentProgramScene()

	status := map[string]interface{}{
		"connected":    s.obsManager.IsConnected(),
		"streaming":    streaming,
		"recording":    recording,
		"currentScene": currentScene,
	}
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleOBSToggleStream(w http.ResponseWriter, r *http.Request) {
	if err := s.obsManager.ToggleStream(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to toggle stream: %v", err), http.StatusBadGateway)
		return
	}
	applog.LogOBS("stream_toggle")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleOBSToggleRecord(w http.ResponseWriter, r *http.Request) {
	if err := s.obsManager.ToggleRecord(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to toggle record: %v", err), http.StatusBadGateway)
		return
	}
	applog.LogOBS("record_toggle")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleOBSPreview(w http.ResponseWriter, r *http.Request) {
	imgData, err := s.obsManager.GetScreenshot()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get preview: %v", err), http.StatusBadGateway)
		return
	}

	// imgData is base64 encoded string from OBS
	// We can serve it as JSON or directly as image. Let's send as JSON for now to be flexible.
	json.NewEncoder(w).Encode(map[string]string{"image": imgData})
}

func (s *Server) handleGetScenes(w http.ResponseWriter, r *http.Request) {
	scenes, err := s.obsManager.GetSceneList()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch scenes: %v", err), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(scenes)
}

func (s *Server) handleSetScene(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SceneName string `json:"sceneName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := s.obsManager.SetScene(req.SceneName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to set scene: %v", err), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleAddMediaSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SceneName   string `json:"sceneName"`
		SourceName  string `json:"sourceName"`
		Protocol    string `json:"protocol"`
		URL         string `json:"url"`
		InputFormat string `json:"inputFormat"` // "mpegts" for SRTLA UDP, empty for RTSP
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.obsManager.AddMediaSource(req.SceneName, req.SourceName, req.Protocol, req.URL, req.InputFormat); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add media source: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Source added successfully"})
}

func (s *Server) handleGetSceneItems(w http.ResponseWriter, r *http.Request) {
	sceneName := chi.URLParam(r, "sceneName")
	if sceneName == "" {
		http.Error(w, "Scene name required", http.StatusBadRequest)
		return
	}

	items, err := s.obsManager.GetSceneItemList(sceneName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get scene items: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(items)
}

func (s *Server) handleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	// Get User ID from context (set by AuthMiddleware)
	userIDCtx := r.Context().Value("user_id")
	if userIDCtx == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	userID := int(userIDCtx.(float64)) // JWT JSON numbers are float64 usually, or int depending on parser.

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Verify current password
	var hash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", userID).Scan(&hash)
	if err != nil {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)); err != nil {
		http.Error(w, "Incorrect current password", http.StatusUnauthorized)
		return
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Update DB
	_, err = s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", string(newHash), userID)
	if err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated"})
}

func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	// Parse max 100MB
	r.ParseMultipartForm(100 << 20)

	file, handler, err := r.FormFile("file")
	if err != nil {
		log.Printf("Error retrieving file: %v", err)
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	log.Printf("Uploaded File: %+v\n", handler.Filename)
	log.Printf("File Size: %+v\n", handler.Size)
	log.Printf("MIME Header: %+v\n", handler.Header)

	// Ensure upload dir exists
	home, _ := os.UserHomeDir()
	uploadDir := filepath.Join(home, "Downloads", "uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("Error creating upload dir: %v", err)
		http.Error(w, "Failed to create upload directory", http.StatusInternalServerError)
		return
	}

	// Save file
	// Fix: Prevent directory traversal by using filepath.Base
	dstPath := filepath.Join(uploadDir, filepath.Base(handler.Filename))
	dst, err := os.Create(dstPath)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("Error saving file: %v", err)
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message":  "File uploaded successfully",
		"filename": handler.Filename,
		"path":     dstPath,
	})
}
func (s *Server) handleListRecordings(w http.ResponseWriter, r *http.Request) {
	recordDir, err := s.obsManager.GetRecordDirectory()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get record directory: %v", err), http.StatusBadGateway)
		return
	}

	files, err := os.ReadDir(recordDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read record directory: %v", err), http.StatusInternalServerError)
		return
	}

	type Recording struct {
		Name string    `json:"name"`
		Size int64     `json:"size"`
		Date time.Time `json:"date"`
	}

	var recordings []Recording
	for _, f := range files {
		if !f.IsDir() {
			info, err := f.Info()
			if err == nil {
				// Filter for common video extensions
				ext := strings.ToLower(filepath.Ext(f.Name()))
				if ext == ".mp4" || ext == ".mkv" || ext == ".mov" || ext == ".flv" || ext == ".ts" {
					recordings = append(recordings, Recording{
						Name: f.Name(),
						Size: info.Size(),
						Date: info.ModTime(),
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordings)
}

func (s *Server) handleDownloadRecording(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" {
		http.Error(w, "Filename required", http.StatusBadRequest)
		return
	}

	// Clean filename to prevent directory traversal
	filename = filepath.Base(filename)

	recordDir, err := s.obsManager.GetRecordDirectory()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get record directory: %v", err), http.StatusBadGateway)
		return
	}

	filePath := filepath.Join(recordDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Use ServeFile to support range requests (seeking in video players)
	http.ServeFile(w, r, filePath)
}

func (s *Server) handleRecordingThumbnail(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" {
		http.Error(w, "Filename required", http.StatusBadRequest)
		return
	}

	filename = filepath.Base(filename)
	recordDir, err := s.obsManager.GetRecordDirectory()
	if err != nil {
		http.Error(w, "Failed to get record directory", http.StatusBadGateway)
		return
	}

	videoPath := filepath.Join(recordDir, filename)
	thumbDir := filepath.Join(recordDir, ".thumbnails")
	thumbPath := filepath.Join(thumbDir, filename+".jpg")

	// Ensure thumb dir exists
	os.MkdirAll(thumbDir, 0755)

	// Check if thumbnail already exists
	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		// Generate thumbnail using ffmpeg
		// -ss 00:00:01 (at 1 second)
		// -i input
		// -vframes 1
		// -s 320x180
		// -f image2
		cmd := exec.Command("ffmpeg", "-ss", "00:00:01", "-i", videoPath, "-vframes", "1", "-s", "320x180", "-f", "image2", "-y", thumbPath)
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to generate thumbnail: %v", err)
			http.Error(w, "Failed to generate thumbnail", http.StatusInternalServerError)
			return
		}
	}

	http.ServeFile(w, r, thumbPath)
}

func (s *Server) handleDeleteRecording(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" {
		http.Error(w, "Filename required", http.StatusBadRequest)
		return
	}

	// Clean filename to prevent directory traversal
	filename = filepath.Base(filename)

	recordDir, err := s.obsManager.GetRecordDirectory()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get record directory: %v", err), http.StatusBadGateway)
		return
	}

	filePath := filepath.Join(recordDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Delete main file
	if err := os.Remove(filePath); err != nil {
		log.Printf("Failed to delete recording %s: %v", filename, err)
		http.Error(w, "Failed to delete file", http.StatusInternalServerError)
		return
	}

	// Delete thumbnail if exists
	thumbPath := filepath.Join(recordDir, ".thumbnails", filename+".jpg")
	if _, err := os.Stat(thumbPath); err == nil {
		os.Remove(thumbPath)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Recording deleted successfully"})
}

func (s *Server) handleGetStorage(w http.ResponseWriter, r *http.Request) {
	recordDir, err := s.obsManager.GetRecordDirectory()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get record directory: %v", err), http.StatusBadGateway)
		return
	}

	var total, free, used uint64

	if runtime.GOOS == "windows" {
		// Windows: Use wmic command to get disk info
		// Get drive letter from path
		drive := filepath.VolumeName(recordDir)
		if drive == "" {
			drive = "C:"
		}

		// Use PowerShell to get disk space (more reliable than wmic)
		cmd := exec.Command("powershell", "-Command",
			fmt.Sprintf("Get-PSDrive -Name '%s' | Select-Object -Property Used,Free | ConvertTo-Json", strings.TrimSuffix(drive, ":")))
		out, err := cmd.Output()
		if err != nil {
			// Fallback: try wmic
			cmd = exec.Command("wmic", "logicaldisk", "where", fmt.Sprintf("DeviceID='%s'", drive), "get", "FreeSpace,Size", "/value")
			out, err = cmd.Output()
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to get storage info: %v", err), http.StatusInternalServerError)
				return
			}
			// Parse wmic output
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "FreeSpace=") {
					free, _ = strconv.ParseUint(strings.TrimPrefix(line, "FreeSpace="), 10, 64)
				} else if strings.HasPrefix(line, "Size=") {
					total, _ = strconv.ParseUint(strings.TrimPrefix(line, "Size="), 10, 64)
				}
			}
		} else {
			// Parse PowerShell JSON output
			var psResult struct {
				Used int64 `json:"Used"`
				Free int64 `json:"Free"`
			}
			if err := json.Unmarshal(out, &psResult); err == nil {
				free = uint64(psResult.Free)
				used = uint64(psResult.Used)
				total = free + used
			}
		}
		used = total - free
	} else {
		// Unix (Linux/macOS): Use df command
		cmd := exec.Command("df", "-B1", recordDir)
		out, err := cmd.Output()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get storage info: %v", err), http.StatusInternalServerError)
			return
		}

		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				total, _ = strconv.ParseUint(fields[1], 10, 64)
				used, _ = strconv.ParseUint(fields[2], 10, 64)
				free, _ = strconv.ParseUint(fields[3], 10, 64)
			}
		}
	}

	resp := map[string]uint64{
		"total": total,
		"free":  free,
		"used":  used,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Scene Switcher Handlers

func (s *Server) handleGetSceneSwitcherConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.sceneSwitcher.GetConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get config: %v", err), http.StatusInternalServerError)
		return
	}

	// Add running status to response
	response := map[string]interface{}{
		"ingest_id":      config.IngestID,
		"online_scene":   config.OnlineScene,
		"offline_scene":  config.OfflineScene,
		"only_on_scene":  config.OnlyOnScene,
		"threshold_kbps": config.ThresholdKbps,
		"enabled":        config.Enabled,
		"running":        s.sceneSwitcher.IsRunning(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleSaveSceneSwitcherConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IngestID      int    `json:"ingest_id"`
		OnlineScene   string `json:"online_scene"`
		OfflineScene  string `json:"offline_scene"`
		OnlyOnScene   string `json:"only_on_scene"`
		ThresholdKbps int    `json:"threshold_kbps"`
		Enabled       bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.IngestID == 0 {
		http.Error(w, "ingest_id is required", http.StatusBadRequest)
		return
	}
	if req.OnlineScene == "" {
		http.Error(w, "online_scene is required", http.StatusBadRequest)
		return
	}
	if req.OfflineScene == "" {
		http.Error(w, "offline_scene is required", http.StatusBadRequest)
		return
	}
	if req.ThresholdKbps <= 0 {
		req.ThresholdKbps = 1000 // Default
	}

	config := &sceneswitcher.Config{
		IngestID:      req.IngestID,
		OnlineScene:   req.OnlineScene,
		OfflineScene:  req.OfflineScene,
		OnlyOnScene:   req.OnlyOnScene,
		ThresholdKbps: req.ThresholdKbps,
		Enabled:       req.Enabled,
	}

	err := s.sceneSwitcher.SaveConfig(config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Configuration saved"})
}

func (s *Server) handleSetSceneSwitcherEnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.sceneSwitcher.SetEnabled(req.Enabled); err != nil {
		http.Error(w, fmt.Sprintf("Failed to set enabled: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": req.Enabled,
		"running": s.sceneSwitcher.IsRunning(),
	})
}

// OBS Settings Handlers

func (s *Server) handleGetOBSSettings(w http.ResponseWriter, r *http.Request) {
	// Get password from database or use current one from manager
	var password string
	err := s.db.QueryRow("SELECT value FROM config WHERE key = 'obs_ws_password'").Scan(&password)
	if err != nil || password == "" {
		// No password stored yet - generate one
		password = generateSecurePassword(20)
		s.db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES ('obs_ws_password', ?)", password)
		s.obsManager.SetPassword(password)
		// Update OBS config with new password
		if err := s.obsManager.ConfigureWebSocket(); err != nil {
			log.Printf("Warning: Failed to update OBS config: %v", err)
		}
	}

	// Get server IP - use public IP for external connections
	serverIP := getPublicIP()
	if serverIP == "" {
		// Fallback to local IP if public IP unavailable
		serverIP = getLocalIP()
	}

	response := map[string]interface{}{
		"password":  password,
		"port":      s.obsManager.GetPort(),
		"server_ip": serverIP,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleGenerateOBSPassword(w http.ResponseWriter, r *http.Request) {
	// Generate new secure password
	password := generateSecurePassword(20)

	// Save to database
	_, err := s.db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES ('obs_ws_password', ?)", password)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save password: %v", err), http.StatusInternalServerError)
		return
	}

	// Update OBS manager
	s.obsManager.SetPassword(password)

	// Update OBS config file
	if err := s.obsManager.ConfigureWebSocket(); err != nil {
		log.Printf("Warning: Failed to update OBS config: %v", err)
		// Continue anyway - the password is saved, OBS will need restart
	}

	// Get server IP
	serverIP := getLocalIP()

	response := map[string]interface{}{
		"password":  password,
		"port":      s.obsManager.GetPort(),
		"server_ip": serverIP,
		"message":   "Password generated. OBS may need to be restarted for changes to take effect.",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetVNCPassword returns the VNC password for noVNC auto-authentication
func (s *Server) handleGetVNCPassword(w http.ResponseWriter, r *http.Request) {
	password := s.getVNCPassword()
	if password == "" {
		http.Error(w, "VNC password not configured", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"password": password,
	})
}

// handleGetVNCToken returns the auth token for WebSocket connections
// This is needed because HttpOnly cookies can't be accessed by JavaScript
// The frontend fetches this token and includes it in the websockify URL
func (s *Server) handleGetVNCToken(w http.ResponseWriter, r *http.Request) {
	// Extract token from Authorization header or cookies
	var tokenStr string

	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			tokenStr = parts[1]
		}
	}

	// Fallback to cookies
	if tokenStr == "" {
		if cookie, err := r.Cookie("jwt_token"); err == nil {
			tokenStr = cookie.Value
		} else if cookie, err := r.Cookie("auth_token"); err == nil {
			tokenStr = cookie.Value
		}
	}

	if tokenStr == "" {
		http.Error(w, "No auth token found", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenStr,
	})
}

// handleGetVNCDirectURL returns info about direct VNC connection availability
func (s *Server) handleGetVNCDirectURL(w http.ResponseWriter, r *http.Request) {
	// Direct connection not available in open source version
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": false,
		"message":   "Direct connection not configured",
	})
}

// generateSecurePassword creates a random alphanumeric password
func generateSecurePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a static password if random fails
		return "StableStream2026Abc"
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

// getLocalIP attempts to find the local IP address
func getLocalIP() string {
	// Try common methods to get IP

	// Method 1: Check environment variable (often set in containers/VMs)
	if ip := os.Getenv("SERVER_IP"); ip != "" {
		return ip
	}

	// Method 2: Use hostname command on Linux
	if runtime.GOOS == "linux" {
		cmd := exec.Command("hostname", "-I")
		out, err := cmd.Output()
		if err == nil {
			ips := strings.Fields(string(out))
			if len(ips) > 0 {
				return ips[0]
			}
		}
	}

	// Method 3: Default fallback
	return "localhost"
}

// ===========================================
// Multistream Handlers
// ===========================================

func (s *Server) handleGetMultistreamConfig(w http.ResponseWriter, r *http.Request) {
	if s.multistreamManager == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"available":        false,
			"enabled":          false,
			"max_destinations": 0,
		})
		return
	}

	config, err := s.multistreamManager.GetConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func (s *Server) handleEnableMultistream(w http.ResponseWriter, r *http.Request) {
	if s.multistreamManager == nil || !s.multistreamManager.IsAvailable() {
		http.Error(w, "Multistream is not available", http.StatusBadRequest)
		return
	}

	// Check if OBS is streaming
	streaming, err := s.obsManager.GetStreamStatus()
	if err != nil {
		http.Error(w, "Failed to check OBS status", http.StatusInternalServerError)
		return
	}
	if streaming {
		http.Error(w, "Cannot enable multistream while streaming", http.StatusBadRequest)
		return
	}

	if err := s.multistreamManager.Enable(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to enable multistream: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Multistream enabled"})
}

func (s *Server) handleDisableMultistream(w http.ResponseWriter, r *http.Request) {
	if s.multistreamManager == nil || !s.multistreamManager.IsAvailable() {
		http.Error(w, "Multistream is not available", http.StatusBadRequest)
		return
	}

	// Check if OBS is streaming
	streaming, err := s.obsManager.GetStreamStatus()
	if err != nil {
		http.Error(w, "Failed to check OBS status", http.StatusInternalServerError)
		return
	}
	if streaming {
		http.Error(w, "Cannot disable multistream while streaming", http.StatusBadRequest)
		return
	}

	if err := s.multistreamManager.Disable(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to disable multistream: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Multistream disabled"})
}

func (s *Server) handleListDestinations(w http.ResponseWriter, r *http.Request) {
	if s.multistreamManager == nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	dests, err := s.multistreamManager.ListDestinations()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list destinations: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Return empty array instead of null
	if dests == nil {
		json.NewEncoder(w).Encode([]struct{}{})
	} else {
		json.NewEncoder(w).Encode(dests)
	}
}

func (s *Server) handleAddDestination(w http.ResponseWriter, r *http.Request) {
	if s.multistreamManager == nil || !s.multistreamManager.IsAvailable() {
		http.Error(w, "Multistream is not available", http.StatusBadRequest)
		return
	}

	// Check if OBS is streaming - don't allow changes while live
	streaming, err := s.obsManager.GetStreamStatus()
	if err != nil {
		http.Error(w, "Failed to check OBS status", http.StatusInternalServerError)
		return
	}
	if streaming {
		http.Error(w, "Cannot add destinations while streaming", http.StatusBadRequest)
		return
	}

	var req struct {
		Name      string `json:"name"`
		RTMPURL   string `json:"rtmp_url"`
		StreamKey string `json:"stream_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.RTMPURL == "" || req.StreamKey == "" {
		http.Error(w, "Name, RTMP URL, and stream key are required", http.StatusBadRequest)
		return
	}

	id, err := s.multistreamManager.AddDestination(req.Name, req.RTMPURL, req.StreamKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to add destination: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "message": "Destination added"})
}

func (s *Server) handleRemoveDestination(w http.ResponseWriter, r *http.Request) {
	if s.multistreamManager == nil || !s.multistreamManager.IsAvailable() {
		http.Error(w, "Multistream is not available", http.StatusBadRequest)
		return
	}

	// Check if OBS is streaming - don't allow changes while live
	streaming, err := s.obsManager.GetStreamStatus()
	if err != nil {
		http.Error(w, "Failed to check OBS status", http.StatusInternalServerError)
		return
	}
	if streaming {
		http.Error(w, "Cannot remove destinations while streaming", http.StatusBadRequest)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid destination ID", http.StatusBadRequest)
		return
	}

	if err := s.multistreamManager.RemoveDestination(id); err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove destination: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Destination removed"})
}

func (s *Server) handleUpdateDestination(w http.ResponseWriter, r *http.Request) {
	if s.multistreamManager == nil || !s.multistreamManager.IsAvailable() {
		http.Error(w, "Multistream is not available", http.StatusBadRequest)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid destination ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Name      string `json:"name"`
		RTMPURL   string `json:"rtmp_url"`
		StreamKey string `json:"stream_key"`
		Enabled   bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.RTMPURL == "" || req.StreamKey == "" {
		http.Error(w, "Name, RTMP URL, and stream key are required", http.StatusBadRequest)
		return
	}

	if err := s.multistreamManager.UpdateDestination(id, req.Name, req.RTMPURL, req.StreamKey, req.Enabled); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update destination: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Destination updated"})
}

func (s *Server) handleMultistreamLogs(w http.ResponseWriter, r *http.Request) {
	// Read from ffmpeg push log and nginx error log
	logPaths := []string{
		"/var/log/nginx/ffmpeg_push.log",
		"/var/log/nginx/error.log",
	}

	var combinedLog string
	for _, path := range logPaths {
		content, err := os.ReadFile(path)
		if err != nil {
			combinedLog += fmt.Sprintf("--- %s ---\n[Could not read: %v]\n\n", path, err)
			continue
		}
		// Only include last 100KB of each log to prevent huge responses
		if len(content) > 100*1024 {
			content = content[len(content)-100*1024:]
		}
		combinedLog += fmt.Sprintf("--- %s ---\n%s\n\n", path, string(content))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"content": combinedLog})
}

// handleAppLogs returns application-level logs (auth, ingest, OBS events)
func (s *Server) handleAppLogs(w http.ResponseWriter, r *http.Request) {
	logPath := "logs/app.log"

	content, err := os.ReadFile(logPath)
	if err != nil {
		// Return empty if log doesn't exist yet
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"content": ""})
		return
	}

	// Only include last 100KB to prevent huge responses
	if len(content) > 100*1024 {
		content = content[len(content)-100*1024:]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"content": string(content)})
}

// handleGetSystemVersion returns current version and checks for updates from GitHub
func (s *Server) handleGetSystemVersion(w http.ResponseWriter, r *http.Request) {
	// Current version from embedded VERSION file
	versionBytes, err := versionFile.ReadFile("VERSION")
	currentVersion := "0.0.0"
	if err == nil {
		currentVersion = strings.TrimSpace(string(versionBytes))
	}

	const installPath = "/etc/stable-stream/install_path"

	// Check for latest version from GitHub
	latestVersion := ""
	updateAvailable := false

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/OHMEED/stable-streaming/releases/latest")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var release struct {
				TagName string `json:"tag_name"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&release); err == nil {
				latestVersion = strings.TrimPrefix(release.TagName, "v")
				updateAvailable = latestVersion != currentVersion && latestVersion != ""
			}
		}
	}

	// If no releases found, try to check commits
	if latestVersion == "" {
		installPathBytes, err := os.ReadFile(installPath)
		if err == nil {
			gitDir := strings.TrimSpace(string(installPathBytes))
			// Fetch from origin
			exec.Command("git", "-C", gitDir, "fetch", "origin", "main").Run()
			// Check if we're behind
			cmd := exec.Command("git", "-C", gitDir, "rev-list", "--count", "HEAD..origin/main")
			if output, err := cmd.Output(); err == nil {
				count := strings.TrimSpace(string(output))
				if count != "0" {
					updateAvailable = true
					latestVersion = currentVersion + " (" + count + " commits behind)"
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"current_version":  currentVersion,
		"latest_version":   latestVersion,
		"update_available": updateAvailable,
	})
}

// handleSystemUpdate triggers an update from GitHub
func (s *Server) handleSystemUpdate(w http.ResponseWriter, r *http.Request) {

	// Log the update attempt
	log.Printf("Update triggered from web UI")
	applog.Log("SYSTEM", "Update triggered from web UI")

	// Run update command in background via systemd-run to detach from the current service cgroup
	// This prevents the update process from being killed when the service stops
	cmd := exec.Command("sudo", "systemd-run", "--unit=stable-stream-updater", "--description=Stable Stream Updater", "stable-stream", "update", "--yes")
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start update via CLI: %v", err)
		http.Error(w, fmt.Sprintf("Failed to start update: %v", err), http.StatusInternalServerError)
		return
	}

	// Respond immediately - the script will restart the service
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "updating",
		"message": "Update started. The service will restart shortly.",
	})
}

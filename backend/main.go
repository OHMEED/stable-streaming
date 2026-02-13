package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"stable-stream-solutions/internal/applog"
	"stable-stream-solutions/internal/database"
	"stable-stream-solutions/internal/ingest"
	"stable-stream-solutions/internal/multistream"
	"stable-stream-solutions/internal/obs"
	"stable-stream-solutions/internal/server"
	"syscall"
)

func main() {
	// Parse command line flags
	jwtSecret := flag.String("jwt", "", "JWT Secret for local authentication (HS256)")
	jwtPublicKey := flag.String("public-key", "", "Path to RSA Public Key file (RS256)")
	multistreamEnabled := flag.Bool("multistream", false, "Enable multi-streaming support")
	maxStreams := flag.Int("max-streams", 0, "Maximum number of stream destinations (0 = disabled)")
	ingestsLocked := flag.Bool("ingests-locked", false, "Lock ingest creation (prevent adding new ingests)")
	flag.Parse()

	// Environment variable fallback
	if !*multistreamEnabled {
		val := os.Getenv("MULTISTREAM_ENABLED")
		if val == "y" || val == "true" || val == "1" {
			*multistreamEnabled = true
		}
	}
	if *maxStreams == 0 {
		if val := os.Getenv("MAX_MULTISTREAMS"); val != "" {
			var m int
			if n, err := fmt.Sscanf(val, "%d", &m); err == nil && n == 1 {
				*maxStreams = m
			}
		}
	}
	if !*ingestsLocked {
		val := os.Getenv("INGESTS_LOCKED")
		if val == "y" || val == "true" || val == "1" {
			*ingestsLocked = true
		}
	}

	// Set JWT Secret from flag if provided
	if *jwtSecret != "" {
		os.Setenv("JWT_SECRET", *jwtSecret)
	}

	// Set Public Key content (if path provided)
	if *jwtPublicKey != "" {
		content, err := os.ReadFile(*jwtPublicKey)
		if err != nil {
			log.Printf("Warning: Failed to read public key file: %v", err)
		} else {
			os.Setenv("JWT_PUBLIC_KEY", string(content))
		}
	}

	log.Println("Starting Stable Streaming Solutions Backend...")

	// Initialize application logger
	if err := applog.Init(); err != nil {
		log.Printf("Warning: Failed to initialize app logger: %v", err)
	}
	log.Println("App logger initialized")

	// Show multistream status
	if *multistreamEnabled && *maxStreams > 0 {
		log.Printf("🔀 Multi-streaming enabled (max %d destinations)", *maxStreams)
	}

	log.Println("Initializing database...")
	db, err := database.InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized")
	defer db.Close() // sql.DB is thread safe and long lived

	ingestManager := ingest.NewManager(db)

	// Start existing ingests on boot
	if err := ingestManager.StartAll(); err != nil {
		log.Printf("Failed to start saved ingests: %v", err)
	}

	// Log ports that need to be forwarded (after ingests are started)
	ingestManager.LogPortsToForward()

	obsManager := obs.NewManager()
	// Try to ensure running on boot? Or wait for user?
	// User req: "Upon boot if OBS is not running it should launch it"
	go func() {
		if err := obsManager.EnsureRunning(); err != nil {
			log.Printf("Failed to launch OBS: %v", err)
		} else {
			obsManager.Connect()
		}
	}()

	// Create multistream manager (maxStreams of 0 means disabled)
	multistreamManager := multistream.NewManager(db, obsManager, *maxStreams)

	// Start server in a goroutine
	srv := server.NewServer(db, ingestManager, obsManager, multistreamManager)

	// Initialize ingest lock state: command-line flag takes precedence, otherwise load from DB
	if *ingestsLocked {
		srv.SetIngestsLocked(true)
		log.Printf("🔒 Ingest creation LOCKED (via command-line flag)")
	} else {
		srv.InitIngestsLocked() // Load from DB if set previously
	}

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()
	log.Println("Server started on port 8080")

	// Wait for interrupt signal to gracefully shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down...")
	ingestManager.StopAll()
	obsManager.Stop()
	// db.Close() is defer-ed
	os.Exit(0)
}

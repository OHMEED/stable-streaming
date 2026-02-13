package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IPTracker tracks request counts and blocks for IP addresses
type IPTracker struct {
	mu       sync.RWMutex
	requests map[string]*ipRecord
	blocked  map[string]time.Time

	// Configuration
	MaxRequests   int           // Max requests per window
	WindowSize    time.Duration // Time window for counting requests
	BlockDuration time.Duration // How long to block after exceeding limit
}

type ipRecord struct {
	count     int
	windowEnd time.Time
}

// NewIPTracker creates a new rate limiter with specified limits
func NewIPTracker(maxRequests int, windowSize, blockDuration time.Duration) *IPTracker {
	tracker := &IPTracker{
		requests:      make(map[string]*ipRecord),
		blocked:       make(map[string]time.Time),
		MaxRequests:   maxRequests,
		WindowSize:    windowSize,
		BlockDuration: blockDuration,
	}

	// Start cleanup goroutine
	go tracker.cleanup()

	return tracker
}

// cleanup periodically removes expired entries
func (t *IPTracker) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		t.mu.Lock()
		now := time.Now()

		// Clean expired blocks
		for ip, unblockTime := range t.blocked {
			if now.After(unblockTime) {
				delete(t.blocked, ip)
			}
		}

		// Clean expired request windows
		for ip, record := range t.requests {
			if now.After(record.windowEnd) {
				delete(t.requests, ip)
			}
		}

		t.mu.Unlock()
	}
}

// IsBlocked checks if an IP is currently blocked
func (t *IPTracker) IsBlocked(ip string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if unblockTime, exists := t.blocked[ip]; exists {
		if time.Now().Before(unblockTime) {
			return true
		}
	}
	return false
}

// RecordRequest records a request from an IP and returns true if blocked
func (t *IPTracker) RecordRequest(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	// Check if already blocked
	if unblockTime, exists := t.blocked[ip]; exists {
		if now.Before(unblockTime) {
			return true // Still blocked
		}
		// Block expired, remove it
		delete(t.blocked, ip)
	}

	// Get or create record
	record, exists := t.requests[ip]
	if !exists || now.After(record.windowEnd) {
		// New window
		t.requests[ip] = &ipRecord{
			count:     1,
			windowEnd: now.Add(t.WindowSize),
		}
		return false
	}

	// Increment count
	record.count++

	// Check if exceeded limit
	if record.count > t.MaxRequests {
		// Block this IP
		t.blocked[ip] = now.Add(t.BlockDuration)
		delete(t.requests, ip)
		return true
	}

	return false
}

// RecordFailedLogin specifically tracks failed login attempts with stricter limits
func (t *IPTracker) RecordFailedLogin(ip string) bool {
	// For failed logins, we use stricter limits
	// Block after 5 failed attempts within window
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	// Check if already blocked
	if unblockTime, exists := t.blocked[ip]; exists {
		if now.Before(unblockTime) {
			return true
		}
		delete(t.blocked, ip)
	}

	key := "login:" + ip
	record, exists := t.requests[key]
	if !exists || now.After(record.windowEnd) {
		t.requests[key] = &ipRecord{
			count:     1,
			windowEnd: now.Add(5 * time.Minute), // 5 minute window for logins
		}
		return false
	}

	record.count++

	// Block after 5 failed login attempts
	if record.count >= 5 {
		t.blocked[ip] = now.Add(15 * time.Minute) // 15 minute block for login abuse
		delete(t.requests, key)
		return true
	}

	return false
}

// GetClientIP extracts the real client IP from the request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Middleware returns HTTP middleware that rate limits requests
func (t *IPTracker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := GetClientIP(r)

		// Check if blocked
		if t.IsBlocked(ip) {
			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
			return
		}

		// Record this request
		if t.RecordRequest(ip) {
			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// BlockedList returns a list of currently blocked IPs (for debugging/admin)
func (t *IPTracker) BlockedList() map[string]time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]time.Time)
	now := time.Now()
	for ip, unblockTime := range t.blocked {
		if now.Before(unblockTime) {
			result[ip] = unblockTime
		}
	}
	return result
}

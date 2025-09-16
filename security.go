package gostc

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

// CSRFProtection provides CSRF token validation for state-changing operations
type CSRFProtection struct {
	tokens    map[string]tokenInfo
	mu        sync.RWMutex
	tokenTTL  time.Duration
	cookieName string
}

type tokenInfo struct {
	token     string
	createdAt time.Time
}

// NewCSRFProtection creates a new CSRF protection middleware
func NewCSRFProtection(tokenTTL time.Duration) *CSRFProtection {
	cp := &CSRFProtection{
		tokens:     make(map[string]tokenInfo),
		tokenTTL:   tokenTTL,
		cookieName: "_csrf_token",
	}

	// Start cleanup goroutine
	go cp.cleanup()

	return cp
}

// GenerateToken creates a new CSRF token
func (cp *CSRFProtection) GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	token := base64.URLEncoding.EncodeToString(b)

	cp.mu.Lock()
	cp.tokens[token] = tokenInfo{
		token:     token,
		createdAt: time.Now(),
	}
	cp.mu.Unlock()

	return token, nil
}

// ValidateToken checks if a CSRF token is valid
func (cp *CSRFProtection) ValidateToken(token string) bool {
	if token == "" {
		return false
	}

	cp.mu.RLock()
	info, exists := cp.tokens[token]
	cp.mu.RUnlock()

	if !exists {
		return false
	}

	// Check if token is expired
	if time.Since(info.createdAt) > cp.tokenTTL {
		cp.mu.Lock()
		delete(cp.tokens, token)
		cp.mu.Unlock()
		return false
	}

	return true
}

// Middleware returns a middleware function for CSRF protection
func (cp *CSRFProtection) Middleware(safeMethod []string) func(http.Handler) http.Handler {
	safeMethodMap := make(map[string]bool)
	for _, method := range safeMethod {
		safeMethodMap[method] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF check for safe methods
			if safeMethodMap[r.Method] {
				next.ServeHTTP(w, r)
				return
			}

			// Get token from header or form
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("csrf_token")
			}

			if !cp.ValidateToken(token) {
				http.Error(w, "Invalid CSRF token", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// cleanup removes expired tokens periodically
func (cp *CSRFProtection) cleanup() {
	ticker := time.NewTicker(cp.tokenTTL / 2)
	defer ticker.Stop()

	for range ticker.C {
		cp.mu.Lock()
		now := time.Now()
		for token, info := range cp.tokens {
			if now.Sub(info.createdAt) > cp.tokenTTL {
				delete(cp.tokens, token)
			}
		}
		cp.mu.Unlock()
	}
}

// SecureCompare performs a constant-time comparison of two strings
func SecureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// IPRateLimiter provides more sophisticated rate limiting with burst support
type IPRateLimiter struct {
	limiters map[string]*TokenBucket
	mu       sync.RWMutex
	rate     int           // Tokens per second
	burst    int           // Maximum burst size
	ttl      time.Duration // TTL for inactive IPs
}

type TokenBucket struct {
	tokens    float64
	lastCheck time.Time
	mu        sync.Mutex
}

// NewIPRateLimiter creates a new IP-based rate limiter
func NewIPRateLimiter(rate, burst int, ttl time.Duration) *IPRateLimiter {
	rl := &IPRateLimiter{
		limiters: make(map[string]*TokenBucket),
		rate:     rate,
		burst:    burst,
		ttl:      ttl,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request from the given IP is allowed
func (rl *IPRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = &TokenBucket{
			tokens:    float64(rl.burst),
			lastCheck: time.Now(),
		}
		rl.limiters[ip] = limiter
	}
	rl.mu.Unlock()

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(limiter.lastCheck).Seconds()
	limiter.lastCheck = now

	// Add tokens based on elapsed time
	limiter.tokens += elapsed * float64(rl.rate)
	if limiter.tokens > float64(rl.burst) {
		limiter.tokens = float64(rl.burst)
	}

	// Check if request is allowed
	if limiter.tokens >= 1 {
		limiter.tokens--
		return true
	}

	return false
}

// cleanup removes inactive IP entries
func (rl *IPRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, limiter := range rl.limiters {
			limiter.mu.Lock()
			if now.Sub(limiter.lastCheck) > rl.ttl {
				delete(rl.limiters, ip)
			}
			limiter.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

// InputSanitizer provides methods to sanitize various input types
type InputSanitizer struct{}

// SanitizePath cleans and validates file paths
func (is *InputSanitizer) SanitizePath(path string) string {
	// Remove null bytes
	for i := 0; i < len(path); i++ {
		if path[i] == 0 {
			path = path[:i] + path[i+1:]
			i--
		}
	}

	// Limit length
	if len(path) > 2048 {
		path = path[:2048]
	}

	return path
}

// SanitizeHeader cleans HTTP header values
func (is *InputSanitizer) SanitizeHeader(value string) string {
	// Remove control characters
	clean := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] >= 32 && value[i] != 127 {
			clean = append(clean, value[i])
		}
	}
	return string(clean)
}
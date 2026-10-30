package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a simple per-IP sliding-window rate limiter.
// Not safe for multi-instance deployments; upgrade to Redis-backed limiter
// when running more than one backend instance.
// shortcut: in-memory map; reset on restart, no distributed coordination.
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string][]time.Time // IP -> request timestamps
	limit   int                    // max requests per window
	window  time.Duration
}

// newRateLimiter creates a rate limiter.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		clients: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

// Allow reports whether the request from the given IP is allowed.
func (r *rateLimiter) Allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Filter out timestamps outside the window
	ts := r.clients[ip]
	valid := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= r.limit {
		r.clients[ip] = valid
		return false
	}

	r.clients[ip] = append(valid, now)
	return true
}

// RateLimitMiddleware returns middleware that limits requests per client IP.
func RateLimitMiddleware(limit int, window time.Duration, next http.Handler) http.Handler {
	rl := newRateLimiter(limit, window)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = xff
		}
		if !rl.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    "RATE_LIMITED",
				"message": "too many requests, please try again later",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CORS returns middleware that adds Cross-Origin Resource Sharing headers.
// shortcut: allows all origins (*); upgrade: restrict to explicit origins in production.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-Correlation-ID, X-Org-ID, X-Project-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders returns middleware that adds browser security headers.
// shortcut: CSP is permissive for dev; upgrade: tighten CSP directives for production.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		// HSTS only makes sense over HTTPS; set it anyway so browsers remember
		// once the site is served over TLS.
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

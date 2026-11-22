package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	valkey "github.com/valkey-io/valkey-go"
)

// trustedNetworks holds parsed CIDRs from CCO_TRUSTED_PROXIES. Requests whose
// direct peer is in one of these networks may have their client IP taken from
// X-Forwarded-For; all others fall back to RemoteAddr.
var trustedNetworks []*net.IPNet

func init() {
	if v := os.Getenv("CCO_TRUSTED_PROXIES"); v != "" {
		for _, cidr := range strings.Split(v, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			_, netw, err := net.ParseCIDR(cidr)
			if err != nil {
				slog.Warn("invalid trusted proxy CIDR, ignoring", "cidr", cidr)
				continue
			}
			trustedNetworks = append(trustedNetworks, netw)
		}
	}
}

// isTrustedProxy reports whether the given address (host:port or IP) belongs
// to a configured trusted proxy network.
func isTrustedProxy(addr string) bool {
	if len(trustedNetworks) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, nw := range trustedNetworks {
		if nw.Contains(ip) {
			return true
		}
	}
	return false
}

// securityFailClosed reports whether Valkey/cache failures must deny requests
// instead of allowing them (fail-closed). Honoured by rate limiting and
// lockout. Controlled via CCO_SECURITY_FAIL_CLOSED; forced true when CCO_ENV=prod.
func securityFailClosed() bool {
	if os.Getenv("CCO_ENV") == "prod" {
		return true
	}
	return os.Getenv("CCO_SECURITY_FAIL_CLOSED") == "true"
}

// rateLimiter implements a simple per-IP sliding-window rate limiter.
// In-memory fallback for single-instance dev; state resets on restart.
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
// With vc != nil it uses a Valkey fixed-window counter shared across
// instances; with vc == nil it falls back to the in-memory limiter.
func RateLimitMiddleware(limit int, window time.Duration, vc valkey.Client, next http.Handler) http.Handler {
	if vc == nil {
		rl := newRateLimiter(limit, window)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.Allow(clientIP(r)) {
				writeRateLimited(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !valkeyAllow(r.Context(), vc, clientIP(r), limit, window, securityFailClosed()) {
			writeRateLimited(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP resolves the request IP. X-Forwarded-For is honoured only when the
// direct peer is a configured trusted proxy (CCO_TRUSTED_PROXIES); otherwise
// RemoteAddr is used, preventing IP spoofing for rate limiting.
func clientIP(r *http.Request) string {
	if isTrustedProxy(r.RemoteAddr) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the left-most (original client) address.
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
	}
	return r.RemoteAddr
}

// writeRateLimited sends the shared 429 JSON response.
func writeRateLimited(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    "RATE_LIMITED",
		"message": "too many requests, please try again later",
	})
}

// valkeyAllow implements the Valkey fixed-window counter.
// When failClosed is true (required in prod), any Valkey error denies the
// request instead of failing open.
func valkeyAllow(ctx context.Context, vc valkey.Client, ip string, limit int, window time.Duration, failClosed bool) bool {
	key := "cco:rl:" + ip
	n, err := vc.Do(ctx, vc.B().Incr().Key(key).Build()).ToInt64()
	if err != nil {
		if failClosed {
			slog.Warn("rate limiter: valkey error, denying request (fail-closed)", "err", err)
			return false
		}
		slog.Warn("rate limiter: valkey error, allowing request", "err", err)
		return true
	}
	if n == 1 {
		// First hit in the window: start the expiry; NX keeps concurrent
		// first hits from extending the window.
		if err := vc.Do(ctx, vc.B().Expire().Key(key).Seconds(int64(window.Seconds())).Nx().Build()).Error(); err != nil {
			if failClosed {
				slog.Warn("rate limiter: valkey expire failed, denying request (fail-closed)", "err", err)
				vc.Do(ctx, vc.B().Del().Key(key).Build())
				return false
			}
			slog.Warn("rate limiter: valkey expire failed, allowing request", "err", err)
			vc.Do(ctx, vc.B().Del().Key(key).Build())
			return true
		}
	}
	return n <= int64(limit)
}

// CORS returns middleware that adds Cross-Origin Resource Sharing headers.
// With no explicit origins, no Access-Control-Allow-Origin header is emitted
// (callers must configure an allowlist; do not default to "*").
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.ToLower(strings.TrimSuffix(o, "/"))] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Add("Vary", "Origin")
			if origin := r.Header.Get("Origin"); allowed[strings.ToLower(origin)] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-Correlation-ID, X-Org-ID, X-Project-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Correlation-ID, X-Operation-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders returns middleware that adds browser security headers.
// Default CSP is dev-permissive; production sets CCO_CSP.
func SecurityHeaders(csp string, next http.Handler) http.Handler {
	if csp == "" {
		csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", csp)
		// HSTS only makes sense over HTTPS; set it anyway so browsers remember
		// once the site is served over TLS.
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

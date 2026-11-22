package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	valkey "github.com/valkey-io/valkey-go"
)

// newTestValkey starts an in-server miniredis and connects a valkey client to it.
func newTestValkey(t *testing.T) valkey.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	// miniredis has no CLIENT TRACKING; disable client-side caching for tests.
	vc, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{mr.Addr()}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(vc.Close)
	return vc
}

// send sends a GET as the given IP and returns the status code.
func send(h http.Handler, ip string) int {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ip != "" {
		req.RemoteAddr = ip + ":12345"
		req.Header.Set("X-Forwarded-For", ip)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func assertCounts(t *testing.T, h http.Handler, limit int, ip string) {
	t.Helper()
	for i := 0; i < limit; i++ {
		if code := send(h, ip); code != http.StatusOK {
			t.Fatalf("request %d from %s: got %d, want 200", i+1, ip, code)
		}
	}
	for i := 0; i < 2; i++ {
		if code := send(h, ip); code != http.StatusTooManyRequests {
			t.Fatalf("over-limit request %d from %s: got %d, want 429", i+1, ip, code)
		}
	}
}

func TestRateLimitValkey(t *testing.T) {
	vc := newTestValkey(t)
	const limit = 3
	var calls int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ })

	h := RateLimitMiddleware(limit, time.Minute, vc, next)
	assertCounts(t, h, limit, "10.0.0.1")
	if calls != limit {
		t.Fatalf("backend got %d calls, want %d", calls, limit)
	}

	// A different IP is unaffected by the first IP's counter.
	if code := send(h, "10.0.0.2"); code != http.StatusOK {
		t.Fatalf("request from second IP: got %d, want 200", code)
	}
}

func TestRateLimitInMemoryFallback(t *testing.T) {
	const limit = 3
	var calls int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ })

	h := RateLimitMiddleware(limit, time.Minute, nil, next)
	assertCounts(t, h, limit, "10.0.0.1")
	if calls != limit {
		t.Fatalf("backend got %d calls, want %d", calls, limit)
	}
	if code := send(h, "10.0.0.2"); code != http.StatusOK {
		t.Fatalf("request from second IP: got %d, want 200", code)
	}
}

func TestSecurityHeadersCSP(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	custom := "default-src 'none'; frame-ancestors 'none'"
	rec := httptest.NewRecorder()
	SecurityHeaders(custom, next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Content-Security-Policy"); got != custom {
		t.Fatalf("custom CSP: got %q, want %q", got, custom)
	}

	def := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'"
	rec = httptest.NewRecorder()
	SecurityHeaders("", next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Content-Security-Policy"); got != def {
		t.Fatalf("default CSP: got %q, want %q", got, def)
	}
}

func TestCORSExposesTraceHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	CORS(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(
		rec,
		httptest.NewRequest(http.MethodGet, "/", nil),
	)
	if got, want := rec.Header().Get("Access-Control-Expose-Headers"), "X-Request-ID, X-Correlation-ID, X-Operation-ID"; got != want {
		t.Fatalf("exposed headers got %q, want %q", got, want)
	}
}

func TestNormalizeTraceID(t *testing.T) {
	t.Run("canonicalizes valid UUID", func(t *testing.T) {
		id, invalid := iam.NormalizeTraceID("550E8400-E29B-41D4-A716-446655440000")
		if invalid {
			t.Fatal("valid UUID marked invalid")
		}
		if got, want := id.String(), "550e8400-e29b-41d4-a716-446655440000"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("replaces invalid value", func(t *testing.T) {
		id, invalid := iam.NormalizeTraceID("not-a-uuid")
		if !invalid {
			t.Fatal("invalid UUID not marked invalid")
		}
		if _, err := uuid.Parse(id.String()); err != nil {
			t.Fatalf("replacement is not a UUID: %v", err)
		}
	})

	t.Run("generates missing value", func(t *testing.T) {
		id, invalid := iam.NormalizeTraceID("")
		if invalid {
			t.Fatal("missing UUID marked invalid")
		}
		if _, err := uuid.Parse(id.String()); err != nil {
			t.Fatalf("generated value is not a UUID: %v", err)
		}
	})
}

func TestNormalizeTraceIDsReplacesInvalidHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "invalid-request-id")
	req.Header.Set("X-Correlation-ID", "invalid-correlation-id")

	requestID, correlationID := normalizeTraceIDs(req)
	if got := req.Header.Get("X-Request-ID"); got != requestID.String() {
		t.Fatalf("request header got %q, want normalized UUID %q", got, requestID)
	}
	if got := req.Header.Get("X-Correlation-ID"); got != correlationID.String() {
		t.Fatalf("correlation header got %q, want normalized UUID %q", got, correlationID)
	}
	for name, value := range map[string]string{
		"request":     requestID.String(),
		"correlation": correlationID.String(),
	} {
		if _, err := uuid.Parse(value); err != nil {
			t.Fatalf("normalized %s ID is not a UUID: %v", name, err)
		}
	}
}

func TestNormalizeTraceIDsDefaultsCorrelationToRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "550E8400-E29B-41D4-A716-446655440000")

	requestID, correlationID := normalizeTraceIDs(req)
	if requestID != correlationID {
		t.Fatalf("request ID %s differs from default correlation ID %s", requestID, correlationID)
	}
	if got, want := req.Header.Get("X-Correlation-ID"), requestID.String(); got != want {
		t.Fatalf("correlation header got %q, want %q", got, want)
	}
}

package iam

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// IdentityContextFromRequest extracts IdentityContext from request context.
// Returns zero value and false if not authenticated.
func IdentityContextFromRequest(r *http.Request) (IdentityContext, bool) {
	return IdentityContextFromContext(r.Context())
}

// NewIdentityContext creates an IdentityContext with defaults.
func NewIdentityContext(userID uuid.UUID, email, displayName, authMethod string) IdentityContext {
	now := time.Now().UnixMilli()
	return IdentityContext{
		UserID:         userID,
		Email:          email,
		DisplayName:    displayName,
		AuthMethod:     authMethod,
		SystemRoles:    []string{},
		OrgID:          uuid.Nil,
		OrgRole:        "viewer",
		ProjectID:      nil,
		ProjectRole:    nil,
		Permissions:    []string{},
		RequestID:      uuid.New().String(),
		CorrelationID:  uuid.New().String(),
		TokenExpiresAt: now + 15*60*1000, // 15 min default
	}
}

// NormalizeTraceID parses a supplied trace ID or generates a UUID when it is
// missing or invalid. The bool reports whether a non-empty value was invalid.
func NormalizeTraceID(value string) (uuid.UUID, bool) {
	if value == "" {
		return uuid.New(), false
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.New(), true
	}
	return id, false
}

// context key types are unexported to prevent collisions.
type ctxKey string

const (
	ctxIdentityKey        ctxKey = "iam.identity"
	ctxIdentityCaptureKey ctxKey = "iam.identity_capture"
	ctxRequestID          ctxKey = "iam.request_id"
	ctxTokenExpiry        ctxKey = "iam.token_expiry"
)

type IdentityCapture struct {
	mu       sync.RWMutex
	identity IdentityContext
	present  bool
}

func NewIdentityCapture() *IdentityCapture {
	return &IdentityCapture{}
}

func WithIdentityCapture(ctx context.Context, capture *IdentityCapture) context.Context {
	return context.WithValue(ctx, ctxIdentityCaptureKey, capture)
}

func (c *IdentityCapture) Identity() (IdentityContext, bool) {
	if c == nil {
		return IdentityContext{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.identity, c.present
}

// WithIdentityContext returns a new context with the IdentityContext embedded.
func WithIdentityContext(ctx context.Context, ic IdentityContext) context.Context {
	if capture, ok := ctx.Value(ctxIdentityCaptureKey).(*IdentityCapture); ok {
		capture.mu.Lock()
		capture.identity = ic
		capture.present = true
		capture.mu.Unlock()
	}
	return context.WithValue(ctx, ctxIdentityKey, ic)
}

// IdentityContextFromContext retrieves the IdentityContext from context.
// Returns zero value and false if not present.
func IdentityContextFromContext(ctx context.Context) (IdentityContext, bool) {
	ic, ok := ctx.Value(ctxIdentityKey).(IdentityContext)
	return ic, ok
}

// WithRequestID returns a new context with request ID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxRequestID, requestID)
}

// RequestIDFromContext retrieves request ID from context.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(ctxRequestID)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// TokenExpiryFromContext retrieves token expiry from context (set by middleware).
func TokenExpiryFromContext(ctx context.Context) (int64, bool) {
	v := ctx.Value(ctxTokenExpiry)
	if v == nil {
		return 0, false
	}
	i, ok := v.(int64)
	return i, ok
}

// ParseTokenExpiry parses "exp" claim (unix seconds) to milliseconds.
func ParseTokenExpiry(exp interface{}) (int64, error) {
	switch v := exp.(type) {
	case float64:
		return int64(v * 1000), nil
	case int64:
		return v * 1000, nil
	case string:
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, err
		}
		return i * 1000, nil
	default:
		return 0, nil
	}
}

// IsTokenExpired checks if token is expired (with 30s clock skew buffer).
func IsTokenExpired(expiresAt int64) bool {
	return time.Now().UnixMilli() > expiresAt-30_000
}

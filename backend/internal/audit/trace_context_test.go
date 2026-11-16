package audit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

func TestCorrelationContextHelpers(t *testing.T) {
	operationID := uuid.New()
	want := CorrelationContext{
		RequestID:     uuid.New(),
		CorrelationID: uuid.New(),
		OperationID:   &operationID,
	}
	ctx := WithCorrelationContext(context.Background(), want)

	got, ok := CorrelationContextFromContext(ctx)
	if !ok {
		t.Fatal("expected trace context")
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestAuditIdentityContextUsesIAMIdentityAndMiddlewareOperation(t *testing.T) {
	operationID := uuid.New()
	requestID := uuid.New()
	correlationID := uuid.New()
	userID := uuid.New()
	organizationID := uuid.New()
	ctx := WithCorrelationContext(context.Background(), CorrelationContext{
		RequestID:     uuid.New(),
		CorrelationID: uuid.New(),
		OperationID:   &operationID,
	})
	ctx = iam.WithIdentityContext(ctx, iam.IdentityContext{
		UserID:        userID,
		OrgID:         organizationID,
		AuthMethod:    "managed_identity",
		RequestID:     requestID.String(),
		CorrelationID: correlationID.String(),
	})

	got, err := AuditIdentityContextFromContext(ctx)
	if err != nil {
		t.Fatalf("build audit identity: %v", err)
	}
	if got.Actor.UserID == nil || *got.Actor.UserID != userID {
		t.Fatalf("unexpected actor user ID: %#v", got.Actor.UserID)
	}
	if got.Actor.ActorType != ActorTypeManagedIdentity || got.Actor.AuthMethod != "managed_identity" {
		t.Fatalf("unexpected actor: %#v", got.Actor)
	}
	if got.OrganizationID == nil || *got.OrganizationID != organizationID {
		t.Fatalf("unexpected organization ID: %#v", got.OrganizationID)
	}
	if got.Trace.RequestID != requestID || got.Trace.CorrelationID != correlationID {
		t.Fatalf("trace IDs should come from IAM: %#v", got.Trace)
	}
	if got.Trace.OperationID == nil || *got.Trace.OperationID != operationID {
		t.Fatalf("operation ID should come from trace middleware: %#v", got.Trace.OperationID)
	}
}

func TestAuditIdentityContextRequiresIAM(t *testing.T) {
	_, err := AuditIdentityContextFromContext(context.Background())
	if !errors.Is(err, ErrIdentityContextMissing) {
		t.Fatalf("got %v, want ErrIdentityContextMissing", err)
	}
}

func TestAuditIdentityContextRejectsInvalidIAMTraceIDs(t *testing.T) {
	ctx := iam.WithIdentityContext(context.Background(), iam.IdentityContext{
		UserID:        uuid.New(),
		RequestID:     "invalid",
		CorrelationID: uuid.NewString(),
	})
	_, err := AuditIdentityContextFromContext(ctx)
	if !errors.Is(err, ErrInvalidTraceContext) {
		t.Fatalf("got %v, want ErrInvalidTraceContext", err)
	}
}

func TestTraceMiddlewareNormalizesAndReturnsTraceHeaders(t *testing.T) {
	var handlerTrace CorrelationContext
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		handlerTrace, ok = CorrelationContextFromContext(r.Context())
		if !ok {
			t.Error("trace context missing in handler")
		}
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "invalid")
	rec := httptest.NewRecorder()

	TraceMiddleware(next).ServeHTTP(rec, req)

	if _, err := uuid.Parse(rec.Header().Get("X-Request-ID")); err != nil {
		t.Fatalf("response request ID is not a UUID: %v", err)
	}
	if _, err := uuid.Parse(rec.Header().Get("X-Correlation-ID")); err != nil {
		t.Fatalf("response correlation ID is not a UUID: %v", err)
	}
	if _, err := uuid.Parse(rec.Header().Get("X-Operation-ID")); err != nil {
		t.Fatalf("response operation ID is not a UUID: %v", err)
	}
	if handlerTrace.RequestID.String() != rec.Header().Get("X-Request-ID") {
		t.Fatalf("handler request ID %s differs from response header", handlerTrace.RequestID)
	}
	if handlerTrace.CorrelationID != handlerTrace.RequestID {
		t.Fatalf("missing correlation ID should reuse request ID, got %#v", handlerTrace)
	}
	if handlerTrace.OperationID == nil || handlerTrace.OperationID.String() != rec.Header().Get("X-Operation-ID") {
		t.Fatalf("handler operation ID differs from response header: %#v", handlerTrace.OperationID)
	}
}

func TestTraceMiddlewarePreservesCorrelationID(t *testing.T) {
	correlationID := uuid.New()
	var got CorrelationContext
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = CorrelationContextFromContext(r.Context())
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Correlation-ID", correlationID.String())
	rec := httptest.NewRecorder()

	TraceMiddleware(next).ServeHTTP(rec, req)

	if got.CorrelationID != correlationID {
		t.Fatalf("got correlation ID %s, want %s", got.CorrelationID, correlationID)
	}
	if rec.Header().Get("X-Correlation-ID") != correlationID.String() {
		t.Fatalf("response correlation ID got %q, want %q", rec.Header().Get("X-Correlation-ID"), correlationID)
	}
}

package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

type auditRecorder []AuditEvent

func (r *auditRecorder) Record(_ context.Context, event AuditEvent) error {
	*r = append(*r, event)
	return nil
}

func TestTraceMiddlewareRecordsAcquisitionLifecycleWithIdentityAndTrace(t *testing.T) {
	recorder := &auditRecorder{}
	userID, organizationID := uuid.New(), uuid.New()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		correlationID := r.Header.Get("X-Correlation-ID")
		identity := iam.IdentityContext{
			UserID: userID, OrgID: organizationID, AuthMethod: "oidc",
			RequestID: requestID, CorrelationID: correlationID,
		}
		r = r.WithContext(iam.WithIdentityContext(r.Context(), identity))
		w.WriteHeader(http.StatusOK)
	})
	handler := TraceMiddleware(next, recorder)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/acquisitions/run", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("X-Request-ID") == "" ||
		response.Header().Get("X-Correlation-ID") == "" ||
		response.Header().Get("X-Operation-ID") == "" {
		t.Fatalf("trace response headers missing: %#v", response.Header())
	}
	if len(*recorder) != 2 {
		t.Fatalf("recorded %d events, want started and completed events", len(*recorder))
	}
	if (*recorder)[0].Action != "acquisition_started" ||
		(*recorder)[1].Action != "acquisition_completed" {
		t.Fatalf("unexpected lifecycle actions: %q, %q", (*recorder)[0].Action, (*recorder)[1].Action)
	}
	for _, event := range *recorder {
		if event.Actor.UserID == nil || *event.Actor.UserID != userID ||
			event.OrganizationID == nil || *event.OrganizationID != organizationID {
			t.Errorf("actor or organization missing: %+v", event)
		}
		if event.Trace.RequestID == uuid.Nil || event.Trace.CorrelationID == uuid.Nil ||
			event.Trace.OperationID == nil || *event.Trace.OperationID == uuid.Nil {
			t.Errorf("trace identifiers missing: %+v", event.Trace)
		}
		for _, forbidden := range []string{"password", "token", "credential", "connection_string"} {
			if _, exists := event.Metadata[forbidden]; exists {
				t.Errorf("event metadata contains forbidden field %q", forbidden)
			}
		}
	}
	if (*recorder)[0].Trace.OperationID == nil ||
		*(*recorder)[0].Trace.OperationID != *(*recorder)[1].Trace.OperationID {
		t.Fatal("start and completion events must share an operation ID")
	}
}

func TestRequestAuditPlanClassifiesFailedLoginWithoutCredentialCapture(t *testing.T) {
	plan := requestAuditPlan(http.MethodPost, "/auth/login")
	if plan.successAction != "user_login_succeeded" || plan.failureAction != "user_login_failed" {
		t.Fatalf("login action mapping = %+v", plan)
	}
}

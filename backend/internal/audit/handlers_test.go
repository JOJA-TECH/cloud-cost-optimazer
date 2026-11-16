package audit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

type auditQueryStub struct {
	filter AuditFilter
	event  AuditEvent
	err    error
}

func (s *auditQueryStub) GetEvent(context.Context, int64) (AuditEvent, error) {
	return s.event, s.err
}

func (s *auditQueryStub) ListEvents(_ context.Context, filter AuditFilter) ([]AuditEvent, error) {
	s.filter = filter
	return []AuditEvent{s.event}, s.err
}

func (s *auditQueryStub) GetResourceHistory(_ context.Context, _ ResourceReference, filter AuditFilter) ([]AuditEvent, error) {
	s.filter = filter
	return []AuditEvent{s.event}, s.err
}

func (s *auditQueryStub) GetCorrelationHistory(context.Context, uuid.UUID) ([]AuditEvent, error) {
	return []AuditEvent{s.event}, s.err
}

type auditPermissionStub map[string]bool

func (s auditPermissionStub) Can(_ context.Context, _ iam.IdentityContext, resource, action string) (bool, error) {
	return s[resource+":"+action], nil
}

func TestAuditListForcesActiveOrganizationScope(t *testing.T) {
	orgID := uuid.New()
	query := &auditQueryStub{}
	permissions := auditPermissionStub{
		"audit:read":              true,
		"audit:read:organization": true,
	}
	identity := iam.IdentityContext{UserID: uuid.New(), OrgID: orgID}
	ctx := iam.WithIdentityContext(context.Background(), identity)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?organization_id="+orgID.String(), nil)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	handleListEvents(query, permissions).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if query.filter.OrganizationID == nil || *query.filter.OrganizationID != orgID {
		t.Fatalf("query organization scope = %v, want %s", query.filter.OrganizationID, orgID)
	}

	otherOrgID := uuid.New()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?organization_id="+otherOrgID.String(), nil)
	request = request.WithContext(ctx)
	response = httptest.NewRecorder()
	handleListEvents(query, permissions).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("mismatched organization status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAuditListOwnScopeForcesCurrentUser(t *testing.T) {
	userID := uuid.New()
	query := &auditQueryStub{}
	permissions := auditPermissionStub{
		"audit:read:own": true,
	}
	ctx := iam.WithIdentityContext(context.Background(), iam.IdentityContext{UserID: userID})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil).WithContext(ctx)
	response := httptest.NewRecorder()

	handleListEvents(query, permissions).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if query.filter.UserID == nil || *query.filter.UserID != userID {
		t.Fatalf("query user scope = %v, want %s", query.filter.UserID, userID)
	}
}

func TestAuditEventDetailHidesEventsOutsideScope(t *testing.T) {
	query := &auditQueryStub{event: AuditEvent{EventID: 8}}
	permissions := auditPermissionStub{
		"audit:read":              true,
		"audit:read:organization": true,
	}
	ctx := iam.WithIdentityContext(context.Background(), iam.IdentityContext{
		UserID: uuid.New(),
		OrgID:  uuid.New(),
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events/8", nil).WithContext(ctx)
	request.SetPathValue("event_id", "8")
	response := httptest.NewRecorder()

	handleGetEvent(query, permissions).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestAuditQueryErrorDoesNotExposeStorageDetails(t *testing.T) {
	query := &auditQueryStub{err: errors.New("db password or connection string")}
	permissions := auditPermissionStub{
		"audit:read":              true,
		"audit:read:organization": true,
	}
	orgID := uuid.New()
	ctx := iam.WithIdentityContext(context.Background(), iam.IdentityContext{
		UserID: uuid.New(),
		OrgID:  orgID,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil).WithContext(ctx)
	response := httptest.NewRecorder()

	handleListEvents(query, permissions).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := response.Body.String(); containsAny(got, "password", "connection string") {
		t.Fatalf("response leaked storage error: %s", got)
	}
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

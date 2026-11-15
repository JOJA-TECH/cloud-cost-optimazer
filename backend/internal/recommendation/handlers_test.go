package recommendation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/google/uuid"
)

func TestHandlersReturnStorageUnavailableWithoutService(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"generate", http.MethodPost, handleGenerate(nil)},
		{"evaluate", http.MethodPost, handleEvaluate(nil)},
		{"evaluate existing", http.MethodPost, handleEvaluateExisting(nil)},
		{"list", http.MethodGet, handleList(nil)},
		{"get", http.MethodGet, handleGet(nil)},
		{"evidence", http.MethodGet, handleEvidence(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, "/", nil)
			test.handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatalf("decode response body: %v", err)
			}
			if body.Code != "RECOMMENDATION_STORAGE_UNAVAILABLE" {
				t.Fatalf("response code = %q, want RECOMMENDATION_STORAGE_UNAVAILABLE", body.Code)
			}
		})
	}
}

type handlerTokenValidator struct {
	identity iam.IdentityContext
}

func (v handlerTokenValidator) Validate(context.Context, string) (iam.IdentityContext, error) {
	return v.identity, nil
}

type handlerIdentityRepository struct {
	iam.IdentityRepository
}

func (handlerIdentityRepository) ListUserRolesByUser(context.Context, uuid.UUID) ([]iam.UserRoleDTO, error) {
	return nil, nil
}

type handlerRecommendationRepository struct {
	RecommendationRepository
	recommendation Recommendation
	err            error
}

func (r handlerRecommendationRepository) GetRecommendation(context.Context, uuid.UUID) (Recommendation, error) {
	return r.recommendation, r.err
}

func TestRecommendationRoutesRequireAuthenticationAndPermission(t *testing.T) {
	mux := http.NewServeMux()
	repo := handlerIdentityRepository{}
	identity := iam.IdentityContext{
		UserID: uuid.New(), TokenExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}
	RegisterRoutes(mux, nil, middleware.NewAuthMiddleware(handlerTokenValidator{identity: identity}, repo), iam.NewPermissionEngine(nil))

	unauthenticated := httptest.NewRecorder()
	mux.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/v1/recommendations", strings.NewReader("{}")))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticated.Code, http.StatusUnauthorized)
	}

	forbidden := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer "+uuid.NewString())
	mux.ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer generation status = %d, want %d", forbidden.Code, http.StatusForbidden)
	}
}

func TestRecommendationHandlersReturnBadRequestAndNotFound(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
		want    int
	}{
		{
			name: "invalid uid", handler: handleGet(&Service{}),
			path: "/api/v1/recommendations/not-a-uuid", want: http.StatusBadRequest,
		},
		{
			name: "missing recommendation",
			handler: handleGet(&Service{repository: handlerRecommendationRepository{
				err: ErrRecommendationNotFound,
			}}),
			path: "/api/v1/recommendations/" + uuid.NewString(), want: http.StatusNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if strings.Contains(test.path, "not-a-uuid") {
				request.SetPathValue("uid", "not-a-uuid")
			} else {
				request.SetPathValue("uid", strings.TrimPrefix(test.path, "/api/v1/recommendations/"))
			}
			test.handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestGenerateHandlerMapsPolicyConflictTo409(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	policy.Enabled = false
	service.policyRepository = engineTestPolicyRepository{policy: policy}
	now := time.Now().UTC()
	body, err := json.Marshal(generateRequest{
		ResourceUID: resourceUID.String(), PolicyUID: policy.UID.String(),
		HorizonDays: 30, From: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		To: now.Format(time.RFC3339), Currency: "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations", strings.NewReader(string(body)))
	handleGenerate(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}

func TestEvaluateHandlerReturnsOKForAuditableNoAction(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	service.costs = engineTestCosts(nil)
	analyticsReader := service.analytics.(engineTestAnalytics)
	analyticsReader.cost.ActualMonthlyCost = 0
	service.analytics = analyticsReader
	now := time.Now().UTC()
	body, err := json.Marshal(generateRequest{
		ResourceUID: resourceUID.String(), PolicyUID: policy.UID.String(),
		HorizonDays: 30, From: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		To: now.Format(time.RFC3339), Currency: "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/evaluate", strings.NewReader(string(body)))
	handleEvaluate(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode evaluation response: %v", err)
	}
	if response.State != "insufficient_information" {
		t.Fatalf("evaluation state = %q, want auditable no-action state", response.State)
	}
}

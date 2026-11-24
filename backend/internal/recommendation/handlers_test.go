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

type handlerTraceReader struct {
	filter RecommendationTraceFilter
	events []RecommendationTraceEvent
	err    error
}

func (r *handlerTraceReader) ListRecommendationTraceEvents(
	_ context.Context,
	_ uuid.UUID,
	filter RecommendationTraceFilter,
) ([]RecommendationTraceEvent, error) {
	r.filter = filter
	return r.events, r.err
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

	for _, action := range []string{"accept", "reject", "dismiss", "apply"} {
		t.Run(action+" requires write permission", func(t *testing.T) {
			statusForbidden := httptest.NewRecorder()
			statusRequest := httptest.NewRequest(http.MethodPost,
				"/api/v1/recommendations/"+uuid.NewString()+"/"+action, nil)
			statusRequest.Header.Set("Authorization", "Bearer "+uuid.NewString())
			mux.ServeHTTP(statusForbidden, statusRequest)
			if statusForbidden.Code != http.StatusForbidden {
				t.Fatalf("status update without recommendations:write returned %d, want %d",
					statusForbidden.Code, http.StatusForbidden)
			}
		})
	}
}

func TestRecommendationStatusHandlerUpdatesAuthorizedRecommendation(t *testing.T) {
	uid, resourceUID, actorID := uuid.New(), uuid.New(), uuid.New()
	repository := &statusRepositoryStub{value: Recommendation{
		UID: uid, ResourceUID: resourceUID, ScenarioUID: uuid.New(),
		PolicyUID: uuid.New(), Status: RecommendationStatusProposed,
	}}
	sink := &statusAuditSink{}
	service, err := NewEngine(Dependencies{
		PolicyEvaluator: policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
			return PolicyEvaluation{}, nil
		}),
		Repository: repository,
		Audit:      sink,
	})
	if err != nil {
		t.Fatalf("create recommendation service: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/"+uid.String()+"/accept", nil)
	request.SetPathValue("uid", uid.String())
	request = request.WithContext(iam.WithIdentityContext(request.Context(), iam.IdentityContext{UserID: actorID}))
	recorder := httptest.NewRecorder()

	handleStatusAction(service, RecommendationStatusAccepted).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Status    RecommendationStatus `json:"status"`
		DecidedBy *string              `json:"decided_by"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != RecommendationStatusAccepted ||
		response.DecidedBy == nil || *response.DecidedBy != actorID.String() {
		t.Fatalf("unexpected status response: %+v", response)
	}
	if len(sink.events) != 1 || sink.events[0].Action != "recommendation_accepted" {
		t.Fatalf("status handler did not audit the decision: %#v", sink.events)
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

func TestRecommendationTraceReturnsDecisionAndScopedEvents(t *testing.T) {
	orgID := uuid.New()
	recommendation := Recommendation{
		UID: uuid.New(), ResourceUID: uuid.New(), ScenarioUID: uuid.New(), PolicyUID: uuid.New(),
		Status:          RecommendationStatusProposed,
		Risk:            RecommendationRisk{Score: 0.21, Level: RiskLevelLow},
		Confidence:      RecommendationConfidence{Score: 0.87},
		EstimatedSaving: EstimatedSaving{MonthlyAmount: 125.5, Currency: "USD"},
	}
	service := &Service{repository: handlerRecommendationRepository{recommendation: recommendation}}
	reader := &handlerTraceReader{events: []RecommendationTraceEvent{
		{EventID: 101, Action: "recommendation_evaluation_started", OccurredAt: time.Now().UTC()},
		{EventID: 102, Action: "recommendation_generated", OccurredAt: time.Now().UTC()},
	}}
	identity := iam.IdentityContext{UserID: uuid.New(), OrgID: orgID}
	ctx := iam.WithIdentityContext(context.Background(), identity)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations/"+recommendation.UID.String()+"/trace", nil).
		WithContext(ctx)
	request.SetPathValue("uid", recommendation.UID.String())
	response := httptest.NewRecorder()

	handleTrace(service, reader).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if reader.filter.OrganizationID == nil || *reader.filter.OrganizationID != orgID {
		t.Fatalf("trace query organization scope = %v, want %s", reader.filter.OrganizationID, orgID)
	}
	var body struct {
		RecommendationUID uuid.UUID `json:"recommendation_uid"`
		ResourceUID       uuid.UUID `json:"resource_uid"`
		Status            string    `json:"status"`
		Decision          string    `json:"decision"`
		Risk              struct {
			Score float64 `json:"score"`
			Level string  `json:"level"`
		} `json:"risk"`
		Confidence struct {
			Score float64 `json:"score"`
		} `json:"confidence"`
		EstimatedSaving struct {
			MonthlyAmount float64 `json:"monthly_amount"`
			Currency      string  `json:"currency"`
		} `json:"estimated_saving"`
		Events []RecommendationTraceEvent `json:"events"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RecommendationUID != recommendation.UID || body.ResourceUID != recommendation.ResourceUID ||
		body.Status != string(RecommendationStatusProposed) || body.Decision != "recommendation_generated" ||
		body.Risk.Score != 0.21 || body.Risk.Level != string(RiskLevelLow) ||
		body.Confidence.Score != 0.87 || body.EstimatedSaving.MonthlyAmount != 125.5 ||
		body.EstimatedSaving.Currency != "USD" || len(body.Events) != 2 {
		t.Fatalf("trace response shape or values mismatch: %+v", body)
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

type handlerPolicyRepositoryStub struct {
	policies []OptimizationPolicy
	created  OptimizationPolicy
}

func (r *handlerPolicyRepositoryStub) CreatePolicy(_ context.Context, policy OptimizationPolicy) (OptimizationPolicy, error) {
	if policy.UID == uuid.Nil {
		policy.UID = uuid.New()
	}
	r.created = policy
	return policy, nil
}

func (r *handlerPolicyRepositoryStub) GetPolicy(context.Context, uuid.UUID) (OptimizationPolicy, error) {
	return OptimizationPolicy{}, ErrPolicyNotFound
}

func (r *handlerPolicyRepositoryStub) GetLatestEnabledPolicy(context.Context, string) (OptimizationPolicy, error) {
	return OptimizationPolicy{}, ErrPolicyNotFound
}

func (r *handlerPolicyRepositoryStub) ListPolicies(context.Context, string, bool) ([]OptimizationPolicy, error) {
	return r.policies, nil
}

func TestPolicyHandlersReadAndCreateUsingExistingRepository(t *testing.T) {
	actorID := uuid.New()
	policyRepository := &handlerPolicyRepositoryStub{policies: []OptimizationPolicy{{
		UID: uuid.New(), Name: "safe", Version: 1, Enabled: true,
	}}}
	service, err := NewEngine(Dependencies{
		PolicyEvaluator: policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
			return PolicyEvaluation{}, nil
		}),
		PolicyRepository: policyRepository,
		Repository:       handlerRecommendationRepository{},
	})
	if err != nil {
		t.Fatalf("create recommendation service: %v", err)
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations/policies?enabled_only=true", nil)
	handleListPolicies(service).ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), policyRepository.policies[0].UID.String()) {
		t.Fatalf("list policy response = %d %s", listRecorder.Code, listRecorder.Body.String())
	}

	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/policies", strings.NewReader(`{"name":"safe-v2","version":2,"enabled":true,"constraints":{"allow_scale_down":false}}`))
	createRequest = createRequest.WithContext(iam.WithIdentityContext(createRequest.Context(), iam.IdentityContext{UserID: actorID}))
	handleCreatePolicy(service).ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create policy status = %d, body = %s", createRecorder.Code, createRecorder.Body.String())
	}
	if policyRepository.created.Name != "safe-v2" || policyRepository.created.Version != 2 || policyRepository.created.CreatedBy != actorID.String() {
		t.Fatalf("created policy = %#v", policyRepository.created)
	}
	invalidRecorder := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/policies", strings.NewReader(`{"name":"unsafe-v3","version":3,"enabled":true,"constraints":{"min_cpu_p95_coverage_pct":0}}`))
	invalidRequest = invalidRequest.WithContext(iam.WithIdentityContext(invalidRequest.Context(), iam.IdentityContext{UserID: actorID}))
	handleCreatePolicy(service).ServeHTTP(invalidRecorder, invalidRequest)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid coverage threshold status = %d, body = %s", invalidRecorder.Code, invalidRecorder.Body.String())
	}
	if policyRepository.created.Name != "safe-v2" {
		t.Fatalf("invalid policy should not be persisted: got %#v", policyRepository.created)
	}
}

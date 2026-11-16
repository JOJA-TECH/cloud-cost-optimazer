package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/google/uuid"
)

func TestGenerateOptimizationHandlerReturnsReportAndArtifact(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"resource_uid": resourceUID.String(),
		"from":         now.Format(time.RFC3339),
		"to":           now.Add(time.Hour).Format(time.RFC3339),
		"format":       "json",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(body))
	identity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	identity.ProjectID = &projectID
	request = request.WithContext(iam.WithIdentityContext(context.Background(), identity))
	response := httptest.NewRecorder()

	GenerateOptimizationHandler(service).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusCreated, response.Body)
	}
	var result ReportResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Report.ProjectID != projectID || result.Report.GeneratedBy != userID {
		t.Fatalf("report identity/project = %s/%s, want %s/%s", result.Report.GeneratedBy, result.Report.ProjectID, userID, projectID)
	}
	if result.Artifact.UID == uuid.Nil || result.Report.Artifact != nil {
		t.Fatalf("response must include artifact metadata separately: %#v", result)
	}
	if hasReportSection(result.Report, ReportSectionAuditTrace) {
		t.Fatal("audit trace must be omitted unless explicitly requested")
	}
}

func TestGenerateOptimizationHandlerRequiresAuditPermission(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	projectID := uuid.New()
	body, _ := json.Marshal(map[string]any{
		"resource_uid":  uuid.NewString(),
		"from":          now.Format(time.RFC3339),
		"to":            now.Add(time.Hour).Format(time.RFC3339),
		"format":        "json",
		"include_audit": true,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(body))
	identity := iam.NewIdentityContext(uuid.New(), "", "", "test")
	identity.ProjectID = &projectID
	request = request.WithContext(iam.WithIdentityContext(context.Background(), identity))
	response := httptest.NewRecorder()

	generateOptimizationHandler(service, nil).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusForbidden, response.Body)
	}
}

func TestGenerateOptimizationHandlerIncludesAuditOnlyWithPermission(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	projectID, organizationID := uuid.New(), uuid.New()
	resourceUID, recommendationUID := uuid.New(), uuid.New()
	provider := &reportDataProviderStub{
		recommendations: []RecommendationReportData{{
			ReportRecommendation: ReportRecommendation{RecommendationUID: recommendationUID},
			ResourceUID:          resourceUID,
		}},
		trace: RecommendationTraceData{
			RecommendationUID: recommendationUID,
			DecisionTraces:    []RecommendationDecisionTrace{{TraceUID: uuid.New()}},
		},
	}
	service, err := NewService(Dependencies{
		DataProvider:  provider,
		Repository:    &reportRepositoryStub{},
		ResourceScope: reportResourceScopeStub{allowed: true},
		Formatters:    []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:      reportExporterStub{calls: &[]string{}},
		Audit:         &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	body, _ := json.Marshal(map[string]any{
		"resource_uid":  resourceUID.String(),
		"from":          now.Format(time.RFC3339),
		"to":            now.Add(time.Hour).Format(time.RFC3339),
		"format":        "json",
		"include_audit": true,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(body))
	identity := iam.NewIdentityContext(uuid.New(), "", "", "test")
	identity.ProjectID = &projectID
	identity.OrgID = organizationID
	request = request.WithContext(iam.WithIdentityContext(context.Background(), identity))
	response := httptest.NewRecorder()

	generateOptimizationHandler(service, reportPermissionStub{
		"audit:read:organization": true,
	}).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
	var result ReportResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !hasReportSection(result.Report, ReportSectionAuditTrace) {
		t.Fatal("audit trace section was not included for an authorized request")
	}
	if len(provider.calls) == 0 || provider.calls[len(provider.calls)-1] != "trace" {
		t.Fatalf("provider calls = %v, want trace read", provider.calls)
	}
}

type reportPermissionStub map[string]bool

func (s reportPermissionStub) Can(_ context.Context, _ iam.IdentityContext, resource, action string) (bool, error) {
	return s[resource+":"+action], nil
}

func hasReportSection(report Report, key string) bool {
	for _, section := range report.Sections {
		if section.Key == key {
			return true
		}
	}
	return false
}

func TestGenerateOptimizationHandlerRejectsInvalidRequestAndMissingProject(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	projectID := uuid.New()
	tests := []struct {
		name       string
		body       string
		projectID  *uuid.UUID
		wantStatus int
	}{
		{name: "invalid JSON", body: "{", projectID: &projectID, wantStatus: http.StatusBadRequest},
		{name: "missing project", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "arbitrary organization is rejected", body: `{"organization_id":"` + uuid.NewString() + `"}`, projectID: &projectID, wantStatus: http.StatusBadRequest},
		{name: "arbitrary project is rejected", body: `{"project_id":"` + uuid.NewString() + `"}`, projectID: &projectID, wantStatus: http.StatusBadRequest},
		{name: "invalid recommendation UUID is rejected", body: `{"recommendation_uids":["not-a-uuid"]}`, projectID: &projectID, wantStatus: http.StatusBadRequest},
		{name: "legacy trace flag cannot opt in without include_audit", body: `{"include_trace":true}`, projectID: &projectID, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewBufferString(test.body))
			identity := iam.NewIdentityContext(uuid.New(), "", "", "test")
			identity.ProjectID = test.projectID
			request = request.WithContext(iam.WithIdentityContext(context.Background(), identity))
			response := httptest.NewRecorder()

			GenerateOptimizationHandler(service).ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body)
			}
		})
	}
}

func newHandlerTestService() (*Service, error) {
	return NewService(Dependencies{
		DataProvider: &reportDataProviderStub{},
		Repository:   &reportRepositoryStub{},
		ResourceScope: reportResourceScopeStub{
			allowed: true,
		},
		Formatters: []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:   reportExporterStub{calls: &[]string{}},
		Audit:      &reportAuditSinkStub{calls: &[]string{}},
	})
}

// reportTestMux registra las rutas de reporte en un mux de prueba para que
// r.PathValue("{uid}") se poblará al invocar los handlers.
func reportTestMux(svc *Service, permissions iam.PermissionChecker) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/reports/optimization", http.HandlerFunc(GenerateOptimizationHandler(svc)))
	mux.Handle("GET /api/v1/reports/{uid}", middleware.RequirePermission(permissions, "reports", "read")(
		http.HandlerFunc(getReportHandler(svc, permissions))))
	mux.Handle("GET /api/v1/reports/{uid}/artifact", middleware.RequirePermission(permissions, "reports", "export")(
		http.HandlerFunc(getReportArtifactHandler(svc, permissions))))
	return mux
}

func TestGetReportHandlerReturnsReportMetadata(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()

	// Generar un reporte para que quede en el caché en memoria del service.
	genBody, _ := json.Marshal(map[string]any{
		"resource_uid": resourceUID.String(),
		"from":         now.Format(time.RFC3339),
		"to":           now.Add(time.Hour).Format(time.RFC3339),
		"format":       "json",
	})
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(genBody))
	genIdentity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	genIdentity.ProjectID = &projectID
	genReq = genReq.WithContext(iam.WithIdentityContext(context.Background(), genIdentity))
	genResp := httptest.NewRecorder()
	GenerateOptimizationHandler(service).ServeHTTP(genResp, genReq)
	if genResp.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, want %d: %s", genResp.Code, http.StatusCreated, genResp.Body)
	}
	var genResult ReportResponse
	if err := json.Unmarshal(genResp.Body.Bytes(), &genResult); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	reportUID := genResult.Report.UID

	// Consultar metadata del reporte con el mismo proyecto.
	mux := reportTestMux(service, reportPermissionStub{"reports:read": true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/"+reportUID.String(), nil)
	identity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	identity.ProjectID = &projectID
	req = req.WithContext(iam.WithIdentityContext(context.Background(), identity))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body)
	}
	var report Report
	if err := json.Unmarshal(resp.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if report.UID != reportUID || report.ProjectID != projectID || report.GeneratedBy != userID {
		t.Fatalf("report identity mismatch: %#v", report)
	}
	if report.Status != ReportStatusCompleted {
		t.Fatalf("report status = %s, want %s", report.Status, ReportStatusCompleted)
	}
}

func TestGetReportHandlerForbidsDifferentProject(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	projectID := uuid.New()
	otherProjectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()

	genBody, _ := json.Marshal(map[string]any{
		"resource_uid": resourceUID.String(),
		"from":         now.Format(time.RFC3339),
		"to":           now.Add(time.Hour).Format(time.RFC3339),
		"format":       "json",
	})
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(genBody))
	genIdentity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	genIdentity.ProjectID = &projectID
	genReq = genReq.WithContext(iam.WithIdentityContext(context.Background(), genIdentity))
	genResp := httptest.NewRecorder()
	GenerateOptimizationHandler(service).ServeHTTP(genResp, genReq)
	if genResp.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, want %d: %s", genResp.Code, http.StatusCreated, genResp.Body)
	}
	var genResult ReportResponse
	if err := json.Unmarshal(genResp.Body.Bytes(), &genResult); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	reportUID := genResult.Report.UID

	// Consultar con un proyecto distinto.
	mux := reportTestMux(service, reportPermissionStub{"reports:read": true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/"+reportUID.String(), nil)
	identity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	identity.ProjectID = &otherProjectID
	req = req.WithContext(iam.WithIdentityContext(context.Background(), identity))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusForbidden, resp.Body)
	}
}

func TestGetReportHandlerAllowsPlatformAdmin(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()

	genBody, _ := json.Marshal(map[string]any{
		"resource_uid": resourceUID.String(),
		"from":         now.Format(time.RFC3339),
		"to":           now.Add(time.Hour).Format(time.RFC3339),
		"format":       "json",
	})
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(genBody))
	genIdentity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	genIdentity.ProjectID = &projectID
	genReq = genReq.WithContext(iam.WithIdentityContext(context.Background(), genIdentity))
	genResp := httptest.NewRecorder()
	GenerateOptimizationHandler(service).ServeHTTP(genResp, genReq)
	if genResp.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, want %d: %s", genResp.Code, http.StatusCreated, genResp.Body)
	}
	var genResult ReportResponse
	if err := json.Unmarshal(genResp.Body.Bytes(), &genResult); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	reportUID := genResult.Report.UID

	// platform_admin sin proyecto activo puede consultar cualquier reporte.
	mux := reportTestMux(service, reportPermissionStub{"reports:read": true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/"+reportUID.String(), nil)
	identity := iam.NewIdentityContext(userID, "user@example.com", "Admin User", "test")
	identity.SystemRoles = []string{"platform_admin"}
	req = req.WithContext(iam.WithIdentityContext(context.Background(), identity))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body)
	}
}

func TestGetReportArtifactHandlerReturnsContent(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()

	genBody, _ := json.Marshal(map[string]any{
		"resource_uid": resourceUID.String(),
		"from":         now.Format(time.RFC3339),
		"to":           now.Add(time.Hour).Format(time.RFC3339),
		"format":       "json",
	})
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(genBody))
	genIdentity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	genIdentity.ProjectID = &projectID
	genReq = genReq.WithContext(iam.WithIdentityContext(context.Background(), genIdentity))
	genResp := httptest.NewRecorder()
	GenerateOptimizationHandler(service).ServeHTTP(genResp, genReq)
	if genResp.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, want %d: %s", genResp.Code, http.StatusCreated, genResp.Body)
	}
	var genResult ReportResponse
	if err := json.Unmarshal(genResp.Body.Bytes(), &genResult); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	reportUID := genResult.Report.UID

	mux := reportTestMux(service, reportPermissionStub{"reports:export": true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/"+reportUID.String()+"/artifact", nil)
	identity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	identity.ProjectID = &projectID
	req = req.WithContext(iam.WithIdentityContext(context.Background(), identity))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusOK, resp.Body)
	}
	if resp.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content-type = %q, want application/json", resp.Header().Get("Content-Type"))
	}
	if string(resp.Body.Bytes()) != `{"report":true}` {
		t.Fatalf("artifact body = %q, want %q", resp.Body.Bytes(), `{"report":true}`)
	}
}

func TestGetReportArtifactHandlerForbidsWithoutExportPermission(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()

	genBody, _ := json.Marshal(map[string]any{
		"resource_uid": resourceUID.String(),
		"from":         now.Format(time.RFC3339),
		"to":           now.Add(time.Hour).Format(time.RFC3339),
		"format":       "json",
	})
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/optimization", bytes.NewReader(genBody))
	genIdentity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	genIdentity.ProjectID = &projectID
	genReq = genReq.WithContext(iam.WithIdentityContext(context.Background(), genIdentity))
	genResp := httptest.NewRecorder()
	GenerateOptimizationHandler(service).ServeHTTP(genResp, genReq)
	if genResp.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, want %d: %s", genResp.Code, http.StatusCreated, genResp.Body)
	}
	var genResult ReportResponse
	if err := json.Unmarshal(genResp.Body.Bytes(), &genResult); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	reportUID := genResult.Report.UID

	mux := reportTestMux(service, reportPermissionStub{"reports:read": true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/"+reportUID.String()+"/artifact", nil)
	identity := iam.NewIdentityContext(userID, "user@example.com", "Report User", "test")
	identity.ProjectID = &projectID
	req = req.WithContext(iam.WithIdentityContext(context.Background(), identity))
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusForbidden, resp.Body)
	}
}

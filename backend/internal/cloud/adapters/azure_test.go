package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/google/uuid"
)

const (
	testSub       = "sub-123"
	testTenant    = "tenant-1"
	testClient    = "client-1"
	testSecret    = "super-secret-must-not-leak"
	serverName    = "sql-prod-01"
	resourceGroup = "rg-prod"
)

func testCreds() cloud.ConnectionCredentials {
	return cloud.ConnectionCredentials{
		TenantID:       testTenant,
		ClientID:       testClient,
		ClientSecret:   testSecret,
		SubscriptionID: testSub,
	}
}

func fakeTokenAdapter(baseURL string) *AzureAdapter {
	a := newAzureAdapterWithBase(baseURL)
	a.tokenFunc = func(ctx context.Context, creds cloud.ConnectionCredentials) (string, error) {
		return "fake-token", nil
	}
	return a
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestValidateConnectionSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"status": "active"})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	result, err := a.ValidateConnection(context.Background(), testCreds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "active" {
		t.Fatalf("expected active status, got %q", result.Status)
	}
}

func TestValidateConnection401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]interface{}{"error": map[string]interface{}{
			"code":    "AuthenticationFailed",
			"message": "invalid token",
		}})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	result, err := a.ValidateConnection(context.Background(), testCreds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("expected error status, got %q", result.Status)
	}
	if strings.Contains(result.Detail, testSecret) {
		t.Fatalf("detail must not contain the client secret: %q", result.Detail)
	}
}

func TestValidateConnection403RBAC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusForbidden, map[string]interface{}{"error": map[string]interface{}{
			"code":    "AuthorizationFailed",
			"message": "The client does not have authorization to perform this action",
		}})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	result, err := a.ValidateConnection(context.Background(), testCreds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("expected error status, got %q", result.Status)
	}
	if !strings.Contains(result.Detail, "AuthorizationFailed") {
		t.Fatalf("expected ARM error code in detail, got %q", result.Detail)
	}
}

func TestListSQLDatabasesSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		q := r.URL.Query()

		// Servers page 1 -> nextLink to page 2.
		if strings.Contains(path, "/servers") && !strings.Contains(path, "/databases") {
			if q.Get("page") == "2" {
				writeJSON(t, w, http.StatusOK, map[string]interface{}{"value": []interface{}{}})
				return
			}
			nextLink := "http://" + r.Host + r.URL.Path + "?api-version=2025-01-01&page=2"
			writeJSON(t, w, http.StatusOK, map[string]interface{}{
				"value": []interface{}{
					map[string]interface{}{
						"id":       "/subscriptions/" + testSub + "/resourceGroups/" + resourceGroup + "/providers/Microsoft.Sql/servers/" + serverName,
						"name":     serverName,
						"kind":     "v12.0",
						"location": "eastus",
					},
				},
				"nextLink": nextLink,
			})
			return
		}

		// Databases: 2 user DBs + 1 system DB (must be filtered).
		if strings.Contains(path, "/databases") {
			writeJSON(t, w, http.StatusOK, map[string]interface{}{
				"value": []interface{}{
					map[string]interface{}{
						"id":       "/subscriptions/" + testSub + "/resourceGroups/" + resourceGroup + "/providers/Microsoft.Sql/servers/" + serverName + "/databases/db-sales",
						"name":     "db-sales",
						"kind":     "v12.0,user,vcore",
						"location": "eastus",
					},
					map[string]interface{}{
						"id":       "/subscriptions/" + testSub + "/resourceGroups/" + resourceGroup + "/providers/Microsoft.Sql/servers/" + serverName + "/databases/db-billing",
						"name":     "db-billing",
						"kind":     "v12.0,user,vcore,provisioned",
						"location": "eastus",
					},
					map[string]interface{}{
						"id":       "/subscriptions/" + testSub + "/resourceGroups/" + resourceGroup + "/providers/Microsoft.Sql/servers/" + serverName + "/databases/master",
						"name":     "master",
						"kind":     "v12.0,system",
						"location": "eastus",
					},
				},
			})
			return
		}

		writeJSON(t, w, http.StatusNotFound, map[string]interface{}{})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	resources, err := a.ListSQLDatabases(context.Background(), testCreds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 user databases (system filtered), got %d", len(resources))
	}

	expected := []struct {
		database      string
		azureResource string
		region        string
		kind          string
	}{
		{database: "db-sales", azureResource: "/subscriptions/" + testSub + "/resourceGroups/" + resourceGroup + "/providers/Microsoft.Sql/servers/" + serverName + "/databases/db-sales", region: "eastus", kind: "v12.0,user,vcore"},
		{database: "db-billing", azureResource: "/subscriptions/" + testSub + "/resourceGroups/" + resourceGroup + "/providers/Microsoft.Sql/servers/" + serverName + "/databases/db-billing", region: "eastus", kind: "v12.0,user,vcore,provisioned"},
	}
	got := map[string]cloud.ResourceDTO{}
	for _, r := range resources {
		got[r.DatabaseName] = r
	}
	for _, e := range expected {
		r, ok := got[e.database]
		if !ok {
			t.Fatalf("missing database %q", e.database)
		}
		if r.SubscriptionID != testSub {
			t.Errorf("db %q subscription = %q, want %q", e.database, r.SubscriptionID, testSub)
		}
		if r.ResourceGroup != resourceGroup {
			t.Errorf("db %q resourceGroup = %q, want %q (ARM id path parsing)", e.database, r.ResourceGroup, resourceGroup)
		}
		if r.ServerName != serverName {
			t.Errorf("db %q serverName = %q, want %q", e.database, r.ServerName, serverName)
		}
		if r.AzureResourceID != e.azureResource {
			t.Errorf("db %q azureResourceID = %q, want %q", e.database, r.AzureResourceID, e.azureResource)
		}
		if r.RegionCode != e.region {
			t.Errorf("db %q regionCode = %q, want %q", e.database, r.RegionCode, e.region)
		}
		if r.Kind == nil || *r.Kind != e.kind {
			t.Errorf("db %q kind = %v, want %q", e.database, r.Kind, e.kind)
		}
		// Service-owned fields must be zero (adapter leaves them zero).
		if r.ResourceUID != uuid.Nil {
			t.Errorf("db %q ResourceUID should be zero, got %v", e.database, r.ResourceUID)
		}
		if r.CloudAccountID != uuid.Nil {
			t.Errorf("db %q CloudAccountID should be zero, got %v", e.database, r.CloudAccountID)
		}
		if r.ProviderCode != "" {
			t.Errorf("db %q ProviderCode should be zero, got %q", e.database, r.ProviderCode)
		}
		if r.LifecycleStatus != "" {
			t.Errorf("db %q LifecycleStatus should be zero, got %q", e.database, r.LifecycleStatus)
		}
		if !r.LastSeenAt.IsZero() {
			t.Errorf("db %q LastSeenAt should be zero, got %v", e.database, r.LastSeenAt)
		}
	}
}

func TestListSQLDatabasesEmptyServers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"value": []interface{}{}})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	resources, err := a.ListSQLDatabases(context.Background(), testCreds())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resources == nil {
		t.Fatal("expected empty non-nil slice for empty servers list")
	}
	if len(resources) != 0 {
		t.Fatalf("expected 0 resources, got %d", len(resources))
	}
}

func TestListSQLDatabasesAuth401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]interface{}{"error": map[string]interface{}{
			"code":    "AuthenticationFailed",
			"message": "invalid token",
		}})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	_, err := a.ListSQLDatabases(context.Background(), testCreds())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !errors.Is(err, cloud.ErrConnectionFailed) {
		t.Fatalf("expected ErrConnectionFailed for 401, got %v", err)
	}
	if strings.Contains(err.Error(), testSecret) {
		t.Fatalf("error must not contain the client secret: %v", err)
	}
}

func TestListSQLDatabasesRBAC403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusForbidden, map[string]interface{}{"error": map[string]interface{}{
			"code":    "AuthorizationFailed",
			"message": "The client does not have authorization",
		}})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	_, err := a.ListSQLDatabases(context.Background(), testCreds())
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !errors.Is(err, cloud.ErrConnectionFailed) {
		t.Fatalf("expected ErrConnectionFailed for 403, got %v", err)
	}
	if !strings.Contains(err.Error(), "AuthorizationFailed") {
		t.Fatalf("expected ARM code in error, got %v", err)
	}
}

func TestListSQLDatabasesServer5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusInternalServerError, map[string]interface{}{"error": map[string]interface{}{
			"code":    "InternalServerError",
			"message": "ARM transient failure",
		}})
	}))
	defer srv.Close()

	a := fakeTokenAdapter(srv.URL)
	_, err := a.ListSQLDatabases(context.Background(), testCreds())
	if err == nil {
		t.Fatal("expected error for 5xx")
	}
	if !errors.Is(err, cloud.ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable for 5xx, got %v", err)
	}
}

func TestListSQLDatabasesNetworkError(t *testing.T) {
	// Unreachable address -> connection refused (network error).
	a := fakeTokenAdapter("http://127.0.0.1:1")
	a.httpClient.Timeout = 0 // no timeout so connect fails fast
	_, err := a.ListSQLDatabases(context.Background(), testCreds())
	if err == nil {
		t.Fatal("expected error for network failure")
	}
	if !errors.Is(err, cloud.ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable for network error, got %v", err)
	}
}

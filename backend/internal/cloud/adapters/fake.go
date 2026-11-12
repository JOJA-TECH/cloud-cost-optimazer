package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/google/uuid"
)

// FakeAdapter implements cloud.ConnectionValidator and cloud.ResourceProvider
// with deterministic, hard-coded data. Intended for dev/tests; replace with a
// real Azure adapter when credentials are available.
type FakeAdapter struct{}

// NewFakeAdapter returns a FakeAdapter.
func NewFakeAdapter() FakeAdapter {
	return FakeAdapter{}
}

// ValidateConnection validates credentials against the fake provider.
// Status is "active" unless TenantID is empty or ClientSecret == "invalid".
func (f FakeAdapter) ValidateConnection(ctx context.Context, creds cloud.ConnectionCredentials) (cloud.ConnectionResult, error) {
	if creds.TenantID == "" || creds.ClientSecret == "invalid" {
		detail := "invalid credentials"
		if creds.TenantID == "" {
			detail = "missing tenant id"
		}
		return cloud.ConnectionResult{Status: "error", Detail: detail}, nil
	}
	return cloud.ConnectionResult{Status: "active", Detail: "ok"}, nil
}

// ListSQLDatabases returns deterministic Azure SQL database resources.
func (f FakeAdapter) ListSQLDatabases(ctx context.Context, creds cloud.ConnectionCredentials) ([]cloud.ResourceDTO, error) {
	servers := []struct {
		server    string
		databases []string
	}{
		{server: "sql-prod-01", databases: []string{"sqldb-sales", "sqldb-billing"}},
		{server: "sql-analytics", databases: []string{"sqldb-metrics"}},
	}
	kind := "v12.0,user,vcore,provisioned"
	var resources []cloud.ResourceDTO
	for _, s := range servers {
		for _, db := range s.databases {
			uid := uuid.New()
			arm := fmt.Sprintf("/subscriptions/%s/resourceGroups/rg-demo/providers/Microsoft.Sql/servers/%s/databases/%s",
				creds.SubscriptionID, s.server, db)
			resources = append(resources, cloud.ResourceDTO{
				ResourceUID:     uid,
				ProviderCode:    cloud.ProviderAzure,
				SubscriptionID:  creds.SubscriptionID,
				ResourceGroup:   "rg-demo",
				ServerName:      s.server,
				DatabaseName:    strings.ToLower(db),
				AzureResourceID: arm,
				RegionCode:      "eastus",
				Kind:            &kind,
				LifecycleStatus: "active",
			})
		}
	}
	return resources, nil
}

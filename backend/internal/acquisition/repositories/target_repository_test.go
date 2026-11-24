package repositories

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func openAppDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, t.Name())
	db, err := database.Open(database.Config{
		Driver: database.DriverSQLite,
		DSN:    "file:" + name + "?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate cco_app: %v", err)
	}
	return db
}

func mustExec(db *gorm.DB, sql string, args ...interface{}) {
	if err := db.Exec(sql, args...).Error; err != nil {
		panic(err)
	}
}

func seedTargetFixtures(t *testing.T, db *gorm.DB) (orgA, orgB, acctA, acctB uuid.UUID) {
	orgA = uuid.New()
	orgB = uuid.New()
	acctA = uuid.New()
	acctB = uuid.New()

	now := time.Now().UTC()
	mustExec(db, `INSERT INTO organizations (organization_id, name, slug, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
		orgA, "Org A", "org-a", now, now)
	mustExec(db, `INSERT INTO organizations (organization_id, name, slug, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)`,
		orgB, "Org B", "org-b", now, now)
	mustExec(db, `INSERT INTO accounts (cloud_account_id, organization_id, provider_code, display_name, subscription_id, tenant_id, credential_reference, status, client_id, created_at, updated_at) VALUES (?, ?, 'azure', ?, 'sub-a', 'tenant-a', 'env:CCO_TEST_SECRET', 'active', 'client-a', ?, ?)`,
		acctA, orgA, "Contoso A", now, now)
	mustExec(db, `INSERT INTO accounts (cloud_account_id, organization_id, provider_code, display_name, subscription_id, tenant_id, credential_reference, status, client_id, created_at, updated_at) VALUES (?, ?, 'azure', ?, 'sub-b', 'tenant-b', NULL, 'active', 'client-b', ?, ?)`,
		acctB, orgB, "Contoso B", now, now)
	resA := uuid.New()
	resB := uuid.New()
	mustExec(db, `INSERT INTO resources (resource_uid, cloud_account_id, provider_code, subscription_id, resource_group, server_name, database_name, azure_resource_id, region_code, kind, lifecycle_status, registered_at, last_seen_at, created_at, updated_at) VALUES (?, ?, 'azure', 'sub-a', 'rg-a', 'srv-a', 'db-a', '/sub/sub-a/rg/a', 'eastus2', 'v12.0,user', 'active', ?, ?, ?, ?)`,
		resA, acctA, now, now, now, now)
	mustExec(db, `INSERT INTO resources (resource_uid, cloud_account_id, provider_code, subscription_id, resource_group, server_name, database_name, azure_resource_id, region_code, kind, lifecycle_status, registered_at, last_seen_at, created_at, updated_at) VALUES (?, ?, 'azure', 'sub-b', 'rg-b', 'srv-b', 'db-b', '/sub/sub-b/rg/b', 'westus2', NULL, 'active', ?, ?, ?, ?)`,
		resB, acctB, now, now, now, now)
	mustExec(db, `INSERT INTO resource_bindings (resource_binding_id, project_id, cloud_account_id, data_resource_uid, is_monitored, created_at, updated_at) VALUES (?, ?, ?, ?, true, ?, ?)`,
		uuid.New(), uuid.New(), acctA, resA, now, now)
	mustExec(db, `INSERT INTO resource_bindings (resource_binding_id, project_id, cloud_account_id, data_resource_uid, is_monitored, created_at, updated_at) VALUES (?, ?, ?, ?, false, ?, ?)`,
		uuid.New(), uuid.New(), acctB, resB, now, now)
	return orgA, orgB, acctA, acctB
}

func TestTargetRepositoryListMonitoredTargets(t *testing.T) {
	db := openAppDB(t)
	orgA, orgB, _, _ := seedTargetFixtures(t, db)
	ctx := context.Background()
	repo := NewTargetRepository(db)

	all, err := repo.ListMonitoredTargets(ctx, nil)
	if err != nil {
		t.Fatalf("ListMonitoredTargets(nil): %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListMonitoredTargets(nil) = %d targets, want 1 (non-monitored excluded)", len(all))
	}
	if all[0].AzureResourceID != "/sub/sub-a/rg/a" {
		t.Errorf("unexpected target: %+v", all[0])
	}

	gotA, err := repo.ListMonitoredTargets(ctx, &orgA)
	if err != nil {
		t.Fatalf("ListMonitoredTargets(orgA): %v", err)
	}
	if len(gotA) != 1 {
		t.Fatalf("ListMonitoredTargets(orgA) = %d, want 1", len(gotA))
	}

	gotB, err := repo.ListMonitoredTargets(ctx, &orgB)
	if err != nil {
		t.Fatalf("ListMonitoredTargets(orgB): %v", err)
	}
	if len(gotB) != 0 {
		t.Fatalf("ListMonitoredTargets(orgB) = %d, want 0 (org B binding not monitored)", len(gotB))
	}
}

func TestTargetRepositoryResolveCredentials(t *testing.T) {
	db := openAppDB(t)
	_, _, acctA, acctB := seedTargetFixtures(t, db)
	ctx := context.Background()
	repo := NewTargetRepository(db)

	t.Setenv("CCO_TEST_SECRET", "s3cret-value")
	creds, err := repo.ResolveCredentials(ctx, acctA)
	if err != nil {
		t.Fatalf("ResolveCredentials(env:VAR): %v", err)
	}
	if creds.ClientSecret != "s3cret-value" {
		t.Errorf("ClientSecret = %q, want s3cret-value", creds.ClientSecret)
	}
	if creds.TenantID != "tenant-a" || creds.ClientID != "client-a" || creds.SubscriptionID != "sub-a" {
		t.Errorf("unexpected creds: %+v", creds)
	}

	if _, err := repo.ResolveCredentials(ctx, acctB); !errors.Is(err, cloud.ErrCredentialsUnavailable) {
		t.Errorf("ResolveCredentials(empty ref) = %v, want ErrCredentialsUnavailable", err)
	}
}

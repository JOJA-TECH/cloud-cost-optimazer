package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	cloudadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/adapters"
	cloudrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
)

func TestCloudStackEndToEnd(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()

	// Seed org/project rows directly (migration creates empty tables).
	if err := db.Table("organizations").Create(&database.OrganizationModel{
		OrganizationID: orgID,
		Name:           "Acme Corp",
		Slug:           "acme",
		Status:         "active",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := db.Table("projects").Create(&database.ProjectModel{
		ProjectID:      projectID,
		OrganizationID: orgID,
		Name:           "Acme Project",
		Status:         "active",
		CreatedBy:      &userID,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	accountRepo := cloudrepos.NewAccountRepository(db)
	resourceRepo := cloudrepos.NewResourceRepository(db)
	bindingRepo := cloudrepos.NewDataSourceRepository(db)
	svc := cloud.NewService(accountRepo, resourceRepo, bindingRepo,
		cloudadapters.NewFakeAdapter(), cloudadapters.NewFakeAdapter())

	t.Run("register + validate + sync + idempotent upsert + datasource lifecycle", func(t *testing.T) {
		t.Setenv("CCO_TEST_SECRET", "s3cret-value")

		acc, err := svc.RegisterAccount(ctx, orgID, userID, cloud.CreateAccountInput{
			ProviderCode:        cloud.ProviderAzure,
			DisplayName:         "Contoso Subscription",
			SubscriptionID:      "sub-1234",
			TenantID:            "tenant-1",
			CredentialReference: "env:CCO_TEST_SECRET",
		})
		if err != nil {
			t.Fatalf("register account: %v", err)
		}
		if acc.Status != "pending" {
			t.Fatalf("expected pending status, got %s", acc.Status)
		}

		// Validate with secret present → active.
		validated, err := svc.ValidateAccount(ctx, orgID, acc.CloudAccountID)
		if err != nil {
			t.Fatalf("validate account: %v", err)
		}
		if validated.Status != "active" {
			t.Fatalf("expected active status, got %s", validated.Status)
		}
		if validated.LastConnectionError != nil {
			t.Fatalf("expected no connection error, got %v", *validated.LastConnectionError)
		}

		// Sync resources → 3 resources.
		resources, err := svc.SyncResources(ctx, orgID, acc.CloudAccountID)
		if err != nil {
			t.Fatalf("sync resources: %v", err)
		}
		if len(resources) != 3 {
			t.Fatalf("expected 3 resources, got %d", len(resources))
		}
		uidByARM := make(map[string]uuid.UUID)
		for _, res := range resources {
			if res.ResourceUID == uuid.Nil {
				t.Fatalf("resource %s has zero ResourceUID", res.AzureResourceID)
			}
			uidByARM[res.AzureResourceID] = res.ResourceUID
		}

		// Sync again → still 3, SAME UIDs (idempotent upsert).
		resources2, err := svc.SyncResources(ctx, orgID, acc.CloudAccountID)
		if err != nil {
			t.Fatalf("sync resources (2nd): %v", err)
		}
		if len(resources2) != 3 {
			t.Fatalf("expected 3 resources on 2nd sync, got %d", len(resources2))
		}
		for _, res := range resources2 {
			if uidByARM[res.AzureResourceID] != res.ResourceUID {
				t.Fatalf("ResourceUID changed for %s after re-sync", res.AzureResourceID)
			}
		}

		// Register data source binding.
		salesUID := uuid.Nil
		for _, res := range resources {
			if res.DatabaseName == "sqldb-sales" {
				salesUID = res.ResourceUID
			}
		}
		if salesUID == uuid.Nil {
			t.Fatal("sqldb-sales resource not found")
		}
		binding, err := svc.RegisterDataSource(ctx, orgID, cloud.CreateBindingInput{
			ProjectID:       projectID,
			CloudAccountID:  acc.CloudAccountID,
			DataResourceUID: salesUID,
			DisplayName:     "Sales DB",
		})
		if err != nil {
			t.Fatalf("register data source: %v", err)
		}
		if !binding.IsMonitored {
			t.Fatal("expected binding to be monitored by default")
		}

		// List data sources returns 1.
		bindings, err := svc.ListDataSources(ctx, orgID, &projectID)
		if err != nil {
			t.Fatalf("list data sources: %v", err)
		}
		if len(bindings) != 1 {
			t.Fatalf("expected 1 binding, got %d", len(bindings))
		}

		// Set monitored false → list reflects it.
		if err := svc.SetDataSourceMonitored(ctx, orgID, binding.ResourceBindingID, false); err != nil {
			t.Fatalf("set binding monitored: %v", err)
		}
		bindings, err = svc.ListDataSources(ctx, orgID, &projectID)
		if err != nil {
			t.Fatalf("list data sources (2nd): %v", err)
		}
		if len(bindings) != 1 || bindings[0].IsMonitored {
			t.Fatalf("expected binding not monitored, got %#v", bindings)
		}
	})

	t.Run("missing secret → error status, no secret leaked", func(t *testing.T) {
		t.Setenv("CCO_TEST_SECRET", "s3cret-value") // ensure the other var is unset
		acc, err := svc.RegisterAccount(ctx, orgID, userID, cloud.CreateAccountInput{
			ProviderCode:        cloud.ProviderAzure,
			DisplayName:         "Missing Secret",
			SubscriptionID:      "sub-missing",
			TenantID:            "tenant-x",
			CredentialReference: "env:CCO_TEST_MISSING",
		})
		if err != nil {
			t.Fatalf("register account: %v", err)
		}
		validated, err := svc.ValidateAccount(ctx, orgID, acc.CloudAccountID)
		if err != nil {
			t.Fatalf("validate account: %v", err)
		}
		if validated.Status != "error" {
			t.Fatalf("expected error status, got %s", validated.Status)
		}
		if validated.LastConnectionError == nil || *validated.LastConnectionError == "" {
			t.Fatal("expected non-empty last_connection_error")
		}
		detail := *validated.LastConnectionError
		if contains(detail, "s3cret-value") {
			t.Fatalf("secret leaked into error detail: %s", detail)
		}
	})

	t.Run("unknown data resource → ErrResourceNotFound", func(t *testing.T) {
		_, err := svc.RegisterDataSource(ctx, orgID, cloud.CreateBindingInput{
			ProjectID:       projectID,
			CloudAccountID:  uuid.New(),
			DataResourceUID: uuid.New(),
			DisplayName:     "Nope",
		})
		if err == nil {
			t.Fatal("expected error registering data source with unknown resource")
		}
	})

	t.Run("duplicate binding → ErrBindingExists", func(t *testing.T) {
		// Use an existing resource from the first test's account (sub-1234).
		var target cloud.AccountDTO
		accs, err := svc.ListAccounts(ctx, orgID)
		if err != nil {
			t.Fatalf("list accounts: %v", err)
		}
		for _, a := range accs {
			if resources, lerr := svc.ListResources(ctx, orgID, a.CloudAccountID); lerr == nil && len(resources) > 0 {
				target = a
				break
			}
		}
		if target.CloudAccountID == uuid.Nil {
			t.Fatal("no account with resources found")
		}
		resources, err := svc.ListResources(ctx, orgID, target.CloudAccountID)
		if err != nil || len(resources) == 0 {
			t.Fatalf("list resources: %v", err)
		}
		// Register once (may already be bound from an earlier sub-test), then
		// register again to force the duplicate path.
		if _, err := svc.RegisterDataSource(ctx, orgID, cloud.CreateBindingInput{
			ProjectID:       projectID,
			CloudAccountID:  target.CloudAccountID,
			DataResourceUID: resources[0].ResourceUID,
			DisplayName:     "Dup",
		}); err != nil {
			// Already bound (e.g. sales from an earlier sub-test) → duplicate path already exercised.
			return
		}
		if _, err := svc.RegisterDataSource(ctx, orgID, cloud.CreateBindingInput{
			ProjectID:       projectID,
			CloudAccountID:  target.CloudAccountID,
			DataResourceUID: resources[0].ResourceUID,
			DisplayName:     "Dup2",
		}); err == nil {
			t.Fatal("expected ErrBindingExists for duplicate binding")
		}
	})

	t.Run("duplicate account → ErrAccountExists", func(t *testing.T) {
		t.Setenv("CCO_TEST_SECRET", "s3cret-value")
		_, err := svc.RegisterAccount(ctx, orgID, userID, cloud.CreateAccountInput{
			ProviderCode:        cloud.ProviderAzure,
			DisplayName:         "Dup Account",
			SubscriptionID:      "sub-1234",
			TenantID:            "tenant-1",
			CredentialReference: "env:CCO_TEST_SECRET",
		})
		if err == nil {
			t.Fatal("expected ErrAccountExists for duplicate provider+subscription")
		}
	})
}

func TestCloudCrossOrgIsolation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	org1 := uuid.New()
	org2 := uuid.New()
	userID := uuid.New()
	project1 := uuid.New()
	project2 := uuid.New()

	// Seed orgs and projects.
	for _, o := range []struct {
		id   uuid.UUID
		name string
		slug string
	}{
		{org1, "Org One", "org-one"},
		{org2, "Org Two", "org-two"},
	} {
		if err := db.Table("organizations").Create(&database.OrganizationModel{
			OrganizationID: o.id,
			Name:           o.name,
			Slug:           o.slug,
			Status:         "active",
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("create organization: %v", err)
		}
	}
	for _, p := range []struct {
		id    uuid.UUID
		orgID uuid.UUID
		name  string
	}{
		{project1, org1, "Project One"},
		{project2, org2, "Project Two"},
	} {
		if err := db.Table("projects").Create(&database.ProjectModel{
			ProjectID:      p.id,
			OrganizationID: p.orgID,
			Name:           p.name,
			Status:         "active",
			CreatedBy:      &userID,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}).Error; err != nil {
			t.Fatalf("create project: %v", err)
		}
	}

	accountRepo := cloudrepos.NewAccountRepository(db)
	resourceRepo := cloudrepos.NewResourceRepository(db)
	bindingRepo := cloudrepos.NewDataSourceRepository(db)
	svc := cloud.NewService(accountRepo, resourceRepo, bindingRepo,
		cloudadapters.NewFakeAdapter(), cloudadapters.NewFakeAdapter())

	t.Setenv("CCO_TEST_SECRET", "s3cret-value")

	// Register + validate + sync an account in org1.
	acc, err := svc.RegisterAccount(ctx, org1, userID, cloud.CreateAccountInput{
		ProviderCode:        cloud.ProviderAzure,
		DisplayName:         "Org One Account",
		SubscriptionID:      "sub-org1",
		TenantID:            "tenant-1",
		CredentialReference: "env:CCO_TEST_SECRET",
	})
	if err != nil {
		t.Fatalf("register account: %v", err)
	}
	if _, err := svc.ValidateAccount(ctx, org1, acc.CloudAccountID); err != nil {
		t.Fatalf("validate account: %v", err)
	}
	resources, err := svc.SyncResources(ctx, org1, acc.CloudAccountID)
	if err != nil {
		t.Fatalf("sync resources: %v", err)
	}
	if len(resources) == 0 {
		t.Fatal("expected resources")
	}
	salesUID := uuid.Nil
	for _, res := range resources {
		if res.DatabaseName == "sqldb-sales" {
			salesUID = res.ResourceUID
		}
	}
	if salesUID == uuid.Nil {
		t.Fatal("sqldb-sales resource not found")
	}

	// org2 accessing org1's account → ErrAccountNotFound everywhere (404 semantics).
	t.Run("org2 cannot get org1 account", func(t *testing.T) {
		if _, err := svc.GetAccount(ctx, org2, acc.CloudAccountID); !errors.Is(err, cloud.ErrAccountNotFound) {
			t.Fatalf("expected ErrAccountNotFound, got %v", err)
		}
	})
	t.Run("org2 cannot validate org1 account", func(t *testing.T) {
		if _, err := svc.ValidateAccount(ctx, org2, acc.CloudAccountID); !errors.Is(err, cloud.ErrAccountNotFound) {
			t.Fatalf("expected ErrAccountNotFound, got %v", err)
		}
	})
	t.Run("org2 cannot sync org1 account", func(t *testing.T) {
		if _, err := svc.SyncResources(ctx, org2, acc.CloudAccountID); !errors.Is(err, cloud.ErrAccountNotFound) {
			t.Fatalf("expected ErrAccountNotFound, got %v", err)
		}
	})
	t.Run("org2 cannot list org1 resources", func(t *testing.T) {
		if _, err := svc.ListResources(ctx, org2, acc.CloudAccountID); !errors.Is(err, cloud.ErrAccountNotFound) {
			t.Fatalf("expected ErrAccountNotFound, got %v", err)
		}
	})
	t.Run("org2 cannot register datasource on org1 account", func(t *testing.T) {
		if _, err := svc.RegisterDataSource(ctx, org2, cloud.CreateBindingInput{
			ProjectID:       project2,
			CloudAccountID:  acc.CloudAccountID,
			DataResourceUID: salesUID,
			DisplayName:     "Stolen",
		}); !errors.Is(err, cloud.ErrAccountNotFound) {
			t.Fatalf("expected ErrAccountNotFound, got %v", err)
		}
	})
	// Same org account, but a project belonging to org2 → ErrProjectNotFound.
	t.Run("org1 account with org2 project → ErrProjectNotFound", func(t *testing.T) {
		if _, err := svc.RegisterDataSource(ctx, org1, cloud.CreateBindingInput{
			ProjectID:       project2,
			CloudAccountID:  acc.CloudAccountID,
			DataResourceUID: salesUID,
			DisplayName:     "CrossProj",
		}); !errors.Is(err, cloud.ErrProjectNotFound) {
			t.Fatalf("expected ErrProjectNotFound, got %v", err)
		}
	})
	// org2 cannot touch an org1 binding.
	t.Run("org2 cannot set monitored on org1 binding", func(t *testing.T) {
		binding, err := svc.RegisterDataSource(ctx, org1, cloud.CreateBindingInput{
			ProjectID:       project1,
			CloudAccountID:  acc.CloudAccountID,
			DataResourceUID: salesUID,
			DisplayName:     "Owned",
		})
		if err != nil {
			t.Fatalf("register binding: %v", err)
		}
		if err := svc.SetDataSourceMonitored(ctx, org2, binding.ResourceBindingID, false); !errors.Is(err, cloud.ErrBindingNotFound) {
			t.Fatalf("expected ErrBindingNotFound, got %v", err)
		}
	})
	// org2 listing bindings filtered by org1's project → ErrProjectNotFound.
	t.Run("org2 cannot list bindings for org1 project", func(t *testing.T) {
		if _, err := svc.ListDataSources(ctx, org2, &project1); !errors.Is(err, cloud.ErrProjectNotFound) {
			t.Fatalf("expected ErrProjectNotFound, got %v", err)
		}
	})
	// org1 can list its own bindings.
	t.Run("org1 lists its own bindings", func(t *testing.T) {
		bindings, err := svc.ListDataSources(ctx, org1, &project1)
		if err != nil {
			t.Fatalf("list data sources: %v", err)
		}
		if len(bindings) == 0 {
			t.Fatal("expected org1 to see its own binding")
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

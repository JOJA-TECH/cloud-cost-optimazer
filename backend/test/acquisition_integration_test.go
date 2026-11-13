package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	acqadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition/adapters"
	acqrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	cloudadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/adapters"
	cloudrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/repositories"
	datarepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// openAcqDataDB opens a separate in-memory SQLite for cco_data and migrates
// the raw tables the acquisition module writes to.
func openAcqDataDB(t *testing.T) *gorm.DB {
	t.Helper()
	dataDB, err := database.Open(database.Config{
		Driver: database.DriverSQLite,
		DSN:    "file:acq_data_" + uuid.New().String() + "?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatalf("open cco_data sqlite: %v", err)
	}
	if err := datarepos.MigrateDataTables(dataDB); err != nil {
		t.Fatalf("migrate cco_data tables: %v", err)
	}
	// SQLite serializes writers; a single connection avoids SQLITE_LOCKED when
	// the cycle's concurrent source jobs write at once (Postgres needs none of
	// this in production).
	if sqlDB, err := dataDB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	return dataDB
}

// seedMonitoredBinding registers an account (active), syncs resources and
// binds the first one to a project with is_monitored=true. Returns the
// binding's resource UID.
func seedMonitoredBinding(t *testing.T, db *gorm.DB, orgID, userID, projectID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	t.Setenv("CCO_TEST_SECRET", "s3cret-value")

	cloudSvc := cloud.NewService(
		cloudrepos.NewAccountRepository(db),
		cloudrepos.NewResourceRepository(db),
		cloudrepos.NewDataSourceRepository(db),
		cloudadapters.NewFakeAdapter(), cloudadapters.NewFakeAdapter(),
	)
	acc, err := cloudSvc.RegisterAccount(ctx, orgID, userID, cloud.CreateAccountInput{
		ProviderCode:        cloud.ProviderAzure,
		DisplayName:         "Acq Test Subscription",
		SubscriptionID:      "sub-acq-" + orgID.String()[:8],
		TenantID:            "tenant-1",
		CredentialReference: "env:CCO_TEST_SECRET",
	})
	if err != nil {
		t.Fatalf("register account: %v", err)
	}
	if _, err := cloudSvc.ValidateAccount(ctx, orgID, acc.CloudAccountID); err != nil {
		t.Fatalf("validate account: %v", err)
	}
	resources, err := cloudSvc.SyncResources(ctx, orgID, acc.CloudAccountID)
	if err != nil || len(resources) == 0 {
		t.Fatalf("sync resources: %v (%d)", err, len(resources))
	}
	binding, err := cloudSvc.RegisterDataSource(ctx, orgID, cloud.CreateBindingInput{
		ProjectID:       projectID,
		CloudAccountID:  acc.CloudAccountID,
		DataResourceUID: resources[0].ResourceUID,
		DisplayName:     "Monitored DB",
	})
	if err != nil {
		t.Fatalf("register binding: %v", err)
	}
	if !binding.IsMonitored {
		t.Fatal("expected monitored binding")
	}
	return resources[0].ResourceUID
}

func TestAcquisitionStackEndToEnd(t *testing.T) {
	db := openTestDB(t)
	dataDB := openAcqDataDB(t)
	ctx := context.Background()

	orgID, userID, projectID := uuid.New(), uuid.New(), uuid.New()
	seedOrgAndProject(t, db, orgID, userID, projectID)
	resourceUID := seedMonitoredBinding(t, db, orgID, userID, projectID)

	targets := acqrepos.NewTargetRepository(db)
	repo := datarepos.NewDataRepository(dataDB)
	svc := acquisition.NewService(targets, repo, repo,
		acqadapters.NewFakeMonitor(), acqadapters.NewFakeDiagnostics(), acqadapters.NewFakeQueryStore(),
		acqadapters.NewFakeConfig(), acqadapters.NewFakeCost(),
		acquisition.Config{
			MonitorDelay:    90 * time.Second,
			MonitorLookback: 5 * time.Minute,
			SQLUser:         "workload",
			SQLPassword:     "pw",
			FetchParallel:   2,
		},
	)

	t.Run("cycle runs monitor + config + dmv + query store for the monitored target", func(t *testing.T) {
		outcomes, err := svc.RunCycle(ctx, &orgID, nil)
		if err != nil {
			t.Fatalf("RunCycle: %v", err)
		}
		// Now 4 sources: azure_monitor_metrics, arm_config, sql_diagnostics, query_store
		if len(outcomes) != 4 {
			t.Fatalf("outcomes = %d, want 4", len(outcomes))
		}
		sources := map[string]bool{}
		for _, out := range outcomes {
			sources[out.Source] = true
			if out.Status != "ok" {
				t.Errorf("source %s status %q detail %q", out.Source, out.Status, out.Detail)
			}
			if out.ResourceUID != resourceUID {
				t.Errorf("outcome resource %s, want %s", out.ResourceUID, resourceUID)
			}
			if out.RowsIngested <= 0 {
				t.Errorf("source %s rows_ingested = %d, want > 0", out.Source, out.RowsIngested)
			}
		}
		for _, want := range []string{acquisition.SourceAzureMonitor, "arm_config", acquisition.SourceSQLDiag, acquisition.SourceQueryStore} {
			if !sources[want] {
				t.Errorf("missing source %s in outcomes %v", want, outcomes)
			}
		}
	})

	t.Run("runs are queryable and org-scoped", func(t *testing.T) {
		runs, err := svc.ListRuns(ctx, orgID, acquisition.RunFilter{Source: acquisition.SourceAzureMonitor})
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(runs) == 0 {
			t.Fatal("expected at least one monitor run")
		}
		if runs[0].Status != "ok" {
			t.Errorf("run status = %q, want ok", runs[0].Status)
		}
		got, err := svc.GetRun(ctx, orgID, runs[0].CollectorRunID)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		if got.CollectorRunID != runs[0].CollectorRunID {
			t.Errorf("GetRun returned id %d, want %d", got.CollectorRunID, runs[0].CollectorRunID)
		}

		// Another organization sees nothing (empty list) and gets 404
		// semantics on a foreign run.
		otherRuns, err := svc.ListRuns(ctx, uuid.New(), acquisition.RunFilter{})
		if err != nil {
			t.Fatalf("ListRuns (other org): %v", err)
		}
		if len(otherRuns) != 0 {
			t.Errorf("other org sees %d runs, want 0", len(otherRuns))
		}
		if _, err := svc.GetRun(ctx, uuid.New(), runs[0].CollectorRunID); !errors.Is(err, acquisition.ErrRunNotFound) {
			t.Errorf("GetRun from other org = %v, want ErrRunNotFound", err)
		}
	})

	t.Run("unknown resource filter → ErrResourceNotFound", func(t *testing.T) {
		other := uuid.New()
		if _, err := svc.RunCycle(ctx, &orgID, &other); !errors.Is(err, acquisition.ErrResourceNotFound) {
			t.Errorf("err = %v, want ErrResourceNotFound", err)
		}
	})

	t.Run("repeat cycle is safe (idempotent persistence)", func(t *testing.T) {
		outcomes, err := svc.RunCycle(ctx, &orgID, nil)
		if err != nil {
			t.Fatalf("second RunCycle: %v", err)
		}
		for _, out := range outcomes {
			if out.Status != "ok" {
				t.Errorf("source %s status %q on repeat cycle", out.Source, out.Status)
			}
		}
	})

	t.Run("no storage configured → ErrStorageUnavailable", func(t *testing.T) {
		noStore := acquisition.NewService(targets, nil, nil,
			acqadapters.NewFakeMonitor(), acqadapters.NewFakeDiagnostics(), acqadapters.NewFakeQueryStore(),
			acqadapters.NewFakeConfig(), acqadapters.NewFakeCost(),
			acquisition.Config{SQLUser: "w", SQLPassword: "p"},
		)
		if _, err := noStore.RunCycle(ctx, &orgID, nil); !errors.Is(err, acquisition.ErrStorageUnavailable) {
			t.Errorf("err = %v, want ErrStorageUnavailable", err)
		}
	})
}

// seedOrgAndProject inserts the organization and project rows shared by the
// acquisition integration test.
func seedOrgAndProject(t *testing.T, db *gorm.DB, orgID, userID, projectID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	if err := db.Table("organizations").Create(&database.OrganizationModel{
		OrganizationID: orgID,
		Name:           "Acq Org",
		Slug:           "acq-org-" + orgID.String()[:8],
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := db.Table("projects").Create(&database.ProjectModel{
		ProjectID:      projectID,
		OrganizationID: orgID,
		Name:           "Acq Project",
		Status:         "active",
		CreatedBy:      &userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
}

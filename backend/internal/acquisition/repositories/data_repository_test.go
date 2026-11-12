package repositories

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func openDataDB(t *testing.T) *gorm.DB {
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
	if err := MigrateDataTables(db); err != nil {
		t.Fatalf("migrate cco_data: %v", err)
	}
	return db
}

func ptrF(v float64) *float64 { return &v }
func ptrI(v int) *int         { return &v }

func TestDataRepositoryEnsureResource(t *testing.T) {
	db := openDataDB(t)
	repo := NewDataRepository(db)
	ctx := context.Background()

	uid := uuid.New()
	t1 := acquisition.Target{
		ResourceUID:     uid,
		CloudAccountID:  uuid.New(),
		AzureResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Sql/servers/s/databases/d",
		SubscriptionID:  "sub",
		ResourceGroup:   "rg",
		ServerName:      "s",
		DatabaseName:    "db",
		RegionCode:      "eastus2",
		Kind:            ptrF0("v12.0,user"),
	}

	id1, err := repo.EnsureResource(ctx, t1)
	if err != nil {
		t.Fatalf("EnsureResource 1st: %v", err)
	}
	if id1 <= 0 {
		t.Fatalf("EnsureResource returned id %d, want > 0", id1)
	}

	// Idempotent: same azure_resource_id -> same resource_id.
	id2, err := repo.EnsureResource(ctx, t1)
	if err != nil {
		t.Fatalf("EnsureResource 2nd: %v", err)
	}
	if id2 != id1 {
		t.Errorf("EnsureResource 2nd id = %d, want %d", id2, id1)
	}

	// resource_uid preserved (contract UID intact).
	var uidStr string
	if err := db.Table(database.TableName(db, "core", "resource")).
		Where("resource_id = ?", id1).Pluck("resource_uid", &uidStr).Error; err != nil {
		t.Fatalf("pluck resource_uid: %v", err)
	}
	uidOut, err := uuid.Parse(uidStr)
	if err != nil {
		t.Fatalf("parse resource_uid: %v", err)
	}
	if uidOut != uid {
		t.Errorf("resource_uid = %s, want %s", uidOut, uid)
	}

	// Changing fields updates the row (on conflict do updates).
	t1.SubscriptionID = "sub-changed"
	if _, err := repo.EnsureResource(ctx, t1); err != nil {
		t.Fatalf("EnsureResource update: %v", err)
	}
	var subOut string
	if err := db.Table(database.TableName(db, "core", "resource")).
		Where("resource_id = ?", id1).Pluck("subscription_id", &subOut).Error; err != nil {
		t.Fatalf("pluck subscription_id: %v", err)
	}
	if subOut != "sub-changed" {
		t.Errorf("subscription_id = %q, want sub-changed", subOut)
	}
}

func TestDataRepositoryInsertMetricReadingsDedup(t *testing.T) {
	db := openDataDB(t)
	repo := NewDataRepository(db)
	ctx := context.Background()

	resourceID := ensureResourceT(t, repo, ctx, uuid.New(), "/arm/metric-res")

	ts := time.Now().UTC().Truncate(time.Minute)
	readings := []acquisition.RawReading{
		{MetricCode: "cpu_percent", Unit: "Percent", Timestamp: ts, IntervalSeconds: 60, Average: ptrF(42), Source: acquisition.SourceAzureMonitor},
		{MetricCode: "sessions_count", Unit: "Count", Timestamp: ts, IntervalSeconds: 60, Average: ptrF(7), Source: acquisition.SourceAzureMonitor},
	}

	n1, err := repo.InsertMetricReadings(ctx, resourceID, readings)
	if err != nil {
		t.Fatalf("InsertMetricReadings 1st: %v", err)
	}
	if n1 != 2 {
		t.Errorf("InsertMetricReadings 1st = %d, want 2", n1)
	}

	// Same readings again -> 0 inserted (idempotent ON CONFLICT DO NOTHING).
	n2, err := repo.InsertMetricReadings(ctx, resourceID, readings)
	if err != nil {
		t.Fatalf("InsertMetricReadings 2nd: %v", err)
	}
	if n2 != 0 {
		t.Errorf("InsertMetricReadings 2nd = %d, want 0", n2)
	}

	var count int64
	if err := db.Table(database.TableName(db, "raw", "metric_reading")).
		Where("resource_id = ?", resourceID).Count(&count).Error; err != nil {
		t.Fatalf("count metric_reading: %v", err)
	}
	if count != 2 {
		t.Errorf("metric_reading count = %d, want 2", count)
	}
}

func TestDataRepositoryListRuns(t *testing.T) {
	db := openDataDB(t)
	repo := NewDataRepository(db)
	ctx := context.Background()

	uid1 := uuid.New()
	uid2 := uuid.New()
	id1 := ensureResourceT(t, repo, ctx, uid1, "/arm/res-one")
	id2 := ensureResourceT(t, repo, ctx, uid2, "/arm/res-two")

	run1, _ := repo.StartRun(ctx, acquisition.SourceAzureMonitor, id1, "sub-a")
	run2, _ := repo.StartRun(ctx, acquisition.SourceSQLDiag, id1, "sub-a")
	run3, _ := repo.StartRun(ctx, acquisition.SourceQueryStore, id2, "sub-b")
	// mark run2 finished with a status so filtering works
	_ = repo.FinishRun(ctx, run2, "ok", 5, "")

	// Empty uids -> empty result without querying.
	empty, err := repo.ListRuns(ctx, nil, acquisition.RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns(nil uids): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListRuns(nil uids) = %d, want 0", len(empty))
	}

	// Filter by uids only, newest first.
	all, err := repo.ListRuns(ctx, []uuid.UUID{uid1, uid2}, acquisition.RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns(uids): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListRuns(uids) = %d, want 3", len(all))
	}
	if all[0].CollectorRunID != run3 || all[1].CollectorRunID != run2 || all[2].CollectorRunID != run1 {
		t.Errorf("run order = %v, want DESC by id (%d, %d, %d)", []int64{all[0].CollectorRunID, all[1].CollectorRunID, all[2].CollectorRunID}, run3, run2, run1)
	}

	// Filter by source.
	mon, err := repo.ListRuns(ctx, []uuid.UUID{uid1, uid2}, acquisition.RunFilter{Source: acquisition.SourceAzureMonitor})
	if err != nil {
		t.Fatalf("ListRuns(source): %v", err)
	}
	if len(mon) != 1 || mon[0].CollectorRunID != run1 {
		t.Errorf("ListRuns(source=azure_monitor) = %v, want run %d", mon, run1)
	}

	// Filter by status.
	oks, err := repo.ListRuns(ctx, []uuid.UUID{uid1, uid2}, acquisition.RunFilter{Status: "ok"})
	if err != nil {
		t.Fatalf("ListRuns(status=ok): %v", err)
	}
	if len(oks) != 1 || oks[0].CollectorRunID != run2 {
		t.Errorf("ListRuns(status=ok) = %v, want run %d", oks, run2)
	}

	// Filter by resource uid.
	only1, err := repo.ListRuns(ctx, []uuid.UUID{uid1, uid2}, acquisition.RunFilter{ResourceUID: &uid1})
	if err != nil {
		t.Fatalf("ListRuns(resource_uid): %v", err)
	}
	if len(only1) != 2 {
		t.Errorf("ListRuns(resource_uid=uid1) = %d, want 2", len(only1))
	}
}

func TestDataRepositoryGetRunNotFound(t *testing.T) {
	db := openDataDB(t)
	repo := NewDataRepository(db)
	_, err := repo.GetRun(context.Background(), 9999)
	if !errors.Is(err, acquisition.ErrRunNotFound) {
		t.Errorf("GetRun(9999) = %v, want ErrRunNotFound", err)
	}
}

func ensureResourceT(t *testing.T, repo acquisition.AcquisitionRepository, ctx context.Context, uid uuid.UUID, armID string) int64 {
	id, err := repo.EnsureResource(ctx, acquisition.Target{
		ResourceUID:     uid,
		CloudAccountID:  uuid.New(),
		AzureResourceID: armID,
		SubscriptionID:  "sub",
		ResourceGroup:   "rg",
		ServerName:      "s",
		DatabaseName:    "db",
		RegionCode:      "eastus2",
	})
	if err != nil {
		t.Fatalf("EnsureResource: %v", err)
	}
	return id
}

func ptrF0(s string) *string { return &s }

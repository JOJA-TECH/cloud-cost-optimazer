package repositories

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
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
func ptrS(v string) *string   { return &v }

func newResourceRef(uid uuid.UUID, armID string) data.ResourceRef {
	return data.ResourceRef{
		ResourceUID:     uid,
		SubscriptionID:  "sub",
		ResourceGroup:   "rg",
		ServerName:      "srv",
		DatabaseName:    "db",
		AzureResourceID: armID,
		RegionCode:      "eastus2",
		Kind:            ptrS("v12.0,user,vcore,serverless"),
	}
}

func ensureResourceT(t *testing.T, repo *DataRepository, ctx context.Context, uid uuid.UUID, armID string) int64 {
	t.Helper()
	id, err := repo.EnsureResource(ctx, newResourceRef(uid, armID))
	if err != nil {
		t.Fatalf("EnsureResource: %v", err)
	}
	return id
}

func TestDataRepositoryEnsureResource(t *testing.T) {
	db := openDataDB(t)
	repo := NewDataRepository(db)
	ctx := context.Background()

	uid := uuid.New()
	id1 := ensureResourceT(t, repo, ctx, uid, "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Sql/servers/s/databases/d")

	// Idempotent: same azure_resource_id -> same resource_id.
	id2 := ensureResourceT(t, repo, ctx, uid, "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Sql/servers/s/databases/d")
	if id2 != id1 {
		t.Errorf("EnsureResource 2nd id = %d, want %d", id2, id1)
	}

	// resource_uid preserved (contract UID intact).
	var uidStr string
	if err := db.Table(database.TableName(db, "core", "resource")).
		Where("resource_id = ?", id1).Pluck("resource_uid", &uidStr).Error; err != nil {
		t.Fatalf("pluck resource_uid: %v", err)
	}
	if got, err := uuid.Parse(uidStr); err != nil || got != uid {
		t.Errorf("resource_uid = %s (err %v), want %s", uidStr, err, uid)
	}

	// Changing fields updates the row (on conflict do updates).
	ref := newResourceRef(uid, "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Sql/servers/s/databases/d")
	ref.SubscriptionID = "sub-changed"
	if _, err := repo.EnsureResource(ctx, ref); err != nil {
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

func TestWriteMetricReadingsRoundtripAndDedup(t *testing.T) {
	db := openDataDB(t)
	repo := NewDataRepository(db)
	ctx := context.Background()

	resourceID := ensureResourceT(t, repo, ctx, uuid.New(), "/arm/metric-res")

	base := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	metrics := []data.Metric{
		{MetricCode: "cpu_percent", TimestampUTC: base, IntervalSeconds: 60, AvgValue: ptrF(42.5), MaxValue: ptrF(60)},
		{MetricCode: "cpu_percent", TimestampUTC: base.Add(time.Minute), IntervalSeconds: 60, AvgValue: ptrF(43.5)},
	}

	n1, err := repo.WriteMetricReadings(ctx, resourceID, metrics)
	if err != nil || n1 != 2 {
		t.Fatalf("WriteMetricReadings 1st = (%d, %v), want (2, nil)", n1, err)
	}
	// Idempotent rewrite: ON CONFLICT DO NOTHING.
	n2, err := repo.WriteMetricReadings(ctx, resourceID, metrics)
	if err != nil || n2 != 0 {
		t.Fatalf("WriteMetricReadings 2nd = (%d, %v), want (0, nil)", n2, err)
	}

	series, err := repo.MetricSeries(ctx, data.DataQuery{
		ResourceUID: mustUID(t, db, resourceID), MetricCode: "cpu_percent",
		FromUTC: base, ToUTC: base.Add(2 * time.Minute),
	}, 0)
	if err != nil {
		t.Fatalf("MetricSeries: %v", err)
	}
	if len(series) != 2 || series[0].TimestampUTC.After(series[1].TimestampUTC) {
		t.Fatalf("series = %+v, want 2 points ascending", series)
	}
	if *series[0].AvgValue != 42.5 || *series[0].MaxValue != 60 {
		t.Errorf("point values = %+v, want avg 42.5 max 60", series[0])
	}
}

func TestMetricSeriesLimitAndUnknownResource(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()

	uid := uuid.New()
	resourceID := ensureResourceT(t, repo, ctx, uid, "/arm/limit-res")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	metrics := make([]data.Metric, 0, 3)
	for i := 0; i < 3; i++ {
		metrics = append(metrics, data.Metric{
			MetricCode: "storage_percent", TimestampUTC: base.Add(time.Duration(i) * time.Minute),
			IntervalSeconds: 60, AvgValue: ptrF(float64(i)),
		})
	}
	if n, err := repo.WriteMetricReadings(ctx, resourceID, metrics); err != nil || n != 3 {
		t.Fatalf("WriteMetricReadings = (%d, %v), want (3, nil)", n, err)
	}

	series, err := repo.MetricSeries(ctx, data.DataQuery{
		ResourceUID: uid, MetricCode: "storage_percent",
		FromUTC: base, ToUTC: base.Add(3 * time.Minute),
	}, 2)
	if err != nil || len(series) != 2 {
		t.Fatalf("MetricSeries limit = (%d points, %v), want (2, nil)", len(series), err)
	}

	_, err = repo.MetricSeries(ctx, data.DataQuery{
		ResourceUID: uuid.New(), MetricCode: "cpu_percent",
		FromUTC: base, ToUTC: base.Add(time.Hour),
	}, 0)
	if !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("MetricSeries unknown resource err = %v, want ErrNotFound", err)
	}
}

func TestWriteResourceStatsAndQueryStore(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()

	resourceID := ensureResourceT(t, repo, ctx, uuid.New(), "/arm/dmv-res")
	end := time.Date(2026, 11, 1, 11, 0, 0, 0, time.UTC)

	n, err := repo.WriteResourceStats(ctx, resourceID, []data.ResourceStat{{
		EndTimeUTC: end, AvgCPUPercent: ptrF(12.5), AvgDataIOPercent: ptrF(3),
	}})
	if err != nil || n != 1 {
		t.Fatalf("WriteResourceStats = (%d, %v), want (1, nil)", n, err)
	}
	if n, err := repo.WriteResourceStats(ctx, resourceID, []data.ResourceStat{{
		EndTimeUTC: end, AvgCPUPercent: ptrF(12.5),
	}}); err != nil || n != 0 {
		t.Fatalf("WriteResourceStats dedup = (%d, %v), want (0, nil)", n, err)
	}

	snap := end
	n, err = repo.WriteQueryStore(ctx, resourceID,
		[]data.QueryStoreQueryRecord{{QueryID: 7, QueryHash: ptrS("hash"), SnapshotAtUTC: snap}},
		[]data.QueryStoreRuntimeRecord{{QueryID: 7, IntervalStartUTC: snap, IntervalEndUTC: snap.Add(time.Second),
			CountExecutions: ptrI64(3), AvgDurationUS: ptrF(1200), SnapshotAtUTC: snap}})
	if err != nil || n != 2 {
		t.Fatalf("WriteQueryStore = (%d, %v), want (2, nil)", n, err)
	}
	if n, err := repo.WriteQueryStore(ctx, resourceID,
		[]data.QueryStoreQueryRecord{{QueryID: 7, SnapshotAtUTC: snap}},
		[]data.QueryStoreRuntimeRecord{{QueryID: 7, IntervalStartUTC: snap, IntervalEndUTC: snap.Add(time.Second), SnapshotAtUTC: snap}}); err != nil || n != 0 {
		t.Fatalf("WriteQueryStore dedup = (%d, %v), want (0, nil)", n, err)
	}
}

func TestUpsertCostDailyIdempotent(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()

	uid := uuid.New()
	ensureResourceT(t, repo, ctx, uid, "/arm/cost-res")
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	rec := data.CostRecord{
		ResourceUID:    &uid,
		ProviderCode:   "azure",
		ScopeType:      "subscription",
		ScopeName:      "sub",
		ServiceName:    "SQL Database",
		RegionCode:     ptrS("eastus2"),
		UsageDate:      day,
		Currency:       "USD",
		SourceChargeID: ptrS("cost:sub:2026-11-01:abc"),
		PreTaxCost:     0.36,
		UsageQuantity:  ptrF(24),
		UsageUnit:      ptrS("1 Hour"),
		PricingModel:   ptrS("consumption"),
		ChargeCategory: ptrS("usage"),
	}
	n, err := repo.UpsertCostDaily(ctx, []data.CostRecord{rec})
	if err != nil || n != 1 {
		t.Fatalf("UpsertCostDaily 1st = (%d, %v), want (1, nil)", n, err)
	}
	if n, err := repo.UpsertCostDaily(ctx, []data.CostRecord{rec}); err != nil || n != 0 {
		t.Fatalf("UpsertCostDaily 2nd = (%d, %v), want (0, nil)", n, err)
	}

	costs, err := repo.CostDailyByResource(ctx, uid, day, day.Add(24*time.Hour), 0)
	if err != nil || len(costs) != 1 {
		t.Fatalf("CostDailyByResource = (%d rows, %v), want 1", len(costs), err)
	}
	if costs[0].ResourceUID == nil || *costs[0].ResourceUID != uid {
		t.Errorf("ResourceUID = %v, want %s", costs[0].ResourceUID, uid)
	}
	if costs[0].PreTaxCost != 0.36 || costs[0].Source != "azure_cost_management" {
		t.Errorf("cost fields = %+v", costs[0])
	}

	// ByScope includes the row (resource known) and date window works.
	byScope, err := repo.CostDailyByScope(ctx, "subscription", "sub", day, day.Add(24*time.Hour), 0)
	if err != nil || len(byScope) != 1 {
		t.Fatalf("CostDailyByScope = (%d rows, %v), want 1", len(byScope), err)
	}
	empty, err := repo.CostDailyByScope(ctx, "subscription", "sub", day.Add(24*time.Hour), day.Add(48*time.Hour), 0)
	if err != nil || len(empty) != 0 {
		t.Fatalf("CostDailyByScope next day = (%d rows, %v), want 0", len(empty), err)
	}
}

func TestUpsertCostDailyValidation(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.UpsertCostDaily(ctx, []data.CostRecord{{
		ProviderCode: "azure", ScopeType: "subscription", ScopeName: "sub",
		ServiceName: "SQL Database", UsageDate: day, Currency: "USD", PreTaxCost: 1,
	}})
	if !errors.Is(err, data.ErrInvalidCostRecord) {
		t.Fatalf("nil source_charge_id err = %v, want ErrInvalidCostRecord", err)
	}

	_, err = repo.UpsertCostDaily(ctx, []data.CostRecord{{
		ProviderCode: "azure", ScopeType: "bogus", ScopeName: "sub",
		ServiceName: "SQL Database", UsageDate: day, Currency: "USD", PreTaxCost: 1,
		SourceChargeID: ptrS("x"),
	}})
	if !errors.Is(err, data.ErrInvalidCostRecord) {
		t.Fatalf("bad scope_type err = %v, want ErrInvalidCostRecord", err)
	}
}

func TestUpsertCostDailyUnknownResourceKeptWithNull(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()
	day := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	unknown := uuid.New()
	n, err := repo.UpsertCostDaily(ctx, []data.CostRecord{{
		ResourceUID:    &unknown,
		ProviderCode:   "azure",
		ScopeType:      "subscription",
		ScopeName:      "sub",
		ServiceName:    "Storage",
		UsageDate:      day,
		Currency:       "USD",
		SourceChargeID: ptrS("cost:sub:2026-11-02:storage"),
		PreTaxCost:     0.02,
	}})
	if err != nil || n != 1 {
		t.Fatalf("UpsertCostDaily unknown uid = (%d, %v), want (1, nil)", n, err)
	}

	rows, err := repo.CostDailyByScope(ctx, "subscription", "sub", day, day.Add(24*time.Hour), 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("CostDailyByScope = (%d rows, %v), want 1", len(rows), err)
	}
	if rows[0].ResourceUID != nil {
		t.Errorf("ResourceUID = %v, want nil (unmapped cost)", rows[0].ResourceUID)
	}
}

func TestUpsertConfigSnapshot(t *testing.T) {
	db := openDataDB(t)
	repo := NewDataRepository(db)
	ctx := context.Background()

	uid := uuid.New()
	ensureResourceT(t, repo, ctx, uid, "/arm/config-res")

	// Seed a known SKU.
	if err := db.Table(database.TableName(db, "catalog", "sql_sku")).Create(&sqlSkuRow{
		SKUName: "GP_S_Gen5_2", Tier: "GeneralPurpose", ComputeModel: "serverless",
		CapacityVCores: 2.0, InScopeR1: true, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed sql_sku: %v", err)
	}

	observed := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	snap := data.ConfigurationSnapshot{
		ResourceUID:       uid,
		SKUName:           ptrS("GP_S_Gen5_2"),
		Tier:              ptrS("GeneralPurpose"),
		Family:            ptrS("Gen5"),
		CapacityVCores:    ptrF(2.0),
		ComputeModel:      ptrS("serverless"),
		MaxSizeBytes:      ptrI64(32 * 1024 * 1024 * 1024),
		AutoPauseDelayMin: ptrI(60),
		Status:            ptrS("Online"),
		FreeLimit:         ptrBool(true),
		SourcePayload:     ptrS(`{"sku":{"name":"GP_S_Gen5_2"}}`),
		ObservedAtUTC:     observed,
	}

	affected, err := repo.UpsertConfigSnapshot(ctx, snap)
	if err != nil || affected != 1 {
		t.Fatalf("UpsertConfigSnapshot 1st = (%d, %v), want (1, nil)", affected, err)
	}
	// Same (resource_id, observed_at_utc) -> no new row.
	if affected, err := repo.UpsertConfigSnapshot(ctx, snap); err != nil || affected != 0 {
		t.Fatalf("UpsertConfigSnapshot dup = (%d, %v), want (0, nil)", affected, err)
	}

	latest, err := repo.LatestConfigSnapshot(ctx, uid)
	if err != nil || latest == nil {
		t.Fatalf("LatestConfigSnapshot = (%v, %v), want a snapshot", latest, err)
	}
	if *latest.SKUName != "GP_S_Gen5_2" || *latest.Tier != "GeneralPurpose" ||
		*latest.ComputeModel != "serverless" || !*latest.FreeLimit {
		t.Errorf("latest = %+v", latest)
	}

	// Unknown SKU name: still stored (sku_id NULL, textual name kept).
	snapUnknown := snap
	snapUnknown.SKUName = ptrS("GP_X_99")
	snapUnknown.ObservedAtUTC = observed.Add(time.Minute)
	if affected, err := repo.UpsertConfigSnapshot(ctx, snapUnknown); err != nil || affected != 1 {
		t.Fatalf("UpsertConfigSnapshot unknown sku = (%d, %v), want (1, nil)", affected, err)
	}
	latest, err = repo.LatestConfigSnapshot(ctx, uid)
	if err != nil || latest == nil || *latest.SKUName != "GP_X_99" {
		t.Fatalf("LatestConfigSnapshot = (%v, %v), want GP_X_99", latest, err)
	}

	// Unknown resource -> domain error.
	snap.ResourceUID = uuid.New()
	if _, err := repo.UpsertConfigSnapshot(ctx, snap); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("unknown resource err = %v, want ErrNotFound", err)
	}
}

func TestConfigSnapshotHistory(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()

	uid := uuid.New()
	ensureResourceT(t, repo, ctx, uid, "/arm/history-res")
	observed := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	for i, name := range []string{"GP_S_Gen5_2", "GP_Gen5_4"} {
		if _, err := repo.UpsertConfigSnapshot(ctx, data.ConfigurationSnapshot{
			ResourceUID: uid, SKUName: ptrS(name), ObservedAtUTC: observed.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("UpsertConfigSnapshot %d: %v", i, err)
		}
	}

	history, err := repo.ConfigSnapshotHistory(ctx, uid, time.Time{}, time.Time{}, 0)
	if err != nil || len(history) != 2 {
		t.Fatalf("ConfigSnapshotHistory = (%d rows, %v), want 2", len(history), err)
	}
	if *history[0].SKUName != "GP_Gen5_4" || *history[1].SKUName != "GP_S_Gen5_2" {
		t.Errorf("history order = [%s, %s], want newest first", *history[0].SKUName, *history[1].SKUName)
	}

	// Window [01:00, ...) excludes the first snapshot.
	windowed, err := repo.ConfigSnapshotHistory(ctx, uid, observed.Add(time.Hour), time.Time{}, 0)
	if err != nil || len(windowed) != 1 || *windowed[0].SKUName != "GP_Gen5_4" {
		t.Fatalf("windowed history = (%v, %v), want only GP_Gen5_4", windowed, err)
	}
}

func TestCoverageEndToEnd(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()

	uid := uuid.New()
	resourceID := ensureResourceT(t, repo, ctx, uid, "/arm/coverage-res")
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	metrics := make([]data.Metric, 0, 3)
	for _, h := range []int{0, 1, 2} {
		metrics = append(metrics, data.Metric{
			MetricCode: "cpu_percent",
			TimestampUTC: day.Add(time.Duration(h)*time.Hour + 5*time.Minute),
			IntervalSeconds: 60, AvgValue: ptrF(10),
		})
	}
	if _, err := repo.WriteMetricReadings(ctx, resourceID, metrics); err != nil {
		t.Fatalf("WriteMetricReadings: %v", err)
	}

	svc := data.NewService(repo)
	rep, err := svc.Coverage(ctx, uid, "cpu_percent", day, day.Add(24*time.Hour), data.BucketHour, 0.8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if rep.WindowsTotal != 24 || rep.WindowsWithData != 3 || rep.State != data.CoverageInsufficient {
		t.Fatalf("report = %+v", rep)
	}
}

func TestListRunsAndGetRun(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()

	uid := uuid.New()
	resourceID := ensureResourceT(t, repo, ctx, uid, "/arm/runs-res")

	runID, err := repo.StartRun(ctx, "azure_monitor_metrics", resourceID, "scope-1")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := repo.FinishRun(ctx, runID, "ok", 12, ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	runs, err := repo.ListRuns(ctx, []uuid.UUID{uid}, data.RunFilter{})
	if err != nil || len(runs) != 1 {
		t.Fatalf("ListRuns = (%d runs, %v), want 1", len(runs), err)
	}
	if runs[0].CollectorRunID != runID || runs[0].Status != "ok" || runs[0].ResourceUID == nil || *runs[0].ResourceUID != uid {
		t.Errorf("run = %+v", runs[0])
	}

	// Empty uids never yields an unscoped query.
	if runs, err := repo.ListRuns(ctx, []uuid.UUID{}, data.RunFilter{}); err != nil || len(runs) != 0 {
		t.Fatalf("ListRuns empty uids = (%d, %v), want (0, nil)", len(runs), err)
	}
	// Filter by source.
	if runs, err := repo.ListRuns(ctx, []uuid.UUID{uid}, data.RunFilter{Source: "query_store"}); err != nil || len(runs) != 0 {
		t.Fatalf("ListRuns filtered = (%d, %v), want 0", len(runs), err)
	}

	run, err := repo.GetRun(ctx, runID)
	if err != nil || run.CollectorRunID != runID {
		t.Fatalf("GetRun = (%+v, %v)", run, err)
	}
	if _, err := repo.GetRun(ctx, 99999); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("GetRun missing err = %v, want ErrNotFound", err)
	}
}

func TestMetricReadingTimestamps(t *testing.T) {
	repo := NewDataRepository(openDataDB(t))
	ctx := context.Background()

	uid := uuid.New()
	resourceID := ensureResourceT(t, repo, ctx, uid, "/arm/ts-res")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	metrics := make([]data.Metric, 0, 3)
	for i := 0; i < 3; i++ {
		metrics = append(metrics, data.Metric{
			MetricCode: "cpu_percent", TimestampUTC: base.Add(time.Duration(i) * time.Hour),
			IntervalSeconds: 60, AvgValue: ptrF(5),
		})
	}
	if _, err := repo.WriteMetricReadings(ctx, resourceID, metrics); err != nil {
		t.Fatalf("WriteMetricReadings: %v", err)
	}

	ts, err := repo.MetricReadingTimestamps(ctx, uid, "cpu_percent", base.Add(30*time.Minute), base.Add(3*time.Hour))
	if err != nil || len(ts) != 2 {
		t.Fatalf("MetricReadingTimestamps = (%d, %v), want 2", len(ts), err)
	}
	if !ts[0].Before(ts[1]) {
		t.Errorf("timestamps not ascending: %v", ts)
	}
}

func mustUID(t *testing.T, db *gorm.DB, resourceID int64) uuid.UUID {
	t.Helper()
	var uidStr string
	if err := db.Table(database.TableName(db, "core", "resource")).
		Where("resource_id = ?", resourceID).Pluck("resource_uid", &uidStr).Error; err != nil {
		t.Fatalf("pluck resource_uid: %v", err)
	}
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		t.Fatalf("parse resource_uid: %v", err)
	}
	return uid
}

func ptrI64(v int64) *int64 { return &v }
func ptrBool(v bool) *bool  { return &v }

package acquisition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	"github.com/google/uuid"
)

// ---------- in-memory stubs ----------

type stubTargets struct {
	targets []Target
	err     error
}

func (s *stubTargets) ListMonitoredTargets(ctx context.Context, orgID *uuid.UUID) ([]Target, error) {
	if s.err != nil {
		return nil, s.err
	}
	if orgID == nil {
		return s.targets, nil
	}
	out := []Target{}
	for _, t := range s.targets {
		out = append(out, t)
	}
	return out, nil
}

func (s *stubTargets) ResolveCredentials(ctx context.Context, accountID uuid.UUID) (cloud.ConnectionCredentials, error) {
	return cloud.ConnectionCredentials{TenantID: "tenant", ClientID: "client", ClientSecret: "secret", SubscriptionID: "sub"}, nil
}

// memStore implements data.DataWriter and data.RunReader in memory: the
// acquisition service must only ever reach cco_data through those contracts.
type memStore struct {
	nextRunID    int64
	runs         []data.RunRecord
	resources    map[string]int64
	metrics      []data.Metric
	stats        []data.ResourceStat
	qsQueries    []data.QueryStoreQueryRecord
	qsStats      []data.QueryStoreRuntimeRecord
	finishStatus map[int64]string
	finishRows   map[int64]int
}

func newMemStore() *memStore {
	return &memStore{
		resources:    map[string]int64{},
		finishStatus: map[int64]string{},
		finishRows:   map[int64]int{},
	}
}

func (m *memStore) EnsureResource(ctx context.Context, ref data.ResourceRef) (int64, error) {
	if id, ok := m.resources[ref.AzureResourceID]; ok {
		return id, nil
	}
	id := int64(len(m.resources) + 1)
	m.resources[ref.AzureResourceID] = id
	return id, nil
}

func (m *memStore) StartRun(ctx context.Context, source string, resourceID int64, scope string) (int64, error) {
	m.nextRunID++
	scopeCopy := scope
	m.runs = append(m.runs, data.RunRecord{
		CollectorRunID: m.nextRunID, Source: source, Scope: &scopeCopy,
		StartedAtUTC: time.Now().UTC(), Status: "partial",
	})
	return m.nextRunID, nil
}

func (m *memStore) FinishRun(ctx context.Context, runID int64, status string, rowsIngested int, errMsg string) error {
	m.finishStatus[runID] = status
	m.finishRows[runID] = rowsIngested
	return nil
}

func (m *memStore) WriteMetricReadings(ctx context.Context, resourceID int64, metrics []data.Metric) (int, error) {
	m.metrics = append(m.metrics, metrics...)
	return len(metrics), nil
}

func (m *memStore) WriteResourceStats(ctx context.Context, resourceID int64, stats []data.ResourceStat) (int, error) {
	m.stats = append(m.stats, stats...)
	return len(stats), nil
}

func (m *memStore) WriteQueryStore(ctx context.Context, resourceID int64, queries []data.QueryStoreQueryRecord, stats []data.QueryStoreRuntimeRecord) (int, error) {
	m.qsQueries = append(m.qsQueries, queries...)
	m.qsStats = append(m.qsStats, stats...)
	return len(queries) + len(stats), nil
}

func (m *memStore) UpsertCostDaily(ctx context.Context, records []data.CostRecord) (int, error) {
	return len(records), nil
}

func (m *memStore) UpsertConfigSnapshot(ctx context.Context, snap data.ConfigurationSnapshot) (int64, error) {
	return 1, nil
}

func (m *memStore) WriteMetricSamples(ctx context.Context, resourceID int64, samples []data.RawSample) (int, error) {
	return len(samples), nil
}

func (m *memStore) ListRuns(ctx context.Context, uids []uuid.UUID, filter data.RunFilter) ([]data.RunRecord, error) {
	out := []data.RunRecord{}
	for _, r := range m.runs {
		if filter.Source != "" && r.Source != filter.Source {
			continue
		}
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (m *memStore) GetRun(ctx context.Context, id int64) (data.RunRecord, error) {
	for _, r := range m.runs {
		if r.CollectorRunID == id {
			return r, nil
		}
	}
	return data.RunRecord{}, data.ErrNotFound
}

type stubMonitor struct {
	readings []RawReading
	err      error
}

func (s *stubMonitor) FetchPlatformMetrics(ctx context.Context, creds cloud.ConnectionCredentials, azureResourceID string, start, end time.Time) ([]RawReading, error) {
	return s.readings, s.err
}

type stubDiag struct {
	stats    []DBResourceStat
	readings []RawReading
	lastSeen time.Time
	err      error
}

func (s *stubDiag) FetchResourceStats(ctx context.Context, sql SQLConfig, t Target, lastSeen time.Time) ([]DBResourceStat, []RawReading, error) {
	s.lastSeen = lastSeen
	return s.stats, s.readings, s.err
}

type stubQueryStore struct {
	queries []QueryStoreQuery
	stats   []QueryStoreRuntimeStat
	err     error
}

func (s *stubQueryStore) FetchRuntimeStats(ctx context.Context, sql SQLConfig, t Target) ([]QueryStoreQuery, []QueryStoreRuntimeStat, error) {
	return s.queries, s.stats, s.err
}

// ---------- helpers ----------

func testTarget() Target {
	return Target{
		ResourceUID:     uuid.New(),
		CloudAccountID:  uuid.New(),
		AzureResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Sql/servers/s/databases/d",
		RegionCode:      "eastus2",
		ServerName:      "s",
		DatabaseName:    "d",
	}
}

func testService(store *memStore, monitor MetricsProvider, diag DiagnosticsProvider, qstore QueryStoreProvider, sqlUser string) *Service {
	targets := &stubTargets{targets: []Target{testTarget()}}
	// A typed nil *memStore would become a non-nil interface; convert
	// explicitly so "no storage" keeps the nil-contract semantics.
	var writer data.DataWriter
	var runs data.RunReader
	if store != nil {
		writer = store
		runs = store
	}
	return NewService(targets, writer, runs, monitor, diag, qstore, nil, nil, nil, Config{
		MonitorDelay: 90 * time.Second, MonitorLookback: 5 * time.Minute,
		SQLUser: sqlUser, SQLPassword: "pw", FetchParallel: 2,
	})
}

func validReading(code string, v float64) RawReading {
	return RawReading{MetricCode: code, Unit: UnitForCode(code), Timestamp: time.Now().UTC(), IntervalSeconds: 60, Average: ptrF(v), Source: SourceAzureMonitor}
}

// ---------- tests ----------

func TestRunCycleHappyPath(t *testing.T) {
	store := newMemStore()
	monitor := &stubMonitor{readings: []RawReading{
		validReading(CodeCPUPercent, 42),
		validReading(CodeCPUPercent, 200), // out of range -> dropped
	}}
	diag := &stubDiag{
		stats:    []DBResourceStat{{EndTimeUTC: time.Now().UTC().Add(-15 * time.Second)}},
		readings: []RawReading{validReading(CodeSessionsCount, 7)},
	}
	qstore := &stubQueryStore{
		queries: []QueryStoreQuery{{QueryID: 1}},
		stats:   []QueryStoreRuntimeStat{{QueryID: 1}},
	}
	svc := testService(store, monitor, diag, qstore, "workload")

	outcomes, err := svc.RunCycle(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	if len(outcomes) != 3 {
		t.Fatalf("outcomes = %d, want 3 (monitor, diag, qstore)", len(outcomes))
	}
	for _, out := range outcomes {
		if out.Status != "ok" {
			t.Errorf("source %s status = %q, want ok (detail=%s)", out.Source, out.Status, out.Detail)
		}
		if store.finishStatus[out.RunID] != "ok" {
			t.Errorf("source %s finish status = %q, want ok", out.Source, store.finishStatus[out.RunID])
		}
	}
	// The out-of-range monitor reading was dropped; the sessions reading kept,
	// already mapped into the data.Metric write contract.
	if len(store.metrics) != 2 {
		t.Errorf("picked metrics = %d, want 2 (1 valid monitor + 1 sessions)", len(store.metrics))
	}
	for _, m := range store.metrics {
		if m.MetricCode != CodeCPUPercent && m.MetricCode != CodeSessionsCount {
			t.Errorf("unexpected metric code %q", m.MetricCode)
		}
	}
	if len(store.stats) != 1 || len(store.qsQueries) != 1 || len(store.qsStats) != 1 {
		t.Errorf("stats=%d queries=%d qsStats=%d, want 1/1/1", len(store.stats), len(store.qsQueries), len(store.qsStats))
	}

	// Second cycle: the DMV cursor must have advanced past zero.
	if _, err := svc.RunCycle(context.Background(), nil, nil); err != nil {
		t.Fatalf("second RunCycle: %v", err)
	}
	if diag.lastSeen.IsZero() {
		t.Errorf("dmv cursor not advanced after first cycle")
	}
}

func TestRunCycleStorageUnavailable(t *testing.T) {
	svc := testService(nil, &stubMonitor{}, nil, nil, "")
	_, err := svc.RunCycle(context.Background(), nil, nil)
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Errorf("err = %v, want ErrStorageUnavailable", err)
	}
}

func TestRunCycleResourceFilterNotFound(t *testing.T) {
	svc := testService(newMemStore(), &stubMonitor{}, nil, nil, "")
	other := uuid.New()
	_, err := svc.RunCycle(context.Background(), nil, &other)
	if !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("err = %v, want ErrResourceNotFound", err)
	}
}

func TestRunCycleMonitorOnlyWithoutSQLCreds(t *testing.T) {
	store := newMemStore()
	diag := &stubDiag{}
	qstore := &stubQueryStore{}
	svc := testService(store, &stubMonitor{readings: []RawReading{validReading(CodeCPUPercent, 10)}}, diag, qstore, "")
	outcomes, err := svc.RunCycle(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Source != SourceAzureMonitor {
		t.Fatalf("outcomes = %+v, want only the monitor source", outcomes)
	}
}

func TestRunCycleSourceFailureDoesNotAbortOthers(t *testing.T) {
	store := newMemStore()
	monitor := &stubMonitor{err: errors.New("azure exploded")}
	diag := &stubDiag{readings: []RawReading{validReading(CodeSessionsCount, 3)}}
	svc := testService(store, monitor, diag, nil, "workload")
	outcomes, err := svc.RunCycle(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("RunCycle must not fail when one source fails: %v", err)
	}
	var monitorOut, diagOut *RunOutcome
	for i := range outcomes {
		switch outcomes[i].Source {
		case SourceAzureMonitor:
			monitorOut = &outcomes[i]
		case SourceSQLDiag:
			diagOut = &outcomes[i]
		}
	}
	if monitorOut == nil || monitorOut.Status != "error" {
		t.Errorf("monitor outcome = %+v, want status error", monitorOut)
	}
	if diagOut == nil || diagOut.Status != "ok" {
		t.Errorf("diag outcome = %+v, want status ok", diagOut)
	}
	if store.finishStatus[monitorOut.RunID] != "error" {
		t.Errorf("finish status for failed monitor = %q, want error", store.finishStatus[monitorOut.RunID])
	}
}

func TestListRunsAndGetRunOrgScope(t *testing.T) {
	svc := testService(newMemStore(), &stubMonitor{}, nil, nil, "")
	if _, err := svc.RunCycle(context.Background(), nil, nil); err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	org := uuid.New()
	runs, err := svc.ListRuns(context.Background(), org, RunFilter{})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatalf("ListRuns returned no runs for the org's monitored targets")
	}
	if _, err := svc.GetRun(context.Background(), org, runs[0].CollectorRunID); err != nil {
		t.Errorf("GetRun: %v", err)
	}
	if _, err := svc.GetRun(context.Background(), org, 9999); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("GetRun(9999) = %v, want ErrRunNotFound", err)
	}
}

package acquisition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
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

type memRepo struct {
	nextRunID    int64
	runs         []AcquisitionRunDTO
	resources    map[string]int64
	readings     []RawReading
	stats        []DBResourceStat
	qsQueries    []QueryStoreQuery
	qsStats      []QueryStoreRuntimeStat
	finishStatus map[int64]string
	finishRows   map[int64]int
}

func newMemRepo() *memRepo {
	return &memRepo{
		resources:    map[string]int64{},
		finishStatus: map[int64]string{},
		finishRows:   map[int64]int{},
	}
}

func (m *memRepo) EnsureResource(ctx context.Context, t Target) (int64, error) {
	if id, ok := m.resources[t.AzureResourceID]; ok {
		return id, nil
	}
	id := int64(len(m.resources) + 1)
	m.resources[t.AzureResourceID] = id
	return id, nil
}

func (m *memRepo) StartRun(ctx context.Context, source string, resourceID int64, scope string) (int64, error) {
	m.nextRunID++
	m.runs = append(m.runs, AcquisitionRunDTO{
		CollectorRunID: m.nextRunID, Source: source, Scope: scope,
		StartedAtUTC: time.Now().UTC().UnixMilli(), Status: "partial",
	})
	return m.nextRunID, nil
}

func (m *memRepo) FinishRun(ctx context.Context, runID int64, status string, rowsIngested int, errMsg string) error {
	m.finishStatus[runID] = status
	m.finishRows[runID] = rowsIngested
	return nil
}

func (m *memRepo) InsertMetricReadings(ctx context.Context, resourceID int64, readings []RawReading) (int, error) {
	m.readings = append(m.readings, readings...)
	return len(readings), nil
}

func (m *memRepo) InsertResourceStats(ctx context.Context, resourceID int64, stats []DBResourceStat) (int, error) {
	m.stats = append(m.stats, stats...)
	return len(stats), nil
}

func (m *memRepo) InsertQueryStore(ctx context.Context, resourceID int64, queries []QueryStoreQuery, stats []QueryStoreRuntimeStat) (int, error) {
	m.qsQueries = append(m.qsQueries, queries...)
	m.qsStats = append(m.qsStats, stats...)
	return len(queries) + len(stats), nil
}

func (m *memRepo) ListRuns(ctx context.Context, uids []uuid.UUID, filter RunFilter) ([]AcquisitionRunDTO, error) {
	out := []AcquisitionRunDTO{}
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

func (m *memRepo) GetRun(ctx context.Context, id int64) (AcquisitionRunDTO, error) {
	for _, r := range m.runs {
		if r.CollectorRunID == id {
			return r, nil
		}
	}
	return AcquisitionRunDTO{}, ErrRunNotFound
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

func testService(repo AcquisitionRepository, monitor MetricsProvider, diag DiagnosticsProvider, qstore QueryStoreProvider, sqlUser string) *Service {
	targets := &stubTargets{targets: []Target{testTarget()}}
	return NewService(targets, repo, monitor, diag, qstore, Config{
		MonitorDelay: 90 * time.Second, MonitorLookback: 5 * time.Minute,
		SQLUser: sqlUser, SQLPassword: "pw", FetchParallel: 2,
	})
}

func validReading(code string, v float64) RawReading {
	return RawReading{MetricCode: code, Unit: UnitForCode(code), Timestamp: time.Now().UTC(), IntervalSeconds: 60, Average: ptrF(v), Source: SourceAzureMonitor}
}

// ---------- tests ----------

func TestRunCycleHappyPath(t *testing.T) {
	repo := newMemRepo()
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
	svc := testService(repo, monitor, diag, qstore, "workload")

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
		if repo.finishStatus[out.RunID] != "ok" {
			t.Errorf("source %s finish status = %q, want ok", out.Source, repo.finishStatus[out.RunID])
		}
	}
	// The out-of-range monitor reading was dropped; the sessions reading kept.
	if len(repo.readings) != 2 {
		t.Errorf("picked readings = %d, want 2 (1 valid monitor + 1 sessions)", len(repo.readings))
	}
	if len(repo.stats) != 1 || len(repo.qsQueries) != 1 || len(repo.qsStats) != 1 {
		t.Errorf("stats=%d queries=%d qsStats=%d, want 1/1/1", len(repo.stats), len(repo.qsQueries), len(repo.qsStats))
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
	svc := testService(newMemRepo(), &stubMonitor{}, nil, nil, "")
	other := uuid.New()
	_, err := svc.RunCycle(context.Background(), nil, &other)
	if !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("err = %v, want ErrResourceNotFound", err)
	}
}

func TestRunCycleMonitorOnlyWithoutSQLCreds(t *testing.T) {
	repo := newMemRepo()
	diag := &stubDiag{}
	qstore := &stubQueryStore{}
	svc := testService(repo, &stubMonitor{readings: []RawReading{validReading(CodeCPUPercent, 10)}}, diag, qstore, "")
	outcomes, err := svc.RunCycle(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Source != SourceAzureMonitor {
		t.Fatalf("outcomes = %+v, want only the monitor source", outcomes)
	}
}

func TestRunCycleSourceFailureDoesNotAbortOthers(t *testing.T) {
	repo := newMemRepo()
	monitor := &stubMonitor{err: errors.New("azure exploded")}
	diag := &stubDiag{readings: []RawReading{validReading(CodeSessionsCount, 3)}}
	svc := testService(repo, monitor, diag, nil, "workload")
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
	if repo.finishStatus[monitorOut.RunID] != "error" {
		t.Errorf("finish status for failed monitor = %q, want error", repo.finishStatus[monitorOut.RunID])
	}
}

func TestListRunsAndGetRunOrgScope(t *testing.T) {
	repo := newMemRepo()
	svc := testService(repo, &stubMonitor{}, nil, nil, "")
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

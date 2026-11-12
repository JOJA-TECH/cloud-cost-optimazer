package acquisition

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/google/uuid"
)

// Config holds the service runtime knobs, resolved from environment by the
// caller (cmd/cco-app).
type Config struct {
	MonitorDelay    time.Duration // window end = now - delay (metrics publish 1-3 min late)
	MonitorLookback time.Duration // window start = end - lookback (idempotency dedups overlaps)
	SQLUser         string        // workload login for DMVs/Query Store; empty disables both
	SQLPassword     string
	FetchParallel   int // concurrent target fetches; default 8
}

// RunOutcome reports one source acquisition for one target within a cycle.
type RunOutcome struct {
	RunID        int64     `json:"run_id"`
	ResourceUID  uuid.UUID `json:"resource_uid"`
	Source       string    `json:"source"`
	Status       string    `json:"status"` // ok | partial | error
	RowsIngested int       `json:"rows_ingested"`
	Detail       string    `json:"detail,omitempty"`
}

// Service implements the acquisition cycle: resolve targets, open
// collector_run, fetch, normalize/validate, persist idempotently, close run.
type Service struct {
	targets TargetRepository
	repo    AcquisitionRepository // nil when cco_data is not configured
	monitor MetricsProvider
	diag    DiagnosticsProvider
	qstore  QueryStoreProvider

	cfg Config

	// cycleMu guarantees one cycle at a time (scheduler tick or manual
	// trigger); overlapping requests get ErrCycleInProgress.
	cycleMu sync.Mutex
	// dmvLastSeen is the incremental cursor for sys.dm_db_resource_stats per
	// resource; only advanced after a successful persist. Safe because cycles
	// are serialized.
	dmvLastSeen map[uuid.UUID]time.Time
}

// NewService constructs the Service. repo may be nil when cco_data is not
// configured: every run then fails with ErrStorageUnavailable.
func NewService(targets TargetRepository, repo AcquisitionRepository, monitor MetricsProvider, diag DiagnosticsProvider, qstore QueryStoreProvider, cfg Config) *Service {
	if cfg.FetchParallel <= 0 {
		cfg.FetchParallel = 8
	}
	return &Service{
		targets:     targets,
		repo:        repo,
		monitor:     monitor,
		diag:        diag,
		qstore:      qstore,
		cfg:         cfg,
		dmvLastSeen: map[uuid.UUID]time.Time{},
	}
}

// monitorWindow returns the Azure Monitor query window.
func (s *Service) monitorWindow() (start, end time.Time) {
	end = time.Now().UTC().Add(-s.cfg.MonitorDelay)
	start = end.Add(-s.cfg.MonitorLookback)
	return start, end
}

// sqlConfig builds the direct-SQL config for a target from the workload env
// credentials. ok=false when disabled.
func (s *Service) sqlConfig(t Target) (SQLConfig, bool) {
	if s.cfg.SQLUser == "" || s.cfg.SQLPassword == "" {
		return SQLConfig{}, false
	}
	server := t.ServerName
	if server != "" && !strings.Contains(server, ".") {
		server += ".database.windows.net" // ARM reports the bare server name
	}
	return SQLConfig{
		Server:   server,
		Database: t.DatabaseName,
		User:     s.cfg.SQLUser,
		Password: s.cfg.SQLPassword,
	}, true
}

// RunCycle executes one acquisition cycle for the monitored targets of orgID
// (nil = all orgs, system scheduler), optionally restricted to one resource.
// Each (target, source) pair produces one raw.collector_run row; a failing
// source never aborts the others or the cycle itself.
func (s *Service) RunCycle(ctx context.Context, orgID *uuid.UUID, resourceUID *uuid.UUID) ([]RunOutcome, error) {
	if s.repo == nil {
		return nil, ErrStorageUnavailable
	}
	if !s.cycleMu.TryLock() {
		return nil, ErrCycleInProgress
	}
	defer s.cycleMu.Unlock()

	targets, err := s.targets.ListMonitoredTargets(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if resourceUID != nil {
		filtered := make([]Target, 0, 1)
		for _, t := range targets {
			if t.ResourceUID == *resourceUID {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			return nil, ErrResourceNotFound
		}
		targets = filtered
	}

	start, end := s.monitorWindow()
	credsByAccount := map[uuid.UUID]cloud.ConnectionCredentials{}

	var (
		wg       sync.WaitGroup
		outMu    sync.Mutex
		outcomes []RunOutcome
		sem      = make(chan struct{}, s.cfg.FetchParallel)
	)
	for _, t := range targets {
		target := t
		for _, job := range s.cycleJobs(target) {
			job := job
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				out := job(ctx, target, start, end, credsByAccount)
				outMu.Lock()
				outcomes = append(outcomes, out)
				outMu.Unlock()
			}()
		}
	}
	wg.Wait()

	slog.Info("acquisition cycle finished",
		"targets", len(targets), "runs", len(outcomes),
	)
	return outcomes, nil
}

// cycleJobs returns the acquisition jobs enabled for a target (monitor always
// when a provider is wired; DMVs and Query Store when both a provider and the
// workload SQL credentials are configured).
func (s *Service) cycleJobs(t Target) []func(ctx context.Context, t Target, start, end time.Time, creds map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	jobs := []func(ctx context.Context, t Target, start, end time.Time, creds map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome{}
	if s.monitor != nil {
		jobs = append(jobs, s.runMonitorJob)
	}
	if _, ok := s.sqlConfig(t); ok {
		if s.diag != nil {
			jobs = append(jobs, s.runDiagnosticsJob)
		}
		if s.qstore != nil {
			jobs = append(jobs, s.runQueryStoreJob)
		}
	}
	return jobs
}

// credentialsFor resolves (and caches per cycle) the ARM credentials of the
// account that owns a target.
func (s *Service) credentialsFor(ctx context.Context, t Target, cache map[uuid.UUID]cloud.ConnectionCredentials) (cloud.ConnectionCredentials, error) {
	if creds, ok := cache[t.CloudAccountID]; ok {
		return creds, nil
	}
	creds, err := s.targets.ResolveCredentials(ctx, t.CloudAccountID)
	if err != nil {
		return cloud.ConnectionCredentials{}, err
	}
	cache[t.CloudAccountID] = creds
	return creds, nil
}

// runMonitorJob acquires Azure Monitor platform metrics for one target.
func (s *Service) runMonitorJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: SourceAzureMonitor, Status: "error"}
	creds, err := s.credentialsFor(ctx, t, credsCache)
	if err != nil {
		out.Detail = "resolve credentials: " + err.Error()
		return out
	}
	resourceID, err := s.repo.EnsureResource(ctx, t)
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	runID, err := s.repo.StartRun(ctx, SourceAzureMonitor, resourceID, t.SubscriptionID)
	if err != nil {
		out.Detail = "start run: " + err.Error()
		return out
	}
	out.RunID = runID

	readings, fetchErr := s.monitor.FetchPlatformMetrics(ctx, creds, t.AzureResourceID, start, end)
	rows := 0
	if fetchErr == nil {
		for i := range readings {
			ValidateReading(&readings[i])
		}
		var dropped int
		readings, dropped = DropInvalid(readings)
		if dropped > 0 {
			slog.Warn("acquisition dropped out-of-range readings", "resource_uid", t.ResourceUID, "source", SourceAzureMonitor, "dropped", dropped)
		}
		rows, err = s.repo.InsertMetricReadings(ctx, resourceID, readings)
		if err != nil {
			fetchErr = err
		}
	}
	out.RowsIngested = rows
	if fetchErr != nil {
		out.Detail = fetchErr.Error()
		out.Status = "error"
		_ = s.repo.FinishRun(ctx, runID, "error", rows, trimErr(fetchErr))
		return out
	}
	out.Status = "ok"
	_ = s.repo.FinishRun(ctx, runID, "ok", rows, "")
	return out
}

// runDiagnosticsJob acquires sys.dm_db_resource_stats + session snapshot for
// one target. The incremental cursor only advances on a successful persist.
func (s *Service) runDiagnosticsJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: SourceSQLDiag, Status: "error"}
	sql, _ := s.sqlConfig(t)
	resourceID, err := s.repo.EnsureResource(ctx, t)
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	runID, err := s.repo.StartRun(ctx, SourceSQLDiag, resourceID, t.SubscriptionID)
	if err != nil {
		out.Detail = "start run: " + err.Error()
		return out
	}
	out.RunID = runID

	stats, readings, fetchErr := s.diag.FetchResourceStats(ctx, sql, t, s.dmvLastSeen[t.ResourceUID])
	rows := 0
	if fetchErr == nil {
		for i := range readings {
			ValidateReading(&readings[i])
		}
		readings, _ = DropInvalid(readings)
		n1, err := s.repo.InsertResourceStats(ctx, resourceID, stats)
		if err != nil {
			fetchErr = err
		} else {
			n2, err := s.repo.InsertMetricReadings(ctx, resourceID, readings)
			rows = n1 + n2
			if err != nil {
				fetchErr = err
			} else {
				// Advance the cursor to the newest stat row read.
				for _, st := range stats {
					if st.EndTimeUTC.After(s.dmvLastSeen[t.ResourceUID]) {
						s.dmvLastSeen[t.ResourceUID] = st.EndTimeUTC
					}
				}
			}
		}
	}
	out.RowsIngested = rows
	if fetchErr != nil {
		out.Detail = fetchErr.Error()
		_ = s.repo.FinishRun(ctx, runID, "error", rows, trimErr(fetchErr))
		return out
	}
	out.Status = "ok"
	_ = s.repo.FinishRun(ctx, runID, "ok", rows, "")
	return out
}

// runQueryStoreJob acquires the top Query Store runtime stats for one target.
func (s *Service) runQueryStoreJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: SourceQueryStore, Status: "error"}
	sql, _ := s.sqlConfig(t)
	resourceID, err := s.repo.EnsureResource(ctx, t)
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	runID, err := s.repo.StartRun(ctx, SourceQueryStore, resourceID, t.SubscriptionID)
	if err != nil {
		out.Detail = "start run: " + err.Error()
		return out
	}
	out.RunID = runID

	queries, stats, fetchErr := s.qstore.FetchRuntimeStats(ctx, sql, t)
	rows := 0
	if fetchErr == nil {
		rows, err = s.repo.InsertQueryStore(ctx, resourceID, queries, stats)
		if err != nil {
			fetchErr = err
		}
	}
	out.RowsIngested = rows
	if fetchErr != nil {
		out.Detail = fetchErr.Error()
		_ = s.repo.FinishRun(ctx, runID, "error", rows, trimErr(fetchErr))
		return out
	}
	out.Status = "ok"
	_ = s.repo.FinishRun(ctx, runID, "ok", rows, "")
	return out
}

// ListRuns returns collector runs scoped to the organization's monitored
// resources. Org scoping happens here (service layer) as defense in depth.
func (s *Service) ListRuns(ctx context.Context, orgID uuid.UUID, filter RunFilter) ([]AcquisitionRunDTO, error) {
	if s.repo == nil {
		return nil, ErrStorageUnavailable
	}
	targets, err := s.targets.ListMonitoredTargets(ctx, &orgID)
	if err != nil {
		return nil, err
	}
	uids := make([]uuid.UUID, 0, len(targets))
	for _, t := range targets {
		uids = append(uids, t.ResourceUID)
	}
	return s.repo.ListRuns(ctx, uids, filter)
}

// GetRun returns one collector run, 404 semantics when it belongs to another
// organization (or does not exist).
func (s *Service) GetRun(ctx context.Context, orgID uuid.UUID, runID int64) (AcquisitionRunDTO, error) {
	if s.repo == nil {
		return AcquisitionRunDTO{}, ErrStorageUnavailable
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return AcquisitionRunDTO{}, err
	}
	if run.ResourceUID == nil {
		return run, nil // scope-only run without resource; not org-restricted
	}
	targets, err := s.targets.ListMonitoredTargets(ctx, &orgID)
	if err != nil {
		return AcquisitionRunDTO{}, err
	}
	for _, t := range targets {
		if t.ResourceUID == *run.ResourceUID {
			return run, nil
		}
	}
	return AcquisitionRunDTO{}, ErrRunNotFound
}

// trimErr shortens provider errors for storage in raw.collector_run.
func trimErr(err error) string {
	msg := err.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}

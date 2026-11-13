package acquisition

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	"github.com/google/uuid"
)

// Config holds the service runtime knobs, resolved from environment by the
// caller (cmd/cco-app).
type Config struct {
	MonitorDelay     time.Duration // window end = now - delay (metrics publish 1-3 min late)
	MonitorLookback  time.Duration // window start = end - lookback (idempotency dedups overlaps)
	CostLookbackDays time.Duration // cost window lookback (days); Cost Management has 8-24h lag
	SQLUser          string        // workload login for DMVs/Query Store; empty disables both
	SQLPassword      string
	FetchParallel    int           // concurrent target fetches; default 8
	FetchTimeout     time.Duration // per-provider call timeout; default 30s
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
// Persistence goes through the Gestión de Datos contracts (data.DataWriter /
// data.RunReader) per MODULES.md — this module never touches cco_data itself.
type Service struct {
	targets TargetRepository
	writer  data.DataWriter // nil when cco_data is not configured
	runs    data.RunReader  // nil when cco_data is not configured
	monitor MetricsProvider
	diag    DiagnosticsProvider
	qstore  QueryStoreProvider
	config  ConfigProvider // T4: ARM config snapshots
	cost    CostProvider   // T5: Cost Management daily costs
	sample  SampleProvider // T1b: individual observations for exact percentiles

	cfg Config

	// cycleMu guarantees one cycle at a time (scheduler tick or manual
	// trigger); overlapping requests get ErrCycleInProgress.
	cycleMu sync.Mutex
	// dmvLastSeen is the incremental cursor for sys.dm_db_resource_stats per
	// resource; only advanced after a successful persist. Safe because cycles
	// are serialized.
	dmvLastSeen map[uuid.UUID]time.Time
}

// NewService constructs the Service. writer/runs may be nil when cco_data is
// not configured: every run then fails with ErrStorageUnavailable.
func NewService(targets TargetRepository, writer data.DataWriter, runs data.RunReader, monitor MetricsProvider, diag DiagnosticsProvider, qstore QueryStoreProvider, config ConfigProvider, cost CostProvider, sample SampleProvider, cfg Config) *Service {
	if cfg.FetchParallel <= 0 {
		cfg.FetchParallel = 8
	}
	if cfg.FetchTimeout <= 0 {
		cfg.FetchTimeout = 30 * time.Second
	}
	return &Service{
		targets:     targets,
		writer:      writer,
		runs:        runs,
		monitor:     monitor,
		diag:        diag,
		qstore:      qstore,
		config:      config,
		cost:        cost,
		sample:      sample,
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
	if s.writer == nil {
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
// workload SQL credentials are configured; config when ConfigProvider is wired).
func (s *Service) cycleJobs(t Target) []func(ctx context.Context, t Target, start, end time.Time, creds map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	jobs := []func(ctx context.Context, t Target, start, end time.Time, creds map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome{}
	if s.monitor != nil {
		jobs = append(jobs, s.runMonitorJob)
	}
	if s.config != nil {
		jobs = append(jobs, s.runConfigJob)
	}
	if s.sample != nil {
		jobs = append(jobs, s.runSampleJob)
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

// diagFetchResult bundles the two values the diagnostics fetch returns so the
// generic runJob helper (fetch -> single write) can carry them together.
type diagFetchResult struct {
	stats    []DBResourceStat
	readings []RawReading
}

// qsFetchResult bundles the two values the Query Store fetch returns.
type qsFetchResult struct {
	queries []QueryStoreQuery
	stats   []QueryStoreRuntimeStat
}

// runJob runs the fetch -> write -> FinishRun boilerplate for one acquisition
// job. The caller resolves credentials and ensures the resource first; runJob
// owns StartRun, the per-fetch timeout, the write, row counting, and FinishRun.
// The fetch closure returns the raw rows; the write closure persists them
// (receiving the run id it may need) and returns the row count. Both closures
// capture the credentials, target and window they need.
func (s *Service) runJob(ctx context.Context, source string, resourceID int64, scope string,
	fetch func(context.Context) (any, error),
	write func(any, int64) (int, error),
) RunOutcome {
	out := RunOutcome{Source: source, Status: "error"}
	runID, err := s.writer.StartRun(ctx, source, resourceID, scope)
	if err != nil {
		out.Detail = "start run: " + err.Error()
		return out
	}
	out.RunID = runID

	fetchCtx, cancel := context.WithTimeout(ctx, s.cfg.FetchTimeout)
	defer cancel()
	fetched, fetchErr := fetch(fetchCtx)
	rows := 0
	if fetchErr == nil {
		n, err := write(fetched, runID)
		if err != nil {
			fetchErr = err
		} else {
			rows = n
		}
	}
	out.RowsIngested = rows
	if fetchErr != nil {
		out.Detail = fetchErr.Error()
		_ = s.writer.FinishRun(ctx, runID, "error", rows, trimErr(fetchErr))
		return out
	}
	out.Status = "ok"
	_ = s.writer.FinishRun(ctx, runID, "ok", rows, "")
	return out
}

// runMonitorJob acquires Azure Monitor platform metrics for one target.
func (s *Service) runMonitorJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: SourceAzureMonitor, Status: "error"}
	creds, err := s.credentialsFor(ctx, t, credsCache)
	if err != nil {
		out.Detail = "resolve credentials: " + err.Error()
		return out
	}
	resourceID, err := s.writer.EnsureResource(ctx, t.resourceRef())
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	out = s.runJob(ctx, SourceAzureMonitor, resourceID, t.SubscriptionID,
		func(fetchCtx context.Context) (any, error) {
			return s.monitor.FetchPlatformMetrics(fetchCtx, creds, t.AzureResourceID, start, end)
		},
		func(rows any, _ int64) (int, error) {
			readings := rows.([]RawReading)
			for i := range readings {
				ValidateReading(&readings[i])
			}
			var dropped int
			readings, dropped = DropInvalid(readings)
			if dropped > 0 {
				slog.Warn("acquisition dropped out-of-range readings", "resource_uid", t.ResourceUID, "source", SourceAzureMonitor, "dropped", dropped)
			}
			return s.writer.WriteMetricReadings(ctx, resourceID, metricsFromReadings(readings))
		},
	)
	out.ResourceUID = t.ResourceUID
	return out
}

// runDiagnosticsJob acquires sys.dm_db_resource_stats + session snapshot for
// one target. The incremental cursor only advances on a successful persist.
func (s *Service) runDiagnosticsJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: SourceSQLDiag, Status: "error"}
	sql, _ := s.sqlConfig(t)
	resourceID, err := s.writer.EnsureResource(ctx, t.resourceRef())
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	out = s.runJob(ctx, SourceSQLDiag, resourceID, t.SubscriptionID,
		func(fetchCtx context.Context) (any, error) {
			stats, readings, err := s.diag.FetchResourceStats(fetchCtx, sql, t, s.dmvLastSeen[t.ResourceUID])
			return diagFetchResult{stats: stats, readings: readings}, err
		},
		func(fetched any, _ int64) (int, error) {
			r := fetched.(diagFetchResult)
			for i := range r.readings {
				ValidateReading(&r.readings[i])
			}
			r.readings, _ = DropInvalid(r.readings)
			n1, err := s.writer.WriteResourceStats(ctx, resourceID, resourceStatsFrom(r.stats))
			if err != nil {
				return 0, err
			}
			n2, err := s.writer.WriteMetricReadings(ctx, resourceID, metricsFromReadings(r.readings))
			if err != nil {
				return n1, err
			}
			// Advance the cursor to the newest stat row read.
			for _, st := range r.stats {
				if st.EndTimeUTC.After(s.dmvLastSeen[t.ResourceUID]) {
					s.dmvLastSeen[t.ResourceUID] = st.EndTimeUTC
				}
			}
			return n1 + n2, nil
		},
	)
	out.ResourceUID = t.ResourceUID
	return out
}

// runQueryStoreJob acquires the top Query Store runtime stats for one target.
func (s *Service) runQueryStoreJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: SourceQueryStore, Status: "error"}
	sql, _ := s.sqlConfig(t)
	resourceID, err := s.writer.EnsureResource(ctx, t.resourceRef())
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	out = s.runJob(ctx, SourceQueryStore, resourceID, t.SubscriptionID,
		func(fetchCtx context.Context) (any, error) {
			queries, stats, err := s.qstore.FetchRuntimeStats(fetchCtx, sql, t)
			return qsFetchResult{queries: queries, stats: stats}, err
		},
		func(rows any, _ int64) (int, error) {
			r := rows.(qsFetchResult)
			qs, rs := queryStoreRecordsFrom(r.queries, r.stats)
			return s.writer.WriteQueryStore(ctx, resourceID, qs, rs)
		},
	)
	out.ResourceUID = t.ResourceUID
	return out
}

// ListRuns returns collector runs scoped to the organization's monitored
// resources. Org scoping happens here (service layer) as defense in depth.
func (s *Service) ListRuns(ctx context.Context, orgID uuid.UUID, filter RunFilter) ([]AcquisitionRunDTO, error) {
	if s.runs == nil {
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
	records, err := s.runs.ListRuns(ctx, uids, data.RunFilter{
		ResourceUID: filter.ResourceUID,
		Source:      filter.Source,
		Status:      filter.Status,
		Limit:       filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	return runDTOsFrom(records), nil
}

// GetRun returns one collector run, 404 semantics when it belongs to another
// organization (or does not exist).
func (s *Service) GetRun(ctx context.Context, orgID uuid.UUID, runID int64) (AcquisitionRunDTO, error) {
	if s.runs == nil {
		return AcquisitionRunDTO{}, ErrStorageUnavailable
	}
	record, err := s.runs.GetRun(ctx, runID)
	if err != nil {
		// Translate the storage not-found into this module's domain error so
		// the API layer keeps its acquisition error contract.
		if errors.Is(err, data.ErrNotFound) {
			return AcquisitionRunDTO{}, ErrRunNotFound
		}
		return AcquisitionRunDTO{}, err
	}
	if record.ResourceUID == nil {
		return runDTOFrom(record), nil // scope-only run without resource; not org-restricted
	}
	targets, err := s.targets.ListMonitoredTargets(ctx, &orgID)
	if err != nil {
		return AcquisitionRunDTO{}, err
	}
	for _, t := range targets {
		if t.ResourceUID == *record.ResourceUID {
			return runDTOFrom(record), nil
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

// runConfigJob fetches the ARM configuration snapshot for one target and
// upserts it into core.resource_config_snapshot (deduped by observed_at_utc).
func (s *Service) runConfigJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: "arm_config", Status: "error"}
	creds, err := s.credentialsFor(ctx, t, credsCache)
	if err != nil {
		out.Detail = "resolve credentials: " + err.Error()
		return out
	}
	resourceID, err := s.writer.EnsureResource(ctx, t.resourceRef())
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	out = s.runJob(ctx, "arm_config", resourceID, t.SubscriptionID,
		func(fetchCtx context.Context) (any, error) {
			return s.config.FetchConfigSnapshot(fetchCtx, creds, t.AzureResourceID)
		},
		func(rows any, runID int64) (int, error) {
			reading := rows.(ConfigReading)
			snap := data.ConfigurationSnapshot{
				ResourceUID:       t.ResourceUID,
				CollectorRunID:    &runID,
				SKUName:           reading.SKUName,
				Tier:              reading.Tier,
				Family:            reading.Family,
				CapacityVCores:    reading.CapacityVCores,
				MinCapacityVCores: reading.MinCapacityVCores,
				ComputeModel:      reading.ComputeModel,
				MaxSizeBytes:      reading.MaxSizeBytes,
				AutoPauseDelayMin: reading.AutoPauseDelayMin,
				Status:            reading.Status,
				FreeLimit:         reading.FreeLimit,
				SourcePayload:     reading.SourcePayload,
				ObservedAtUTC:     reading.ObservedAtUTC,
			}
			rows64, err := s.writer.UpsertConfigSnapshot(ctx, snap)
			return int(rows64), err
		},
	)
	out.ResourceUID = t.ResourceUID
	return out
}

// RunCostCycle executes the cost collection for all monitored targets grouped
// by subscription. It is invoked by a separate scheduler (T5: Cost Management).
// Returns one RunOutcome per subscription. Errors are recorded per subscription
// but do not fail the whole call.
func (s *Service) RunCostCycle(ctx context.Context, orgID *uuid.UUID) ([]RunOutcome, error) {
	if s.writer == nil {
		return nil, ErrStorageUnavailable
	}
	if s.cost == nil {
		return nil, errors.New("cost provider not configured")
	}

	targets, err := s.targets.ListMonitoredTargets(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return []RunOutcome{}, nil
	}

	// Group targets by subscription (cost query is per subscription).
	subMap := make(map[string][]Target)
	for _, t := range targets {
		subMap[t.SubscriptionID] = append(subMap[t.SubscriptionID], t)
	}

	var (
		wg       sync.WaitGroup
		outMu    sync.Mutex
		outcomes []RunOutcome
	)
	from := time.Now().UTC().Add(-s.cfg.MonitorLookback).Truncate(24 * time.Hour)
	to := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)

	for subID, subTargets := range subMap {
		subID := subID
		subTargets := subTargets
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := RunOutcome{ResourceUID: subTargets[0].ResourceUID, Source: "cost_management", Status: "error"}

			creds, err := s.targets.ResolveCredentials(ctx, subTargets[0].CloudAccountID)
			if err != nil {
				out.Detail = "resolve credentials: " + err.Error()
				outMu.Lock()
				outcomes = append(outcomes, out)
				outMu.Unlock()
				return
			}

			resourceID, err := s.writer.EnsureResource(ctx, subTargets[0].resourceRef())
			if err != nil {
				out.Detail = "ensure resource: " + err.Error()
				outMu.Lock()
				outcomes = append(outcomes, out)
				outMu.Unlock()
				return
			}
			out = s.runJob(ctx, "cost_management", resourceID, subID,
				func(fetchCtx context.Context) (any, error) {
					return s.cost.FetchDailyCosts(fetchCtx, creds, subID, from, to)
				},
				func(rows any, _ int64) (int, error) {
					return s.writer.UpsertCostDaily(ctx, rows.([]data.CostRecord))
				},
			)
			out.ResourceUID = subTargets[0].ResourceUID
			outMu.Lock()
			outcomes = append(outcomes, out)
			outMu.Unlock()
		}()
	}
	wg.Wait()

	slog.Info("cost cycle finished", "subscriptions", len(subMap), "runs", len(outcomes))
	return outcomes, nil
}

// runSampleJob acquires individual observations for exact percentiles
// (raw.metric_sample) from sources that expose them (Query Store runtime,
// workload observations).
func (s *Service) runSampleJob(ctx context.Context, t Target, start, end time.Time, credsCache map[uuid.UUID]cloud.ConnectionCredentials) RunOutcome {
	out := RunOutcome{ResourceUID: t.ResourceUID, Source: "metric_sample", Status: "error"}
	creds, err := s.credentialsFor(ctx, t, credsCache)
	if err != nil {
		out.Detail = "resolve credentials: " + err.Error()
		return out
	}
	resourceID, err := s.writer.EnsureResource(ctx, t.resourceRef())
	if err != nil {
		out.Detail = "ensure resource: " + err.Error()
		return out
	}
	out = s.runJob(ctx, "metric_sample", resourceID, t.SubscriptionID,
		func(fetchCtx context.Context) (any, error) {
			return s.sample.FetchSamples(fetchCtx, creds, t.AzureResourceID, start, end)
		},
		func(fetched any, _ int64) (int, error) {
			return s.writer.WriteMetricSamples(ctx, resourceID, samplesFromSamples(fetched.([]RawSample)))
		},
	)
	out.ResourceUID = t.ResourceUID
	return out
}

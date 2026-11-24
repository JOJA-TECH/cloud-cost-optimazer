package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "github.com/microsoft/go-mssqldb" // registers the "sqlserver" driver

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
)

// SQLServerDiagnostics reads SQL diagnostics through a direct TDS connection
// (github.com/microsoft/go-mssqldb). It implements both
// acquisition.DiagnosticsProvider (DMVs) and acquisition.QueryStoreProvider
// (Query Store). Read-only: it runs bounded SELECTs and never mutates the
// monitored database.
type SQLServerDiagnostics struct{}

var (
	_ acquisition.DiagnosticsProvider = (*SQLServerDiagnostics)(nil)
	_ acquisition.QueryStoreProvider  = (*SQLServerDiagnostics)(nil)
)

// NewSQLServerDiagnostics returns a SQLServerDiagnostics.
func NewSQLServerDiagnostics() *SQLServerDiagnostics {
	return &SQLServerDiagnostics{}
}

// dsnFor builds the sqlserver:// DSN for the workload login. net/url escapes
// the userinfo and the query values, so special characters in the password
// (@, :, /, spaces) survive the round trip.
func dsnFor(cfg acquisition.SQLConfig) string {
	u := url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   cfg.Server,
	}
	q := url.Values{}
	q.Set("database", cfg.Database)
	q.Set("encrypt", "true")
	q.Set("app name", "cco-acquisition")
	u.RawQuery = q.Encode()
	return u.String()
}

// resourceStatsQuery is incremental: the single placeholder carries the
// service cursor (zero time on the first run; sys.dm_db_resource_stats only
// keeps ~1 hour of rows, so a zero cursor is safe).
const resourceStatsQuery = `SELECT end_time, avg_cpu_percent, avg_data_io_percent, avg_log_write_percent, avg_memory_usage_percent, avg_instance_cpu_percent, dtu_limit, cpu_limit FROM sys.dm_db_resource_stats WHERE end_time > ? ORDER BY end_time`

const sessionsQuery = `SELECT COUNT(*) FROM sys.dm_exec_sessions WHERE is_user_process = 1 AND status <> 'pre-connect'`

// queryStoreQuery returns the TOP 20 runtime stats most recently executed,
// joined with the query and query-text metadata.
const queryStoreQuery = `SELECT TOP (20) q.query_id, q.query_text_id, q.query_hash, qt.query_sql_text, q.is_internal_query, rs.first_execution_time, rs.last_execution_time, rs.count_executions, rs.avg_duration, rs.avg_cpu_time, rs.avg_logical_io_reads, rs.avg_logical_io_writes FROM sys.query_store_runtime_stats rs JOIN sys.query_store_plan p ON p.plan_id = rs.plan_id JOIN sys.query_store_query q ON q.query_id = p.query_id JOIN sys.query_store_query_text qt ON qt.query_text_id = q.query_text_id ORDER BY rs.last_execution_time DESC`

// The helpers below convert nullable scan targets into pointers (nil when
// NULL), matching the *T fields of the acquisition DTOs.

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func nullIntPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullBoolPtr(v sql.NullBool) *bool {
	if !v.Valid {
		return nil
	}
	return &v.Bool
}

// FetchResourceStats reads the incremental sys.dm_db_resource_stats rows plus
// a current user-sessions reading.
func (d *SQLServerDiagnostics) FetchResourceStats(ctx context.Context, cfg acquisition.SQLConfig, _ acquisition.Target, lastSeen time.Time) ([]acquisition.DBResourceStat, []acquisition.RawReading, error) {
	// shortcut: one connection per call at laboratory scale; upgrade: a pool
	// per server when many databases are monitored concurrently.
	db, err := sql.Open("sqlserver", dsnFor(cfg))
	if err != nil {
		return nil, nil, fmt.Errorf("sql diagnostics: %w", err)
	}
	defer db.Close()

	stats, err := queryResourceStats(ctx, db, lastSeen)
	if err != nil {
		return nil, nil, fmt.Errorf("sql diagnostics: %w", err)
	}
	readings, err := querySessions(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("sql diagnostics: %w", err)
	}
	return stats, readings, nil
}

// FetchRuntimeStats reads the TOP 20 Query Store runtime stats plus the
// distinct query texts they reference.
func (d *SQLServerDiagnostics) FetchRuntimeStats(ctx context.Context, cfg acquisition.SQLConfig, _ acquisition.Target) ([]acquisition.QueryStoreQuery, []acquisition.QueryStoreRuntimeStat, error) {
	db, err := sql.Open("sqlserver", dsnFor(cfg))
	if err != nil {
		return nil, nil, fmt.Errorf("query store: %w", err)
	}
	defer db.Close()

	queries, stats, err := queryRuntimeStats(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("query store: %w", err)
	}
	return queries, stats, nil
}

func queryResourceStats(ctx context.Context, db *sql.DB, lastSeen time.Time) ([]acquisition.DBResourceStat, error) {
	rows, err := db.QueryContext(ctx, resourceStatsQuery, lastSeen.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []acquisition.DBResourceStat
	for rows.Next() {
		var (
			endTime                                            time.Time
			cpu, dataIO, logWrite, mem, instCPU, dtu, cpuLimit sql.NullFloat64
		)
		if err := rows.Scan(&endTime, &cpu, &dataIO, &logWrite, &mem, &instCPU, &dtu, &cpuLimit); err != nil {
			return nil, err
		}
		stats = append(stats, acquisition.DBResourceStat{
			EndTimeUTC:            endTime.UTC(),
			AvgCPUPercent:         nullFloatPtr(cpu),
			AvgDataIOPercent:      nullFloatPtr(dataIO),
			AvgLogWritePercent:    nullFloatPtr(logWrite),
			AvgMemoryUsagePercent: nullFloatPtr(mem),
			AvgInstanceCPUPercent: nullFloatPtr(instCPU),
			DTULimit:              nullFloatPtr(dtu),
			CPULimit:              nullFloatPtr(cpuLimit),
		})
	}
	return stats, rows.Err()
}

// querySessions returns the current user-session count as a 60s point
// reading (not incremental).
func querySessions(ctx context.Context, db *sql.DB) ([]acquisition.RawReading, error) {
	var count int64
	if err := db.QueryRowContext(ctx, sessionsQuery).Scan(&count); err != nil {
		return nil, err
	}
	value := float64(count)
	return []acquisition.RawReading{{
		MetricCode:      acquisition.CodeSessionsCount,
		Unit:            acquisition.UnitForCode(acquisition.CodeSessionsCount),
		Timestamp:       time.Now().UTC().Truncate(time.Second),
		IntervalSeconds: 60,
		Average:         &value,
		Source:          acquisition.SourceSQLDiag,
	}}, nil
}

func queryRuntimeStats(ctx context.Context, db *sql.DB) ([]acquisition.QueryStoreQuery, []acquisition.QueryStoreRuntimeStat, error) {
	rows, err := db.QueryContext(ctx, queryStoreQuery)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	// One snapshot instant shared by every row read in this call.
	snapshotAt := time.Now().UTC().Truncate(time.Second)
	seen := make(map[int64]bool)
	var queries []acquisition.QueryStoreQuery
	var stats []acquisition.QueryStoreRuntimeStat
	for rows.Next() {
		var (
			queryID                       int64
			queryTextID                   sql.NullInt64
			queryHash                     []byte
			querySQLText                  sql.NullString
			isInternal                    sql.NullBool
			firstExecution, lastExecution time.Time
			countExecutions               sql.NullInt64
			avgDuration, avgCPU           sql.NullFloat64
			avgReads, avgWrites           sql.NullFloat64
		)
		if err := rows.Scan(&queryID, &queryTextID, &queryHash, &querySQLText, &isInternal,
			&firstExecution, &lastExecution, &countExecutions, &avgDuration, &avgCPU,
			&avgReads, &avgWrites); err != nil {
			return nil, nil, err
		}

		// avg_duration and avg_cpu_time arrive in microseconds (native Query
		// Store units) and map directly to AvgDurationUS/AvgCPUTimeUS.
		stats = append(stats, acquisition.QueryStoreRuntimeStat{
			QueryID:            queryID,
			IntervalStartUTC:   firstExecution.UTC(),
			IntervalEndUTC:     lastExecution.UTC(),
			CountExecutions:    nullIntPtr(countExecutions),
			AvgDurationUS:      nullFloatPtr(avgDuration),
			AvgCPUTimeUS:       nullFloatPtr(avgCPU),
			AvgLogicalIOReads:  nullFloatPtr(avgReads),
			AvgLogicalIOWrites: nullFloatPtr(avgWrites),
			SnapshotAtUTC:      snapshotAt,
		})

		// Keep one query row per query_id (first appearance); runtime stats
		// keep every interval row.
		if seen[queryID] {
			continue
		}
		seen[queryID] = true
		var hash *string
		if len(queryHash) > 0 {
			h := fmt.Sprintf("%x", queryHash) // binary(8) -> stable hex string
			hash = &h
		}
		queries = append(queries, acquisition.QueryStoreQuery{
			QueryID:         queryID,
			QueryTextID:     nullIntPtr(queryTextID),
			QueryHash:       hash,
			QuerySQLText:    nullStringPtr(querySQLText),
			IsInternalQuery: nullBoolPtr(isInternal),
			SnapshotAtUTC:   snapshotAt,
		})
	}
	return queries, stats, rows.Err()
}

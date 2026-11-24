package adapters

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

// TestDSNFor covers the DSN builder without a real SQL Server: escaping must
// survive a url.Parse round trip.
func TestDSNFor(t *testing.T) {
	cases := []struct {
		name string
		cfg  acquisition.SQLConfig
	}{
		{
			name: "password with special characters",
			cfg: acquisition.SQLConfig{
				Server:   "sql-lab.database.windows.net",
				Database: "sqldb-sales",
				User:     "workload",
				Password: "p@ss:w/ord space",
			},
		},
		{
			name: "server with port",
			cfg: acquisition.SQLConfig{
				Server:   "host.database.windows.net:1433",
				Database: "sqldb-metrics",
				User:     "workload",
				Password: "s3cret-pw",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dsn := dsnFor(c.cfg)
			if strings.ContainsAny(c.cfg.Password, "@:/ ") && strings.Contains(dsn, c.cfg.Password) {
				t.Errorf("password not URL-escaped in %q", dsn)
			}
			if strings.Count(dsn, "@") != 1 {
				t.Errorf("want exactly one raw @ (the userinfo separator) in %q", dsn)
			}

			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", dsn, err)
			}
			if u.Scheme != "sqlserver" {
				t.Errorf("scheme = %q, want sqlserver", u.Scheme)
			}
			if u.Host != c.cfg.Server {
				t.Errorf("host = %q, want %q", u.Host, c.cfg.Server)
			}
			if u.User.Username() != c.cfg.User {
				t.Errorf("user = %q, want %q", u.User.Username(), c.cfg.User)
			}
			if password, ok := u.User.Password(); !ok || password != c.cfg.Password {
				t.Errorf("password = (%q, %v), want %q", password, ok, c.cfg.Password)
			}
			q := u.Query()
			if q.Get("database") != c.cfg.Database {
				t.Errorf("database = %q, want %q", q.Get("database"), c.cfg.Database)
			}
			if q.Get("encrypt") != "true" {
				t.Errorf("encrypt = %q, want true", q.Get("encrypt"))
			}
			if q.Get("app name") != "cco-acquisition" {
				t.Errorf("app name = %q, want cco-acquisition", q.Get("app name"))
			}
		})
	}
}

func TestFakeDiagnosticsFetchResourceStats(t *testing.T) {
	f := NewFakeDiagnostics()
	ctx := context.Background()

	stats, readings, err := f.FetchResourceStats(ctx, acquisition.SQLConfig{}, acquisition.Target{}, time.Time{})
	if err != nil {
		t.Fatalf("FetchResourceStats: %v", err)
	}
	if len(stats) != 4 {
		t.Fatalf("stats = %d, want 4", len(stats))
	}
	for i := 1; i < len(stats); i++ {
		if gap := stats[i].EndTimeUTC.Sub(stats[i-1].EndTimeUTC); gap != 15*time.Second {
			t.Errorf("stats[%d]-stats[%d] = %v, want 15s", i, i-1, gap)
		}
	}
	if stats[0].DTULimit != nil {
		t.Errorf("DTULimit = %v, want nil", stats[0].DTULimit)
	}
	if stats[0].CPULimit == nil || *stats[0].CPULimit != 2 {
		t.Errorf("CPULimit = %v, want 2", stats[0].CPULimit)
	}
	if stats[0].AvgCPUPercent == nil || *stats[0].AvgCPUPercent != 20 {
		t.Errorf("AvgCPUPercent = %v, want 20", stats[0].AvgCPUPercent)
	}

	if len(readings) != 1 {
		t.Fatalf("readings = %d, want 1", len(readings))
	}
	r := readings[0]
	if r.MetricCode != acquisition.CodeSessionsCount || r.Unit != acquisition.UnitCount ||
		r.Source != acquisition.SourceSQLDiag || r.Average == nil || *r.Average != 7 {
		t.Errorf("reading = %+v, want sessions_count=7 Count sql_diagnostics", r)
	}

	// A future cursor filters every stat row out; the sessions snapshot is a
	// point reading and always comes back.
	stats, readings, err = f.FetchResourceStats(ctx, acquisition.SQLConfig{}, acquisition.Target{}, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("FetchResourceStats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("stats = %d, want 0 for a future lastSeen", len(stats))
	}
	if len(readings) != 1 {
		t.Errorf("readings = %d, want the sessions snapshot regardless of lastSeen", len(readings))
	}
}

func TestFakeMonitorDeterministic(t *testing.T) {
	f := NewFakeMonitor()
	ctx := context.Background()
	start := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Minute)

	first, err := f.FetchPlatformMetrics(ctx, cloud.ConnectionCredentials{}, "/subscriptions/x/resourceGroups/rg/providers/Microsoft.Sql/servers/s/databases/d", start, end)
	if err != nil {
		t.Fatalf("FetchPlatformMetrics: %v", err)
	}
	if want := len(acquisition.AzureMonitorMetricNames) * 3; len(first) != want {
		t.Fatalf("readings = %d, want %d (one per metric per minute)", len(first), want)
	}
	for _, r := range first {
		if r.MetricCode == "" || r.Unit == "" || r.Source != acquisition.SourceAzureMonitor || r.IntervalSeconds != 60 {
			t.Errorf("reading = %+v, want mapped code/unit and 60s interval", r)
		}
		if r.Average == nil || r.Minimum == nil || r.Maximum == nil {
			t.Fatalf("reading = %+v, want avg/min/max set", r)
		}
		if *r.Average < 0 || *r.Average > 100 || *r.Minimum < 0 || *r.Maximum > 100 {
			t.Errorf("reading out of 0..100 range: %+v", r)
		}
	}

	second, err := f.FetchPlatformMetrics(ctx, cloud.ConnectionCredentials{}, "", start, end)
	if err != nil {
		t.Fatalf("FetchPlatformMetrics: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("two calls over the same window differ; the fake must be deterministic")
	}
}

func TestFakeQueryStoreFetchRuntimeStats(t *testing.T) {
	queries, stats, err := NewFakeQueryStore().FetchRuntimeStats(context.Background(), acquisition.SQLConfig{}, acquisition.Target{})
	if err != nil {
		t.Fatalf("FetchRuntimeStats: %v", err)
	}
	if len(queries) != 2 || len(stats) != 2 {
		t.Fatalf("queries/stats = %d/%d, want 2/2", len(queries), len(stats))
	}
	if queries[0].QuerySQLText == nil || *queries[0].QuerySQLText == "" {
		t.Errorf("queries[0] has no SQL text: %+v", queries[0])
	}
	if stats[0].AvgDurationUS == nil || *stats[0].AvgDurationUS != 1500 {
		t.Errorf("stats[0].AvgDurationUS = %v, want 1500", stats[0].AvgDurationUS)
	}
	if stats[1].AvgDurationUS == nil || *stats[1].AvgDurationUS != 42000 {
		t.Errorf("stats[1].AvgDurationUS = %v, want 42000", stats[1].AvgDurationUS)
	}
	if !stats[0].SnapshotAtUTC.Equal(queries[0].SnapshotAtUTC) {
		t.Errorf("snapshot mismatch: stats %v, query %v", stats[0].SnapshotAtUTC, queries[0].SnapshotAtUTC)
	}
}

package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

const (
	amResourceID = "/subscriptions/sub-1/resourceGroups/rg-1/providers/Microsoft.Sql/servers/sql-1/databases/db-1"
	amToken      = "test-token"
	amMetricsURL = "/providers/Microsoft.Insights/metrics"
)

func amCreds() cloud.ConnectionCredentials {
	return cloud.ConnectionCredentials{
		TenantID:       "tenant",
		ClientID:       "client",
		ClientSecret:   "secret-must-not-leak",
		SubscriptionID: "sub-1",
	}
}

// amProvider returns a provider pointed at baseURL with a fake token source.
func amProvider(baseURL string) *AzureMonitorProvider {
	p := newAzureMonitorProviderWithBase(baseURL)
	p.tokenFunc = func(context.Context, cloud.ConnectionCredentials) (string, error) {
		return amToken, nil
	}
	return p
}

func amStart() time.Time {
	return time.Date(2026, 1, 2, 8, 4, 5, 0, time.UTC)
}

func amWriteJSON(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

// amHappyBody: 2 cpu points (second with null aggregations), 1 storage point,
// one metric rejected by Azure (errorCode) and one unknown raw name.
const amHappyBody = `{
  "value": [
    {
      "name": {"value": "cpu_percent"},
      "unit": "Percent",
      "errorCode": "Success",
      "timeseries": [
        {"data": [
          {"timeStamp": "2026-01-02T08:04:00Z", "average": 42.5, "minimum": 40, "maximum": 45, "total": 50, "count": 60},
          {"timeStamp": "2026-01-02T08:05:00Z", "average": 55, "minimum": null, "maximum": null, "total": null, "count": null}
        ]}
      ]
    },
    {
      "name": {"value": "storage"},
      "unit": "Bytes",
      "timeseries": [
        {"data": [{"timeStamp": "2026-01-02T08:04:00Z", "average": 1073741824, "minimum": null, "maximum": null, "total": null, "count": null}]}
      ]
    },
    {
      "name": {"value": "dtu_consumption_percent"},
      "unit": "Percent",
      "errorCode": "MetricNotFound",
      "timeseries": []
    },
    {
      "name": {"value": "foo_bar"},
      "unit": "Count",
      "errorCode": "Success",
      "timeseries": [{"data": [{"timeStamp": "2026-01-02T08:04:00Z", "average": 1}]}]
    }
  ]
}`

const amMinimalBody = `{
  "value": [
    {
      "name": {"value": "cpu_percent"},
      "unit": "Percent",
      "errorCode": "Success",
      "timeseries": [{"data": [{"timeStamp": "2026-01-02T08:04:00Z", "average": 1}]}]
    }
  ]
}`

func TestAzureMonitorFetchHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+amToken {
			t.Errorf("Authorization = %q, want %q", got, "Bearer "+amToken)
		}
		if !strings.HasSuffix(r.URL.Path, amMetricsURL) {
			t.Errorf("path = %q, want suffix %q", r.URL.Path, amMetricsURL)
		}
		q := r.URL.Query()
		want := map[string]string{
			"api-version": "2023-10-01",
			"timespan":    "2026-01-02T08:04:05Z/2026-01-02T09:04:05Z",
			"interval":    "PT1M",
			"metricnames": strings.Join(acquisition.AzureMonitorMetricNames, ","),
			"aggregation": "average,minimum,maximum,total,count",
		}
		for param, wantVal := range want {
			if got := q.Get(param); got != wantVal {
				t.Errorf("query %s = %q, want %q", param, got, wantVal)
			}
		}
		amWriteJSON(t, w, http.StatusOK, amHappyBody)
	}))
	defer srv.Close()

	// Non-UTC input must be converted to UTC in the timespan.
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("UTC-5", -5*3600))
	end := start.Add(time.Hour)

	readings, err := amProvider(srv.URL).FetchPlatformMetrics(context.Background(), amCreds(), amResourceID, start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(readings) != 3 {
		t.Fatalf("expected 3 readings (2 cpu + 1 storage; errorCode and unknown name dropped), got %d", len(readings))
	}

	cpu := readings[0]
	if cpu.MetricCode != acquisition.CodeCPUPercent {
		t.Errorf("reading 0 MetricCode = %q, want %q", cpu.MetricCode, acquisition.CodeCPUPercent)
	}
	if cpu.Unit != acquisition.UnitPercent {
		t.Errorf("reading 0 Unit = %q, want %q", cpu.Unit, acquisition.UnitPercent)
	}
	if cpu.IntervalSeconds != 60 {
		t.Errorf("reading 0 IntervalSeconds = %d, want 60", cpu.IntervalSeconds)
	}
	if cpu.Source != acquisition.SourceAzureMonitor {
		t.Errorf("reading 0 Source = %q, want %q", cpu.Source, acquisition.SourceAzureMonitor)
	}
	if !cpu.Timestamp.Equal(time.Date(2026, 1, 2, 8, 4, 0, 0, time.UTC)) {
		t.Errorf("reading 0 Timestamp = %v, want 2026-01-02T08:04:00Z", cpu.Timestamp)
	}
	if cpu.Average == nil || *cpu.Average != 42.5 {
		t.Errorf("reading 0 Average = %v, want 42.5", cpu.Average)
	}
	if cpu.Minimum == nil || *cpu.Minimum != 40 {
		t.Errorf("reading 0 Minimum = %v, want 40", cpu.Minimum)
	}
	if cpu.Maximum == nil || *cpu.Maximum != 45 {
		t.Errorf("reading 0 Maximum = %v, want 45", cpu.Maximum)
	}
	if cpu.Total == nil || *cpu.Total != 50 {
		t.Errorf("reading 0 Total = %v, want 50", cpu.Total)
	}
	if cpu.Count == nil || *cpu.Count != 60 {
		t.Errorf("reading 0 Count = %v, want 60", cpu.Count)
	}

	// Second cpu point reports only the average; absent aggregations stay nil.
	second := readings[1]
	if !second.Timestamp.Equal(time.Date(2026, 1, 2, 8, 5, 0, 0, time.UTC)) {
		t.Errorf("reading 1 Timestamp = %v, want 2026-01-02T08:05:00Z", second.Timestamp)
	}
	if second.Average == nil || *second.Average != 55 {
		t.Errorf("reading 1 Average = %v, want 55", second.Average)
	}
	if second.Minimum != nil || second.Maximum != nil || second.Total != nil || second.Count != nil {
		t.Errorf("reading 1 expected nil Minimum/Maximum/Total/Count, got %v/%v/%v/%v",
			second.Minimum, second.Maximum, second.Total, second.Count)
	}

	st := readings[2]
	if st.MetricCode != acquisition.CodeStorageUsedBytes {
		t.Errorf("reading 2 MetricCode = %q, want %q", st.MetricCode, acquisition.CodeStorageUsedBytes)
	}
	if st.Unit != acquisition.UnitBytes {
		t.Errorf("reading 2 Unit = %q, want %q", st.Unit, acquisition.UnitBytes)
	}
	if st.Average == nil || *st.Average != 1073741824 {
		t.Errorf("reading 2 Average = %v, want 1073741824", st.Average)
	}
}

func TestAzureMonitorRetryOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		amWriteJSON(t, w, http.StatusOK, amMinimalBody)
	}))
	defer srv.Close()

	readings, err := amProvider(srv.URL).FetchPlatformMetrics(context.Background(), amCreds(), amResourceID, amStart(), amStart().Add(time.Hour))
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 attempts (1 retry), got %d", got)
	}
	if len(readings) != 1 {
		t.Fatalf("expected 1 reading after retry, got %d", len(readings))
	}
}

func TestAzureMonitorRetryExhausted(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := amProvider(srv.URL).FetchPlatformMetrics(context.Background(), amCreds(), amResourceID, amStart(), amStart().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("expected 4 attempts (initial + 3 retries), got %d", got)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 in error, got %v", err)
	}
}

func TestAzureMonitorHTTP400Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		amWriteJSON(t, w, http.StatusBadRequest, `{"error":{"code":"BadRequest","message":"Invalid timespan"}}`)
	}))
	defer srv.Close()

	readings, err := amProvider(srv.URL).FetchPlatformMetrics(context.Background(), amCreds(), amResourceID, amStart(), amStart().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
	if readings != nil {
		t.Fatalf("expected nil readings on error, got %d", len(readings))
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "BadRequest") {
		t.Fatalf("expected status and body in error, got %v", err)
	}
}

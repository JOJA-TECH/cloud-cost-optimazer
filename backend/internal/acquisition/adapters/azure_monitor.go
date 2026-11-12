package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	azpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

// defaultMonitorBase is the public ARM management endpoint.
// Tests override baseURL via newAzureMonitorProviderWithBase to point at httptest.
const defaultMonitorBase = "https://management.azure.com"

const (
	monitorScope       = "https://management.azure.com/.default"
	monitorAPIVersion  = "2023-10-01"
	monitorInterval    = "PT1M"
	monitorMetricsPath = "/providers/Microsoft.Insights/metrics"
	maxRetries429      = 3
)

// tokenFunc acquires a bearer token for given credentials. Tests override it to
// avoid calling Microsoft Entra ID; production uses the real azidentity flow.
type tokenFunc func(ctx context.Context, creds cloud.ConnectionCredentials) (string, error)

// AzureMonitorProvider implements acquisition.MetricsProvider against Azure
// Monitor REST using azidentity client-secret credentials and REST GETs only.
type AzureMonitorProvider struct {
	baseURL    string
	httpClient *http.Client
	tokenFunc  tokenFunc
}

// NewAzureMonitorProvider returns a real provider pointing at the public ARM endpoint.
func NewAzureMonitorProvider() *AzureMonitorProvider {
	return &AzureMonitorProvider{
		baseURL:    defaultMonitorBase,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokenFunc:  realToken,
	}
}

// newAzureMonitorProviderWithBase is an unexported constructor letting tests
// point the ARM base URL at an httptest server.
func newAzureMonitorProviderWithBase(baseURL string) *AzureMonitorProvider {
	return &AzureMonitorProvider{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokenFunc:  realToken,
	}
}

// realToken acquires an ARM bearer token via azidentity client-secret flow.
// It never logs or returns the client secret.
func realToken(ctx context.Context, creds cloud.ConnectionCredentials) (string, error) {
	cred, err := azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.ClientSecret, nil)
	if err != nil {
		return "", fmt.Errorf("build credential: %w", err)
	}
	tok, err := cred.GetToken(ctx, azpolicy.TokenRequestOptions{
		Scopes: []string{monitorScope},
	})
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}
	return tok.Token, nil
}

// FetchPlatformMetrics queries PT1M platform metrics for the resource over
// [start, end] (converted to UTC) and returns raw readings already mapped to
// canonical catalog codes. Metrics unsupported by the resource type (HTTP 200
// with errorCode != "Success") and unknown raw names are dropped silently.
func (p *AzureMonitorProvider) FetchPlatformMetrics(ctx context.Context, creds cloud.ConnectionCredentials, azureResourceID string, start, end time.Time) ([]acquisition.RawReading, error) {
	token, err := p.tokenFunc(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("azure monitor auth: %w", err)
	}

	q := url.Values{}
	q.Set("api-version", monitorAPIVersion)
	q.Set("timespan", start.UTC().Format("2006-01-02T15:04:05Z")+"/"+end.UTC().Format("2006-01-02T15:04:05Z"))
	q.Set("interval", monitorInterval)
	q.Set("metricnames", strings.Join(acquisition.AzureMonitorMetricNames, ","))
	q.Set("aggregation", "average,minimum,maximum,total,count")
	requestURL := p.baseURL + azureResourceID + monitorMetricsPath + "?" + q.Encode()

	body, err := p.getWithRetry(ctx, token, requestURL)
	if err != nil {
		return nil, err
	}

	var payload monitorResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode metrics response: %w", err)
	}

	readings := []acquisition.RawReading{}
	for _, m := range payload.Value {
		if m.ErrorCode != "" && m.ErrorCode != "Success" {
			continue // metric not supported for this resource type: permissive 200
		}
		code, ok := acquisition.MapMetricName(m.Name.Value)
		if !ok {
			continue // unknown raw name: never invent catalog codes
		}
		for _, series := range m.Timeseries {
			for _, pt := range series.Data {
				timestamp, err := time.Parse(time.RFC3339, pt.TimeStamp)
				if err != nil {
					continue // malformed timestamp: drop the point
				}
				readings = append(readings, acquisition.RawReading{
					MetricCode:      code,
					Unit:            acquisition.UnitForCode(code),
					Timestamp:       timestamp.UTC(),
					IntervalSeconds: 60, // requested PT1M granularity
					Average:         pt.Average,
					Minimum:         pt.Minimum,
					Maximum:         pt.Maximum,
					Total:           pt.Total,
					Count:           pt.Count,
					Source:          acquisition.SourceAzureMonitor,
				})
			}
		}
	}
	return readings, nil
}

// getWithRetry performs the GET, retrying up to maxRetries429 times on 429
// while honoring Retry-After seconds (1s when absent). The context deadline
// always wins; other non-200 statuses fail immediately with the status and a
// truncated response body.
func (p *AzureMonitorProvider) getWithRetry(ctx context.Context, token, requestURL string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries429 {
			wait := time.Second
			if secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && secs >= 0 {
				wait = time.Duration(secs) * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		msg := string(body)
		if len(msg) > 256 {
			msg = msg[:256]
		}
		return nil, fmt.Errorf("azure monitor HTTP %d: %s", resp.StatusCode, msg)
	}
}

// monitorResponse is the Microsoft.Insights/metrics envelope.
type monitorResponse struct {
	Value []monitorMetric `json:"value"`
}

type monitorMetric struct {
	Name       monitorName         `json:"name"`
	Unit       string              `json:"unit"`
	ErrorCode  string              `json:"errorCode"`
	Timeseries []monitorTimeseries `json:"timeseries"`
}

type monitorName struct {
	Value string `json:"value"`
}

type monitorTimeseries struct {
	Data []monitorPoint `json:"data"`
}

// monitorPoint carries the nullable PT1M aggregations; absent ones stay nil so
// downstream quality validation sees "not reported".
type monitorPoint struct {
	TimeStamp string   `json:"timeStamp"`
	Average   *float64 `json:"average"`
	Minimum   *float64 `json:"minimum"`
	Maximum   *float64 `json:"maximum"`
	Total     *float64 `json:"total"`
	Count     *float64 `json:"count"`
}

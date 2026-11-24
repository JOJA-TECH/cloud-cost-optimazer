package adapters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

const (
	acResourceID = "/subscriptions/sub-1/resourceGroups/rg-1/providers/Microsoft.Compute/virtualMachines/vm-1"
	acToken      = "test-token"
)

func acCreds() cloud.ConnectionCredentials {
	return cloud.ConnectionCredentials{
		TenantID:       "tenant",
		ClientID:       "client",
		ClientSecret:   "secret-must-not-leak",
		SubscriptionID: "sub-1",
	}
}

func acProvider(baseURL string) *AzureCostProvider {
	p := newAzureCostProviderWithBase(baseURL)
	p.tokenFunc = func(context.Context, cloud.ConnectionCredentials) (string, error) {
		return acToken, nil
	}
	return p
}

func acWriteJSON(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func acColumns() []costColumn {
	return []costColumn{
		{Name: "UsageDate", Type: "String"},
		{Name: "ResourceId", Type: "String"},
		{Name: "ServiceName", Type: "String"},
		{Name: "MeterId", Type: "String"},
		{Name: "ChargeType", Type: "String"},
		{Name: "Cost", Type: "Number"},
		{Name: "UsageQuantity", Type: "Number"},
		{Name: "Currency", Type: "String"},
		{Name: "UsageUnit", Type: "String"},
		{Name: "PricingModel", Type: "String"},
		{Name: "ChargeCategory", Type: "String"},
	}
}

func acRow(date, resourceID, serviceName, meterID, chargeType string, cost, usageQty float64) []interface{} {
	return []interface{}{date, resourceID, serviceName, meterID, chargeType, cost, usageQty, "USD", "Hour", "consumption", "usage"}
}

func acBody(columns []costColumn, rows [][]interface{}, nextLink string) string {
	payload := map[string]interface{}{
		"properties": map[string]interface{}{
			"columns": columns,
			"rows":    rows,
		},
	}
	if nextLink != "" {
		payload["nextLink"] = nextLink
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func TestParseCostResponseValidMultiplePages(t *testing.T) {
	// Page 1 carries a nextLink back to the same server; the handler serves
	// page 1 on the first call and page 2 on the second.
	var page1Calls int
	var page1NextLink string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+acToken {
			t.Errorf("Authorization = %q, want %q", got, "Bearer "+acToken)
		}
		_, _ = io.ReadAll(r.Body)
		if page1Calls == 0 {
			page1Calls++
			acWriteJSON(t, w, http.StatusOK, acBody(acColumns(),
				[][]interface{}{acRow("2026-01-02T00:00:00Z", acResourceID, "Microsoft.Compute/virtualMachines", "meter-1", "Actual", 12.34, 5)},
				page1NextLink))
			return
		}
		acWriteJSON(t, w, http.StatusOK, acBody(acColumns(),
			[][]interface{}{acRow("2026-01-03T00:00:00Z", acResourceID, "Microsoft.Compute/virtualMachines", "meter-1", "Actual", 56.78, 2)},
			""))
	}))
	defer srv.Close()

	page1NextLink = srv.URL + "/subscriptions/sub-1/providers/Microsoft.CostManagement/query?api-version=" + costAPIVersion

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	records, err := acProvider(srv.URL).FetchDailyCosts(context.Background(), acCreds(), "sub-1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records across 2 pages, got %d", len(records))
	}
	wantDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if records[0].UsageDate.Unix() != wantDate.Unix() {
		t.Errorf("record 0 UsageDate = %v, want 2026-01-02", records[0].UsageDate)
	}
	if records[0].ServiceName != "Microsoft.Compute/virtualMachines" {
		t.Errorf("record 0 ServiceName = %q", records[0].ServiceName)
	}
	if records[0].PreTaxCost != 12.34 {
		t.Errorf("record 0 PreTaxCost = %v, want 12.34", records[0].PreTaxCost)
	}
	if records[0].SourceChargeID == nil {
		t.Fatal("record 0 SourceChargeID is nil")
	}
	if records[1].UsageDate.Unix() != time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC).Unix() {
		t.Errorf("record 1 UsageDate = %v, want 2026-01-03", records[1].UsageDate)
	}
}

func TestParseCostResponseMissingColumns(t *testing.T) {
	// No "columns" key at all; rows exist but nothing can be mapped.
	payload := map[string]interface{}{
		"properties": map[string]interface{}{
			"rows": [][]interface{}{
				{"2026-01-02T00:00:00Z", acResourceID, "Microsoft.Compute/virtualMachines"},
			},
		},
	}
	b, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acWriteJSON(t, w, http.StatusOK, string(b))
	}))
	defer srv.Close()

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	records, err := acProvider(srv.URL).FetchDailyCosts(context.Background(), acCreds(), "sub-1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records when columns are missing, got %d", len(records))
	}
}

func TestParseCostResponseEmptyRows(t *testing.T) {
	body := acBody(acColumns(), nil, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acWriteJSON(t, w, http.StatusOK, body)
	}))
	defer srv.Close()

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	records, err := acProvider(srv.URL).FetchDailyCosts(context.Background(), acCreds(), "sub-1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records with empty rows, got %d", len(records))
	}
}

func TestParseCostResponseMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("not json")); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	_, err := acProvider(srv.URL).FetchDailyCosts(context.Background(), acCreds(), "sub-1", from, to)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode cost response") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestBuildCostQueryBody(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	body := buildCostQueryBody(from, to)

	if body["type"] != "Usage" {
		t.Errorf("type = %v, want Usage", body["type"])
	}
	if body["timeframe"] != "Custom" {
		t.Errorf("timeframe = %v, want Custom", body["timeframe"])
	}
	tp, ok := body["timePeriod"].(map[string]string)
	if !ok {
		t.Fatalf("timePeriod not a map: %v", body["timePeriod"])
	}
	if tp["from"] != "2026-01-01" || tp["to"] != "2026-01-05" {
		t.Errorf("timePeriod = %v, want from=2026-01-01 to=2026-01-05", tp)
	}
	ds, ok := body["dataset"].(map[string]interface{})
	if !ok {
		t.Fatalf("dataset not a map: %v", body["dataset"])
	}
	if ds["granularity"] != "Daily" {
		t.Errorf("granularity = %v, want Daily", ds["granularity"])
	}
	agg, ok := ds["aggregation"].(map[string]interface{})
	if !ok {
		t.Fatalf("aggregation not a map: %v", ds["aggregation"])
	}
	totalCost, ok := agg["totalCost"].(map[string]interface{})
	if !ok || totalCost["name"] != "Cost" || totalCost["function"] != "Sum" {
		t.Errorf("totalCost aggregation = %v", agg["totalCost"])
	}
	grouping, ok := ds["grouping"].([]map[string]interface{})
	if !ok || len(grouping) != 4 {
		t.Fatalf("expected 4 grouping dimensions, got %v", ds["grouping"])
	}
	wantGroups := []string{"ResourceId", "ServiceName", "MeterId", "ChargeType"}
	for i, name := range wantGroups {
		g := grouping[i]
		if g["name"] != name || g["type"] != "Dimension" {
			t.Errorf("grouping[%d] = %v, want {Dimension, %s}", i, grouping[i], name)
		}
	}
}

func TestSourceChargeIDDeterministic(t *testing.T) {
	body := acBody(acColumns(), [][]interface{}{acRow("2026-01-02T00:00:00Z", acResourceID, "Microsoft.Compute/virtualMachines", "meter-1", "Actual", 12.34, 5)}, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acWriteJSON(t, w, http.StatusOK, body)
	}))
	defer srv.Close()

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	r1, err := acProvider(srv.URL).FetchDailyCosts(context.Background(), acCreds(), "sub-1", from, to)
	if err != nil {
		t.Fatalf("first fetch error: %v", err)
	}
	r2, err := acProvider(srv.URL).FetchDailyCosts(context.Background(), acCreds(), "sub-1", from, to)
	if err != nil {
		t.Fatalf("second fetch error: %v", err)
	}
	if len(r1) != 1 || len(r2) != 1 {
		t.Fatalf("expected 1 record each, got %d and %d", len(r1), len(r2))
	}
	if *r1[0].SourceChargeID != *r2[0].SourceChargeID {
		t.Errorf("source_charge_id not deterministic: %q vs %q", *r1[0].SourceChargeID, *r2[0].SourceChargeID)
	}
	if !strings.HasPrefix(*r1[0].SourceChargeID, "cost:sub-1:2026-01-02:") {
		t.Errorf("unexpected source_charge_id shape: %q", *r1[0].SourceChargeID)
	}
}

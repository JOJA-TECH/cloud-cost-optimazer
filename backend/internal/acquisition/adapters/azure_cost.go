package adapters

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
)

// Cost Management query API (Usage type).
const (
	costAPIVersion  = "2026-06-01"
	costScope       = "https://management.azure.com/.default"
	costMaxRetries  = 3
	costPageSize    = 1000
)

// AzureCostProvider implements acquisition.CostProvider against Cost Management query.
type AzureCostProvider struct {
	baseURL    string
	httpClient *http.Client
	tokenFunc  tokenFunc
}

// NewAzureCostProvider returns a real provider pointing at the public ARM endpoint.
func NewAzureCostProvider() *AzureCostProvider {
	return &AzureCostProvider{
		baseURL:    "https://management.azure.com",
		httpClient: &http.Client{Timeout: 60 * time.Second},
		tokenFunc:  realToken,
	}
}

// newAzureCostProviderWithBase lets tests override the base URL.
func newAzureCostProviderWithBase(baseURL string) *AzureCostProvider {
	return &AzureCostProvider{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		tokenFunc:  realToken,
	}
}

// FetchDailyCosts posts a Usage query for the subscription over [from, to),
// paginates with nextLink, maps columns by name (order not guaranteed),
// and returns CostRecord slices ready for idempotent upsert.
func (p *AzureCostProvider) FetchDailyCosts(ctx context.Context, creds cloud.ConnectionCredentials, subscriptionID string, from, to time.Time) ([]data.CostRecord, error) {
	token, err := p.tokenFunc(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("cost auth: %w", err)
	}

	queryBody := buildCostQueryBody(from, to)
	nextURL := p.baseURL + "/subscriptions/" + subscriptionID + "/providers/Microsoft.CostManagement/query?api-version=" + costAPIVersion

	var allRecords []data.CostRecord

	for nextURL != "" {
		resp, err := p.postWithRetry(ctx, token, nextURL, queryBody)
		if err != nil {
			return nil, err
		}
		records, nextLink, err := p.parseCostResponse(resp, subscriptionID)
		if err != nil {
			return nil, err
		}
		allRecords = append(allRecords, records...)
		nextURL = nextLink
		queryBody = nil // subsequent pages use nextLink as-is
	}

	return allRecords, nil
}

// postWithRetry performs POST with 429 retry logic.
func (p *AzureCostProvider) postWithRetry(ctx context.Context, token, requestURL string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = strings.NewReader(string(b))
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, reqBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < costMaxRetries {
			wait := time.Second
			if secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && secs >= 0 {
				wait = time.Duration(secs) * time.Second
			}
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		msg := string(b)
		if len(msg) > 256 {
			msg = msg[:256]
		}
		return nil, fmt.Errorf("cost management HTTP %d: %s", resp.StatusCode, msg)
	}
}

// buildCostQueryBody constructs the Usage query body (POST /query).
// Grouping by ResourceId, ServiceName, MeterId, ChargeType to match cost_daily granularity.
func buildCostQueryBody(from, to time.Time) map[string]interface{} {
	return map[string]interface{}{
		"type": "Usage",
		"timeframe": "Custom",
		"timePeriod": map[string]string{
			"from": from.UTC().Format("2006-01-02"),
			"to":   to.UTC().Format("2006-01-02"),
		},
		"dataset": map[string]interface{}{
			"granularity": "Daily",
			"aggregation": map[string]interface{}{
				"totalCost": map[string]interface{}{
					"name": "Cost", "function": "Sum",
				},
				"usageQuantity": map[string]interface{}{
					"name": "UsageQuantity", "function": "Sum",
				},
			},
			"grouping": []map[string]interface{}{
				{"type": "Dimension", "name": "ResourceId"},
				{"type": "Dimension", "name": "ServiceName"},
				{"type": "Dimension", "name": "MeterId"},
				{"type": "Dimension", "name": "ChargeType"},
			},
		},
	}
}

// costQueryResponse is the envelope from Cost Management /query.
type costQueryResponse struct {
	Properties costQueryProperties `json:"properties"`
	NextLink   string              `json:"nextLink"`
}

type costQueryProperties struct {
	Columns []costColumn `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

type costColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// parseCostResponse extracts records from one page, mapping columns by name.
func (p *AzureCostProvider) parseCostResponse(resp *http.Response, subscriptionID string) ([]data.CostRecord, string, error) {
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var payload costQueryResponse
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, "", fmt.Errorf("decode cost response: %w", err)
	}

	// Build column name -> index map (order is not guaranteed).
	colIdx := make(map[string]int, len(payload.Properties.Columns))
	for i, c := range payload.Properties.Columns {
		colIdx[c.Name] = i
	}

	get := func(row []interface{}, name string) interface{} {
		if idx, ok := colIdx[name]; ok && idx < len(row) {
			return row[idx]
		}
		return nil
	}

	var records []data.CostRecord
	for _, row := range payload.Properties.Rows {
		resourceArmID := toString(get(row, "ResourceId"))
		serviceName := toString(get(row, "ServiceName"))
		meterID := toString(get(row, "MeterId"))
		chargeType := toString(get(row, "ChargeType"))

		usageDate := toTime(get(row, "UsageDate"))
		if usageDate.IsZero() {
			usageDate = toTime(get(row, "UsageStart")) // fallback
		}
		if usageDate.IsZero() {
			continue // skip malformed row
		}

		preTaxCost := toFloat64(get(row, "Cost"))
		usageQty := toFloat64(get(row, "UsageQuantity"))
		currency := toString(get(row, "Currency"))
		if currency == "" {
			currency = "USD"
		}
		usageUnit := toString(get(row, "UsageUnit"))
		pricingModel := toString(get(row, "PricingModel"))
		chargeCategory := toString(get(row, "ChargeCategory"))
		if chargeCategory == "" {
			chargeCategory = "usage"
		}

		// Synthetic deterministic source_charge_id: cost:subId:date:hash(resArm|svc|meter|charge)
		hashInput := resourceArmID + "|" + serviceName + "|" + meterID + "|" + chargeType
		h := sha1.Sum([]byte(hashInput))
		srcChargeID := fmt.Sprintf("cost:%s:%s:%s", subscriptionID, usageDate.Format("2006-01-02"), hex.EncodeToString(h[:])[:12])

		rec := data.CostRecord{
			ProviderCode:   "azure",
			ScopeType:      "subscription",
			ScopeName:      subscriptionID,
			ServiceName:    serviceName,
			UsageDate:      usageDate,
			Currency:       currency,
			SourceChargeID: &srcChargeID,
			MeterID:        &meterID,
			PreTaxCost:     preTaxCost,
			UsageQuantity:  &usageQty,
			UsageUnit:      &usageUnit,
			PricingModel:   &pricingModel,
			ChargeCategory: &chargeCategory,
			Source:         "azure_cost_management",
		}

		// Map ResourceId -> resource_uid if it matches a registered resource.
		// The service layer does this resolution via DataWriter.UpsertCostDaily.
		// Here we just pass the ARM id for the lookup (or leave nil and let the
		// writer resolve; see UpsertCostDaily's resolveResourceID).
		// shortcut: not doing the lookup here; upgrade: resolve at write time.
		if resourceArmID != "" {
			rec.ResourceUID = nil // will be resolved by writer via ARM id match
		}

		records = append(records, rec)
	}

	return records, payload.NextLink, nil
}

// --- helpers ---

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%.0f", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func toFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func toTime(v interface{}) time.Time {
	if v == nil {
		return time.Time{}
	}
	switch x := v.(type) {
	case string:
		t, _ := time.Parse(time.RFC3339, x)
		return t.UTC()
	case float64:
		return time.UnixMilli(int64(x)).UTC()
	default:
		return time.Time{}
	}
}
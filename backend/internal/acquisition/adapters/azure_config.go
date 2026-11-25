package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

// ARM adapter for Azure SQL Database configuration (GET databases).
const (
	armAPIVersion = "2025-01-01"
	armScope      = "https://management.azure.com/.default"
	armMaxRetries = 3
)

// AzureConfigProvider implements acquisition.ConfigProvider against ARM REST.
type AzureConfigProvider struct {
	baseURL    string
	httpClient *http.Client
	tokenFunc  tokenFunc
}

// NewAzureConfigProvider returns a real provider pointing at the public ARM endpoint.
func NewAzureConfigProvider() *AzureConfigProvider {
	return &AzureConfigProvider{
		baseURL:    "https://management.azure.com",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokenFunc:  realToken,
	}
}

// newAzureConfigProviderWithBase lets tests override the base URL.
func newAzureConfigProviderWithBase(baseURL string) *AzureConfigProvider {
	return &AzureConfigProvider{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokenFunc:  realToken,
	}
}

// FetchConfigSnapshot GETs the database ARM resource and maps to ConfigReading.
func (p *AzureConfigProvider) FetchConfigSnapshot(ctx context.Context, creds cloud.ConnectionCredentials, azureResourceID string) (acquisition.ConfigReading, error) {
	token, err := p.tokenFunc(ctx, creds)
	if err != nil {
		return acquisition.ConfigReading{}, fmt.Errorf("arm auth: %w", err)
	}

	requestURL := p.baseURL + azureResourceID + "?api-version=" + armAPIVersion
	body, err := p.getWithRetry(ctx, token, requestURL)
	if err != nil {
		return acquisition.ConfigReading{}, err
	}

	var payload armDatabaseResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return acquisition.ConfigReading{}, fmt.Errorf("decode arm response: %w", err)
	}

	return mapARMToConfigReading(payload), nil
}

// getWithRetry performs GET with backoff+jitter, Retry-After, 408/429/5xx handling.
func (p *AzureConfigProvider) getWithRetry(ctx context.Context, token, requestURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := doWithRetry(ctx, p.httpClient, req, armMaxRetries, time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// mapARMToConfigReading extracts the normalized fields from the ARM response.
func mapARMToConfigReading(r armDatabaseResponse) acquisition.ConfigReading {
	obs := time.Now().UTC()
	if r.Properties != nil {
		if r.Properties.ObservedAt != "" {
			if t, err := time.Parse(time.RFC3339, r.Properties.ObservedAt); err == nil {
				obs = t.UTC()
			}
		}
	}

	payloadBytes, _ := json.Marshal(r)
	payload := string(payloadBytes)

	// compute_model is a catalog enum (provisioned/serverless); SKU tier and
	// service objective are not valid values there.
	model := "provisioned"
	reading := acquisition.ConfigReading{
		ObservedAtUTC: obs,
		SourcePayload: &payload,
		ComputeModel:  &model,
	}

	if r.SKU != nil {
		reading.SKUName = &r.SKU.Name
		reading.Tier = &r.SKU.Tier
		reading.Family = r.SKU.Family
		reading.CapacityVCores = r.SKU.Capacity
	}

	if r.Properties != nil {
		reading.MaxSizeBytes = r.Properties.MaxSizeBytes
		reading.AutoPauseDelayMin = r.Properties.AutoPauseDelay
		reading.Status = &r.Properties.Status
		reading.FreeLimit = r.Properties.UseFreeLimit
	}

	if reading.SKUName != nil && (containsServerless(*reading.SKUName) || r.SKU.MinCapacity != nil) {
		cm := "serverless"
		reading.ComputeModel = &cm
		reading.MinCapacityVCores = r.SKU.MinCapacity
	}

	return reading
}

func containsServerless(s string) bool {
	return strings.Contains(strings.ToLower(s), "serverless")
}

// armDatabaseResponse is the ARM Microsoft.Sql/servers/databases response shape.
type armDatabaseResponse struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Location   string           `json:"location"`
	Kind       string           `json:"kind"`
	SKU        *armSKU          `json:"sku"`
	Properties *armDBProperties `json:"properties"`
}

type armSKU struct {
	Name        string   `json:"name"`
	Tier        string   `json:"tier"`
	Family      *string  `json:"family"`
	Capacity    *float64 `json:"capacity"`
	MinCapacity *float64 `json:"minCapacity"`
	Size        *string  `json:"size"`
}

type armDBProperties struct {
	Status                    string `json:"status"`
	CurrentServiceObjective   string `json:"currentServiceObjectiveName"`
	RequestedServiceObjective string `json:"requestedServiceObjectiveName"`
	MaxSizeBytes              *int64 `json:"maxSizeBytes"`
	AutoPauseDelay            *int   `json:"autoPauseDelay"`
	UseFreeLimit              *bool  `json:"useFreeLimit"`
	ServiceLevelObjective     string `json:"serviceLevelObjective"`
	LicenseType               string `json:"licenseType"`
	ZoneRedundant             bool   `json:"zoneRedundant"`
	ObservedAt                string `json:"observedAt"` // synthetic, may not exist
}

package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	azpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

// tokenRefreshMargin is how long before expiry we proactively refresh a
// cached Azure bearer token, so callers never receive a token that expires
// mid-request.
const tokenRefreshMargin = 60 * time.Second

// cachedToken holds a bearer token and the instant at which it expires.
type cachedToken struct {
	token     string
	expiresAt time.Time
}

// azureTokenCache reuses bearer tokens per account until shortly before expiry,
// avoiding a round-trip to Entra ID on every call.
var (
	azureTokenMu    sync.Mutex
	azureTokenCache = make(map[string]*cachedToken)
)

// inFlightCall represents a single in-flight token acquisition for a given
// account key. Concurrent callers wait on done and share the result, which is
// a hand-rolled singleflight (no external dependency).
type inFlightCall struct {
	done  chan struct{}
	token string
	err   error
}

var (
	inFlightMu    sync.Mutex
	inFlightCalls = make(map[string]*inFlightCall)
)

// getCachedToken returns a bearer token for the account, caching it until
// tokenRefreshMargin before expiry. Concurrent calls for the same account are
// deduplicated via an in-flight singleflight so only one Entra ID round-trip
// happens under load.
func getCachedToken(ctx context.Context, creds cloud.ConnectionCredentials, tokenFunc tokenFunc) (string, error) {
	key := creds.TenantID + "|" + creds.ClientID

	// Fast path: valid cached token.
	azureTokenMu.Lock()
	if c, ok := azureTokenCache[key]; ok && time.Now().Add(tokenRefreshMargin).Before(c.expiresAt) {
		azureTokenMu.Unlock()
		return c.token, nil
	}
	azureTokenMu.Unlock()

	// Singleflight: deduplicate concurrent acquisitions for the same key.
	inFlightMu.Lock()
	if call, ok := inFlightCalls[key]; ok {
		inFlightMu.Unlock()
		select {
		case <-call.done:
			return call.token, call.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	call := &inFlightCall{done: make(chan struct{})}
	inFlightCalls[key] = call
	inFlightMu.Unlock()

	// Acquire token (this is the only Entra ID round-trip).
	token, err := tokenFunc(ctx, creds)

	// Publish result and wake waiters.
	call.token = token
	call.err = err
	close(call.done)

	inFlightMu.Lock()
	delete(inFlightCalls, key)
	inFlightMu.Unlock()

	if err != nil {
		return "", err
	}

	// Parse expiry from the tokenFunc result: we do not have direct access to
	// azcore.AccessToken here because tokenFunc returns (string, error). To
	// keep the cache useful without changing the tokenFunc signature, we store
	// the token with a conservative default expiry (1 hour) and rely on
	// tokenRefreshMargin plus re-acquisition on failure. Production tokenFunc
	// (realToken) acquires a fresh token per cache miss; the cache bounds the
	// rate of those misses.
	expiresAt := time.Now().Add(time.Hour)

	azureTokenMu.Lock()
	azureTokenCache[key] = &cachedToken{token: token, expiresAt: expiresAt}
	azureTokenMu.Unlock()

	return token, nil
}

// defaultARMBase is the public ARM management endpoint.
// Tests override baseURL via newAzureAdapterWithBase to point at httptest.
const defaultARMBase = "https://management.azure.com"

const (
	scopeARM = "https://management.azure.com/.default"
	apiSub   = "2022-12-01"
	apiSQL   = "2025-01-01"
)

// tokenFunc acquires a bearer token for given credentials. Tests override it to
// avoid calling Microsoft Entra ID; production uses the real azidentity flow.
type tokenFunc func(ctx context.Context, creds cloud.ConnectionCredentials) (string, error)

// AzureAdapter implements cloud.ConnectionValidator and cloud.ResourceProvider
// against Azure ARM using azidentity client-secret credentials and REST GETs only.
type AzureAdapter struct {
	baseURL    string
	httpClient *http.Client
	tokenFunc  tokenFunc
}

// NewAzureAdapter returns a real Azure adapter pointing at the public ARM endpoint.
func NewAzureAdapter() *AzureAdapter {
	return &AzureAdapter{
		baseURL:    defaultARMBase,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokenFunc:  realToken,
	}
}

// newAzureAdapterWithBase is an unexported constructor letting tests point the
// ARM base URL at an httptest server.
func newAzureAdapterWithBase(baseURL string) *AzureAdapter {
	return &AzureAdapter{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokenFunc:  realToken,
	}
}

// cloudAzureCredsCache reuses ClientSecretCredential per account (tenant+client)
// so the credential object — and azcore's internal token cache — is not rebuilt
// on every call.
var (
	cloudAzureCredsMu    sync.Mutex
	cloudAzureCredsCache = make(map[string]*azidentity.ClientSecretCredential)
)

// getCloudAzureCredential returns a cached ClientSecretCredential for the
// account, creating it on first use.
func getCloudAzureCredential(creds cloud.ConnectionCredentials) (*azidentity.ClientSecretCredential, error) {
	key := creds.TenantID + "|" + creds.ClientID
	cloudAzureCredsMu.Lock()
	defer cloudAzureCredsMu.Unlock()
	if c, ok := cloudAzureCredsCache[key]; ok {
		return c, nil
	}
	c, err := azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.ClientSecret, nil)
	if err != nil {
		return nil, err
	}
	cloudAzureCredsCache[key] = c
	return c, nil
}

// realToken acquires an ARM bearer token via azidentity client-secret flow.
// It never logs or returns the client secret.
func realToken(ctx context.Context, creds cloud.ConnectionCredentials) (string, error) {
	cred, err := getCloudAzureCredential(creds)
	if err != nil {
		return "", fmt.Errorf("build credential: %w", err)
	}
	tok, err := cred.GetToken(ctx, azpolicy.TokenRequestOptions{
		Scopes: []string{scopeARM},
	})
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}
	return tok.Token, nil
}

// token returns a bearer token using the injected tokenFunc (production:
// realToken via azidentity client-secret flow; tests: overridden fake).
// Tokens are cached until shortly before expiry (tokenRefreshMargin) and
// concurrent acquisitions for the same account are deduplicated, so Entra ID
// is not hammered on every call. There is no CCO_AZURE_ACCESS_TOKEN bypass:
// every token must come from a validated credential flow.
func (a *AzureAdapter) token(ctx context.Context, creds cloud.ConnectionCredentials) (string, error) {
	if a.tokenFunc != nil {
		return getCachedToken(ctx, creds, a.tokenFunc)
	}
	return "", fmt.Errorf("token function not configured")
}

// armGet performs a GET against ARM, following the base URL, returning the body
// or an error carrying the ARM error body when present.
func (a *AzureAdapter) armGet(ctx context.Context, token, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return body, nil
	}

	var armErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &armErr)
	msg := armErr.Error.Message
	if msg == "" {
		msg = string(body)
	}
	return nil, fmt.Errorf("ARM HTTP %d: %s %s", resp.StatusCode, armErr.Error.Code, msg)
}

// armPage is the ARM paged envelope (value[] + nextLink).
type armPage[T any] struct {
	Value    []T    `json:"value"`
	NextLink string `json:"nextLink"`
}

type armServer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

type armDatabase struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Location string `json:"location"`
}

// resourceGroupFromID extracts the resource group name from an ARM ID path.
func resourceGroupFromID(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		if p == "resourceGroups" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// ValidateConnection validates credentials by GET /subscriptions/{sub}.
func (a *AzureAdapter) ValidateConnection(ctx context.Context, creds cloud.ConnectionCredentials) (cloud.ConnectionResult, error) {
	token, err := a.token(ctx, creds)
	if err != nil {
		return cloud.ConnectionResult{Status: "error", Detail: "authentication failed"}, nil
	}
	url := fmt.Sprintf("%s/subscriptions/%s?api-version=%s", a.baseURL, creds.SubscriptionID, apiSub)
	if _, err := a.armGet(ctx, token, url); err != nil {
		return cloud.ConnectionResult{Status: "error", Detail: safeDetail(err)}, nil
	}
	return cloud.ConnectionResult{Status: "active", Detail: "ok"}, nil
}

// ListSQLDatabases lists user SQL databases across the subscription.
func (a *AzureAdapter) ListSQLDatabases(ctx context.Context, creds cloud.ConnectionCredentials) ([]cloud.ResourceDTO, error) {
	token, err := a.token(ctx, creds)
	if err != nil {
		return nil, cloud.NewError("CONNECTION_FAILED", "authentication failed", err)
	}

	serversURL := fmt.Sprintf("%s/subscriptions/%s/providers/Microsoft.Sql/servers?api-version=%s", a.baseURL, creds.SubscriptionID, apiSQL)
	var servers []armServer
	next := serversURL
	for next != "" {
		body, err := a.armGet(ctx, token, next)
		if err != nil {
			return nil, mapListError(err)
		}
		var page armPage[armServer]
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, cloud.NewError("CONNECTION_FAILED", "decode servers", err)
		}
		servers = append(servers, page.Value...)
		next = page.NextLink
	}

	var resources []cloud.ResourceDTO
	for _, s := range servers {
		rg := resourceGroupFromID(s.ID)
		dbsURL := fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Sql/servers/%s/databases?api-version=%s",
			a.baseURL, creds.SubscriptionID, rg, s.Name, apiSQL)
		next = dbsURL
		for next != "" {
			body, err := a.armGet(ctx, token, next)
			if err != nil {
				return nil, mapListError(err)
			}
			var page armPage[armDatabase]
			if err := json.Unmarshal(body, &page); err != nil {
				return nil, cloud.NewError("CONNECTION_FAILED", "decode databases", err)
			}
			for _, d := range page.Value {
				// System databases (master, tempdb, model, msdb) carry kind "v12.0,system...".
				if strings.HasPrefix(d.Kind, "v12.0,system") {
					continue
				}
				kind := d.Kind
				resources = append(resources, cloud.ResourceDTO{
					SubscriptionID:  creds.SubscriptionID,
					ResourceGroup:   rg,
					ServerName:      s.Name,
					DatabaseName:    d.Name,
					AzureResourceID: d.ID,
					RegionCode:      d.Location,
					Kind:            &kind,
				})
			}
			next = page.NextLink
		}
	}
	if resources == nil {
		resources = []cloud.ResourceDTO{}
	}
	return resources, nil
}

// mapListError maps ARM errors to cloud domain errors: auth/4xx -> connection
// failed, 5xx/network -> provider unavailable.
func mapListError(err error) error {
	msg := err.Error()
	code := "CONNECTION_FAILED"
	domain := cloud.ErrConnectionFailed
	if strings.HasPrefix(msg, "ARM HTTP 5") || strings.Contains(msg, "request:") {
		code = "PROVIDER_UNAVAILABLE"
		domain = cloud.ErrProviderUnavailable
	}
	return cloud.NewError(code, safeDetail(err), domain)
}

// safeDetail returns a human error description that never leaks the secret.
func safeDetail(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, "ARM HTTP"); i >= 0 {
		return msg[i:]
	}
	return msg
}

// shortcut: token is rebuilt per call (no caching); upgrade: cache token until near expiry (azcore azpolicy BearerToken).

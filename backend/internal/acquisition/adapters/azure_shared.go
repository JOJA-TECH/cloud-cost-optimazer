package adapters

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	azpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

// tokenFunc acquires a bearer token for given credentials. Tests override it to
// avoid calling Microsoft Entra ID; production uses the real azidentity flow.
type tokenFunc func(ctx context.Context, creds cloud.ConnectionCredentials) (string, error)

// azureCredentialCache reuses ClientSecretCredential per account (tenant+client)
// so the credential object — and azcore's internal token cache — is not rebuilt
// on every call. Credentials are immutable after creation, so the map is safe
// for concurrent reads once published under credsMu.
var (
	azureCredsMu    sync.Mutex
	azureCredsCache = make(map[string]*azidentity.ClientSecretCredential)
)

// getCredential returns a cached ClientSecretCredential for the account,
// creating it on first use.
func getCredential(creds cloud.ConnectionCredentials) (*azidentity.ClientSecretCredential, error) {
	key := creds.TenantID + "|" + creds.ClientID
	azureCredsMu.Lock()
	defer azureCredsMu.Unlock()
	if c, ok := azureCredsCache[key]; ok {
		return c, nil
	}
	c, err := azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.ClientSecret, nil)
	if err != nil {
		return nil, err
	}
	azureCredsCache[key] = c
	return c, nil
}

// doWithRetry executes an HTTP request with exponential backoff + jitter,
// honoring Retry-After on 429/408 and retrying 5xx. The caller builds the
// *http.Request (headers, body, auth) so each attempt gets a fresh clone.
func doWithRetry(ctx context.Context, client *http.Client, req *http.Request, maxRetries int, baseBackoff time.Duration) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * baseBackoff
			jitter := time.Duration(rand.Int63n(int64(backoff)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff + jitter):
			}
		}
		resp, err := client.Do(req.Clone(ctx))
		if err != nil {
			lastErr = fmt.Errorf("request: %w", err)
			continue
		}
		switch resp.StatusCode {
		case http.StatusOK:
			return resp, nil
		case http.StatusTooManyRequests, http.StatusRequestTimeout:
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if secs, err := strconv.Atoi(retryAfter); err == nil && secs >= 0 {
					resp.Body.Close()
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(time.Duration(secs) * time.Second):
					}
					continue
				}
			}
			fallthrough
		default:
			if resp.StatusCode >= 500 && resp.StatusCode < 600 && attempt < maxRetries {
				resp.Body.Close()
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
		}
	}
	return nil, lastErr
}

// realToken acquires an ARM bearer token via azidentity client-secret flow.
// It never logs or returns the client secret.
func realToken(ctx context.Context, creds cloud.ConnectionCredentials) (string, error) {
	cred, err := getCredential(creds)
	if err != nil {
		return "", err
	}
	tok, err := cred.GetToken(ctx, azpolicy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

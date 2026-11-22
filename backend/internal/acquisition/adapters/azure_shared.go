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

// retryMetrics tracks retry attempts across Azure calls (simple in-memory
// counters for MVP; can be exported to Prometheus later).
var (
	retryMetricsMu sync.Mutex
	retryAttempts  = make(map[string]int64) // key -> count (subscriptionID or "global")
)

// recordRetry increments the retry counter for the given key.
func recordRetry(key string) {
	retryMetricsMu.Lock()
	defer retryMetricsMu.Unlock()
	retryAttempts[key]++
}

// RetryAttempts returns a copy of the current retry counters (for tests/ops).
func RetryAttempts() map[string]int64 {
	retryMetricsMu.Lock()
	defer retryMetricsMu.Unlock()
	out := make(map[string]int64, len(retryAttempts))
	for k, v := range retryAttempts {
		out[k] = v
	}
	return out
}

// circuitState is the state of a subscription circuit breaker.
type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// circuitBreaker protects a single subscription from cascading failures when
// Azure is repeatedly returning transient errors. It is intentionally tiny:
// failure threshold, recovery timeout, and half-open probe.
type circuitBreaker struct {
	mu            sync.Mutex
	state         circuitState
	failures      int
	lastFail      time.Time
	maxFailures   int
	resetTimeout  time.Duration
	halfOpenMax   int
	halfOpenCalls int
}

func newCircuitBreaker(maxFailures int, resetTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		state:        circuitClosed,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		halfOpenMax:  1,
	}
}

func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case circuitClosed:
		return true
	case circuitOpen:
		if time.Since(cb.lastFail) >= cb.resetTimeout {
			cb.state = circuitHalfOpen
			cb.halfOpenCalls = 0
			return true
		}
		return false
	case circuitHalfOpen:
		if cb.halfOpenCalls >= cb.halfOpenMax {
			return false
		}
		cb.halfOpenCalls++
		return true
	}
	return true
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == circuitHalfOpen {
		cb.state = circuitClosed
		cb.failures = 0
	}
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFail = time.Now()
	if cb.state == circuitHalfOpen {
		cb.state = circuitOpen
		return
	}
	if cb.failures >= cb.maxFailures {
		cb.state = circuitOpen
	}
}

// circuitBreakers holds one breaker per subscription (key = tenant|subscription).
var (
	circuitBreakersMu sync.Mutex
	circuitBreakers   = make(map[string]*circuitBreaker)
)

func getCircuitBreaker(key string) *circuitBreaker {
	circuitBreakersMu.Lock()
	defer circuitBreakersMu.Unlock()
	cb, ok := circuitBreakers[key]
	if !ok {
		cb = newCircuitBreaker(5, 30*time.Second)
		circuitBreakers[key] = cb
	}
	return cb
}

// doWithRetry executes an HTTP request with exponential backoff + jitter,
// honoring Retry-After on 429/408 and retrying 5xx. The caller builds the
// *http.Request (headers, body, auth) so each attempt gets a fresh clone.
// subscriptionID enables a per-subscription circuit breaker; when empty, no
// circuit breaker is applied (backwards compatible).
func doWithRetry(ctx context.Context, client *http.Client, req *http.Request, maxRetries int, baseBackoff time.Duration, subscriptionID ...string) (*http.Response, error) {
	var lastErr error
	retryKey := "global"
	if len(subscriptionID) > 0 && subscriptionID[0] != "" {
		retryKey = subscriptionID[0]
	}
	cb := getCircuitBreaker(retryKey)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if !cb.allow() {
			return nil, fmt.Errorf("circuit breaker open for %s", retryKey)
		}
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
			cb.recordFailure()
			recordRetry(retryKey)
			continue
		}
		switch resp.StatusCode {
		case http.StatusOK:
			cb.recordSuccess()
			return resp, nil
		case http.StatusTooManyRequests, http.StatusRequestTimeout:
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if secs, err := strconv.Atoi(retryAfter); err == nil && secs >= 0 {
					lastErr = fmt.Errorf("HTTP %d: rate limited", resp.StatusCode)
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
				cb.recordFailure()
				recordRetry(retryKey)
				continue
			}
			cb.recordFailure()
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
		}
	}
	if lastErr == nil {
		return nil, fmt.Errorf("retries exhausted for %s", retryKey)
	}
	return nil, lastErr
}

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

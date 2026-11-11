package adapters

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwksCache caches RSA public keys from a JWKS endpoint (e.g. Entra ID).
type jwksCache struct {
	mu        sync.Mutex
	url       string
	client    *http.Client
	ttl       time.Duration
	fetchedAt time.Time
	keys      map[string]*rsa.PublicKey
}

func newJWKSCache(url string) *jwksCache {
	return &jwksCache{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		ttl:    time.Hour,
	}
}

// publicKey returns the RSA key for kid, fetching/refreshing the JWKS on
// first use, TTL expiry, or unknown kid (single refresh attempt on unknown kid).
func (c *jwksCache) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.keys == nil || time.Since(c.fetchedAt) > c.ttl {
		if err := c.fetch(ctx); err != nil {
			return nil, err
		}
	}
	if key, ok := c.keys[kid]; ok {
		return key, nil
	}
	// Unknown kid: refresh once in case of key rotation, then fail.
	if err := c.fetch(ctx); err != nil {
		return nil, err
	}
	if key, ok := c.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("unknown signing key: %s", kid)
}

func (c *jwksCache) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("jwks request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: status %d", resp.StatusCode)
	}

	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: e,
		}
	}
	if len(keys) == 0 {
		return fmt.Errorf("jwks contains no RSA keys")
	}
	c.keys = keys
	c.fetchedAt = time.Now()
	return nil
}

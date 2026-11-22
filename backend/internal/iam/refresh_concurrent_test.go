package iam

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestValidateRefreshTokenConcurrentReuse verifies consume-on-use rotation:
// concurrent attempts to validate the same refresh token must result in
// exactly one success; every other attempt must fail (no double-spend).
func TestValidateRefreshTokenConcurrentReuse(t *testing.T) {
	userID := uuid.New()
	token := "refresh-token-concurrent-test-12345"
	expiresAt := time.Now().Add(24 * time.Hour)

	mgr := NewTokenManager(TokenConfig{}, nil) // nil db => in-memory store
	if err := mgr.persistRefreshToken(token, userID, expiresAt); err != nil {
		t.Fatalf("persistRefreshToken: %v", err)
	}

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers)

	successes := 0
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, ok := mgr.ValidateRefreshToken(token)
			mu.Lock()
			if ok {
				successes++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 successful consume of the refresh token, got %d", successes)
	}
	// After all consumes, the token must be gone.
	if _, ok := mgr.ValidateRefreshToken(token); ok {
		t.Fatal("refresh token should be consumed and unavailable after rotation")
	}
}

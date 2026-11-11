package iam_test

import (
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
)

func TestRefreshTokenPersistenceOnSQLite(t *testing.T) {
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	tm := iam.NewTokenManager(iam.DefaultTokenConfig("test-signing-key"), db)
	userID := uuid.New()

	// Store -> Validate returns the userID.
	tm.StoreRefreshToken("token-a", userID, time.Now().Add(time.Hour))
	got, ok := tm.ValidateRefreshToken("token-a")
	if !ok || got != userID {
		t.Fatalf("expected userID %s, got %s (ok=%v)", userID, got, ok)
	}

	// Second validate returns false: consumed on rotation.
	if _, ok := tm.ValidateRefreshToken("token-a"); ok {
		t.Fatal("expected token to be consumed after first validation")
	}

	// Revoke makes a stored token invalid.
	tm.StoreRefreshToken("token-b", userID, time.Now().Add(time.Hour))
	tm.RevokeRefreshToken("token-b")
	if _, ok := tm.ValidateRefreshToken("token-b"); ok {
		t.Fatal("expected revoked token to be invalid")
	}

	// Expired entry returns false.
	tm.StoreRefreshToken("token-c", userID, time.Now().Add(-time.Hour))
	if _, ok := tm.ValidateRefreshToken("token-c"); ok {
		t.Fatal("expected expired token to be invalid")
	}
}

func TestRefreshTokenInMemoryFallback(t *testing.T) {
	tm := iam.NewTokenManager(iam.DefaultTokenConfig("test-signing-key"), nil)
	userID := uuid.New()
	tm.StoreRefreshToken("token-mem", userID, time.Now().Add(time.Hour))
	got, ok := tm.ValidateRefreshToken("token-mem")
	if !ok || got != userID {
		t.Fatalf("expected userID %s, got %s (ok=%v)", userID, got, ok)
	}
}

package iam

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
)

func TestAccountLockoutValkey(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	vc, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		AlwaysRESP2:  true, // miniredis is safest on RESP2
		DisableCache: true, // required: client-side caching needs RESP3
	})
	if err != nil {
		t.Fatalf("valkey client: %v", err)
	}
	defer vc.Close()
	runLockoutBehaviorTest(t, NewAccountLockout(5, time.Minute, vc))
}

func TestAccountLockoutInMemory(t *testing.T) {
	runLockoutBehaviorTest(t, NewAccountLockout(5, time.Minute, nil))
}

// runLockoutBehaviorTest exercises lock/clear/remaining behavior; both the
// Valkey and the in-memory modes must behave identically.
func runLockoutBehaviorTest(t *testing.T, l *AccountLockout) {
	t.Helper()
	email := "Brute@Example.com" // mixed case on purpose; lookup must lowercase

	if l.IsLocked(email) {
		t.Fatal("fresh email must not be locked")
	}
	if got := l.RemainingAttempts(email); got != 5 {
		t.Fatalf("RemainingAttempts = %d, want 5", got)
	}

	for i := 1; i <= 4; i++ {
		l.RecordFailure(email)
		if l.IsLocked(email) {
			t.Fatalf("locked after %d failures, want lock only at 5", i)
		}
		if got := l.RemainingAttempts(email); got != 5-i {
			t.Fatalf("RemainingAttempts after %d failures = %d, want %d", i, got, 5-i)
		}
	}

	l.RecordFailure(email)
	if !l.IsLocked(email) {
		t.Fatal("email must be locked after max attempts")
	}
	if got := l.RemainingAttempts(email); got != 0 {
		t.Fatalf("RemainingAttempts while locked = %d, want 0", got)
	}

	l.RecordSuccess(email)
	if l.IsLocked(email) {
		t.Fatal("RecordSuccess must clear the lock")
	}
	if got := l.RemainingAttempts(email); got != 5 {
		t.Fatalf("RemainingAttempts after success = %d, want 5", got)
	}
}

package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/jobs"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
)

func newRepo(t *testing.T) jobs.Repository {
	t.Helper()
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// SQLite no tiene schema app: la tabla se llama jobs (sin prefijo).
	if err := db.AutoMigrate(&jobs.Job{}); err != nil {
		t.Fatalf("automigrate jobs: %v", err)
	}
	return jobs.NewRepository(db)
}

func ptrUUID(s string) *uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return &id
}

func TestRepositoryCreateGetListCancel(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	orgID := ptrUUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	key := "idem-key-1"
	idemKey := &key
	job := &jobs.Job{Type: "acquisition:run", Payload: `{"a":1}`, OrganizationID: orgID, IdempotencyKey: idemKey}
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if job.ID == uuid.Nil {
		t.Fatal("create did not assign an id")
	}
	if job.Status != jobs.JobStatusQueued {
		t.Fatalf("expected queued, got %s", job.Status)
	}

	got, err := repo.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Type != "acquisition:run" || got.ID != job.ID {
		t.Fatalf("get mismatch: %+v", got)
	}

	// Idempotencia: misma clave -> mismo job (sin duplicar).
	dup := &jobs.Job{Type: "acquisition:run", Payload: `{"a":1}`, IdempotencyKey: idemKey}
	if err := repo.Create(ctx, dup); err != nil {
		t.Fatalf("create idempotent: %v", err)
	}
	if dup.ID != job.ID {
		t.Fatalf("expected same job id for idempotency key, got %s vs %s", dup.ID, job.ID)
	}
	dup2 := &jobs.Job{Type: "other", Payload: `{"b":2}`, IdempotencyKey: idemKey}
	if err := repo.Create(ctx, dup2); err != nil {
		t.Fatalf("create idempotent conflicting: %v", err)
	}
	if dup2.ID != job.ID {
		t.Fatalf("expected same job id even with different payload, got %s vs %s", dup2.ID, job.ID)
	}

	// List por estado y tipo.
	list, err := repo.List(ctx, jobs.ListFilter{Status: jobs.JobStatusQueued, Type: "acquisition:run"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 queued acquisition job, got %d", len(list))
	}

	// Mark completed.
	if err := repo.MarkCompleted(ctx, job.ID, 123); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	got, err = repo.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get after complete: %v", err)
	}
	if got.Status != jobs.JobStatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.DurationMs == nil || *got.DurationMs != 123 {
		t.Fatalf("expected duration 123, got %v", got.DurationMs)
	}

	// Un job terminal no se puede cancelar.
	if err := repo.MarkCancelled(ctx, job.ID); err == nil {
		t.Fatal("expected error cancelling terminal job")
	}

	// Un job queued sí se cancela.
	other := &jobs.Job{Type: "x", Payload: `{"c":3}`}
	if err := repo.Create(ctx, other); err != nil {
		t.Fatalf("create other: %v", err)
	}
	if err := repo.MarkCancelled(ctx, other.ID); err != nil {
		t.Fatalf("mark cancelled: %v", err)
	}
	got, err = repo.Get(ctx, other.ID)
	if err != nil {
		t.Fatalf("get after cancel: %v", err)
	}
	if got.Status != jobs.JobStatusCancelled {
		t.Fatalf("expected cancelled, got %s", got.Status)
	}
}

func TestRepositoryMarkFailedAndReprogram(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	job := &jobs.Job{Type: "x", Payload: `{}`, MaxAttempts: 2}
	if err := repo.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Reprogram: vuelve a queued con scheduled_at futuro.
	job.ScheduledAt = timePtr(time.Now().UTC().Add(time.Minute))
	job.Error = strPtr("transient")
	if err := repo.Reprogram(ctx, job); err != nil {
		t.Fatalf("reprogram: %v", err)
	}
	got, err := repo.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get after reprogram: %v", err)
	}
	if got.Status != jobs.JobStatusQueued {
		t.Fatalf("expected queued after reprogram, got %s", got.Status)
	}
	if got.ScheduledAt == nil {
		t.Fatal("expected scheduled_at after reprogram")
	}
	if got.Error == nil || *got.Error != "transient" {
		t.Fatalf("expected error retained, got %v", got.Error)
	}

	// Mark failed terminal.
	if err := repo.MarkFailed(ctx, job.ID, "boom", 50); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	got, err = repo.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get after failed: %v", err)
	}
	if got.Status != jobs.JobStatusFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}
	if got.Error == nil || *got.Error != "boom" {
		t.Fatalf("expected error 'boom', got %v", got.Error)
	}
	if got.DurationMs == nil || *got.DurationMs != 50 {
		t.Fatalf("expected duration 50, got %v", got.DurationMs)
	}
}

func TestClaimNotPostgresReturnsNoJobs(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.Claim(context.Background())
	if !errors.Is(err, jobs.ErrNoJobsAvailable) {
		t.Fatalf("expected ErrNoJobsAvailable in sqlite, got %v", err)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
func strPtr(s string) *string         { return &s }

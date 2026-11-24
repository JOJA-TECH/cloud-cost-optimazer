package jobs_test

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/jobs"
	"github.com/google/uuid"
)

// fakeRepository es un Repository en memoria con claim serializado por mutex,
// equivalente en comportamiento a FOR UPDATE SKIP LOCKED: varios workers no
// reclaman la misma fila.
type fakeRepository struct {
	mu    sync.Mutex
	jobs  map[uuid.UUID]*jobs.Job
	order []uuid.UUID
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{jobs: make(map[uuid.UUID]*jobs.Job)}
}

func (r *fakeRepository) Create(ctx context.Context, job *jobs.Job) error {
	if job == nil {
		return jobs.ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	if job.Status == "" {
		job.Status = jobs.JobStatusQueued
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 5
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	r.jobs[job.ID] = job
	r.order = append(r.order, job.ID)
	return nil
}

func (r *fakeRepository) Claim(ctx context.Context) (*jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for _, id := range r.order {
		j := r.jobs[id]
		if j.Status == jobs.JobStatusQueued && (j.ScheduledAt == nil || !j.ScheduledAt.After(now)) {
			j.Status = jobs.JobStatusRunning
			j.Attempts++
			j.StartedAt = &now
			j.UpdatedAt = now
			out := *j
			return &out, nil
		}
	}
	return nil, jobs.ErrNoJobsAvailable
}

func (r *fakeRepository) Update(ctx context.Context, job *jobs.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[job.ID]; !ok {
		return jobs.ErrJobNotFound
	}
	job.UpdatedAt = time.Now().UTC()
	r.jobs[job.ID] = job
	return nil
}

func (r *fakeRepository) Get(ctx context.Context, id uuid.UUID) (*jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return nil, jobs.ErrJobNotFound
	}
	out := *j
	return &out, nil
}

func (r *fakeRepository) List(ctx context.Context, f jobs.ListFilter) ([]jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]jobs.Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, *j)
	}
	return out, nil
}

func (r *fakeRepository) setTerminal(ctx context.Context, id uuid.UUID, status string, errMsg *string, durationMs int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return jobs.ErrJobNotFound
	}
	if jobs.EsTerminal(j.Status) {
		return jobs.ErrJobTerminal
	}
	now := time.Now().UTC()
	j.Status = status
	j.FinishedAt = &now
	j.Error = errMsg
	if durationMs > 0 {
		j.DurationMs = &durationMs
	}
	j.UpdatedAt = now
	return nil
}

func (r *fakeRepository) MarkCompleted(ctx context.Context, id uuid.UUID, durationMs int64) error {
	return r.setTerminal(ctx, id, jobs.JobStatusCompleted, nil, durationMs)
}

func (r *fakeRepository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, durationMs int64) error {
	return r.setTerminal(ctx, id, jobs.JobStatusFailed, &errMsg, durationMs)
}

func (r *fakeRepository) MarkCancelled(ctx context.Context, id uuid.UUID) error {
	return r.setTerminal(ctx, id, jobs.JobStatusCancelled, nil, 0)
}

func (r *fakeRepository) Reprogram(ctx context.Context, job *jobs.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[job.ID]
	if !ok {
		return jobs.ErrJobNotFound
	}
	now := time.Now().UTC()
	j.Status = jobs.JobStatusQueued
	j.ScheduledAt = job.ScheduledAt
	j.Error = job.Error
	j.UpdatedAt = now
	return nil
}

// TestWorkerPoolRunsJobsConcurrently comprueba que el pool de workers ejecuta
// varios jobs a la vez (un job largo no bloquea a los demás) y que todos
// llegan a completado.
func TestWorkerPoolRunsJobsConcurrently(t *testing.T) {
	repo := newFakeRepository()
	var inFlight int32
	var maxInFlight int32
	var mu sync.Mutex
	finished := make(chan struct{}, 6)

	handler := func(ctx context.Context, payload []byte) error {
		v := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if v > maxInFlight {
			maxInFlight = v
		}
		mu.Unlock()
		// Trabajo "largo": sin el pool, este sleep serializaría toda la cola.
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		finished <- struct{}{}
		return nil
	}

	w := jobs.NewWorker(repo, 3, 10*time.Millisecond)
	w.RegisterHandler("batch", handler)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go w.Run(ctx)

	var created []*jobs.Job
	for i := 0; i < 6; i++ {
		job := &jobs.Job{Type: "batch", Payload: `{"n":` + strconv.Itoa(i) + `}`}
		if err := repo.Create(ctx, job); err != nil {
			t.Fatalf("create: %v", err)
		}
		created = append(created, job)
	}

	for i := 0; i < 6; i++ {
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for jobs to finish")
		}
	}

	if got := atomic.LoadInt32(&maxInFlight); got < 2 || got > 3 {
		t.Fatalf("expected max concurrency between 2 and 3, got %d", got)
	}

	for _, job := range created {
		got, err := repo.Get(ctx, job.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status != jobs.JobStatusCompleted {
			t.Fatalf("job %s status %s, want completed", job.ID, got.Status)
		}
		if got.StartedAt == nil {
			t.Fatalf("job %s was never claimed", job.ID)
		}
	}
}

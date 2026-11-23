package jobs

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"sync"
	"time"
)

// backoffBase es la unidad base del backoff exponencial entre reintentos.
const backoffBase = 10 * time.Second

// defaultPollInterval es el intervalo de escaneo del worker cuando está ocioso.
const defaultPollInterval = 5 * time.Second

type worker struct {
	repo         Repository
	mu           sync.RWMutex
	handlers     map[string]HandlerFunc
	pollInterval time.Duration
	concurrency  int
}

// NewWorker construye un pool de N workers que reclaman jobs de la tabla
// duradera. Cada worker ejecuta un job a la vez; Claim usa FOR UPDATE SKIP
// LOCKED, así que varios workers no chocan por la misma fila.
func NewWorker(repo Repository, concurrency int, pollInterval time.Duration) Worker {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	return &worker{
		repo:         repo,
		handlers:     make(map[string]HandlerFunc),
		pollInterval: pollInterval,
		concurrency:  concurrency,
	}
}

// RegisterHandler asocia un tipo de job a su función de ejecución.
func (w *worker) RegisterHandler(jobType string, fn HandlerFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[jobType] = fn
}

// Run lanza `concurrency` goroutines que reclaman y ejecutan jobs hasta que
// ctx se cancele; espera a que termino los en curso antes de devolver.
func (w *worker) Run(ctx context.Context) error {
	slog.Info("jobs worker started", "workers", w.concurrency, "poll_interval", w.pollInterval.String())
	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(w.pollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					w.processOnce(ctx)
				}
			}
		}()
	}
	wg.Wait()
	slog.Info("jobs worker stopped")
	return ctx.Err()
}

func (w *worker) processOnce(ctx context.Context) {
	job, err := w.repo.Claim(ctx)
	if err != nil {
		if !errors.Is(err, ErrNoJobsAvailable) {
			slog.Warn("jobs claim failed", "error", err.Error())
		}
		return
	}

	w.mu.RLock()
	handler, ok := w.handlers[job.Type]
	w.mu.RUnlock()
	if !ok {
		if ferr := w.repo.MarkFailed(ctx, job.ID, "no handler registered for job type "+job.Type, 0); ferr != nil {
			slog.Warn("jobs mark failed (no handler)", "job", job.ID, "error", ferr.Error())
		}
		return
	}

	start := time.Now()
	err = handler(ctx, []byte(job.Payload))
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		w.handleFailure(ctx, job, err.Error(), durationMs)
		return
	}
	if ferr := w.repo.MarkCompleted(ctx, job.ID, durationMs); ferr != nil {
		slog.Warn("jobs mark completed failed", "job", job.ID, "error", ferr.Error())
	}
}

func (w *worker) handleFailure(ctx context.Context, job *Job, errMsg string, durationMs int64) {
	if job.Attempts < job.MaxAttempts {
		// Backoff exponencial con jitter: base * 2^(attempts-1) + [0, base).
		pow := 1
		for i := 1; i < job.Attempts; i++ {
			pow *= 2
		}
		delay := time.Duration(pow) * backoffBase
		if delay > 0 {
			delay += time.Duration(rand.Intn(int(backoffBase)))
		}
		next := time.Now().UTC().Add(delay)
		job.ScheduledAt = &next
		job.Error = &errMsg
		if err := w.repo.Reprogram(ctx, job); err != nil {
			slog.Warn("jobs reprogram failed", "job", job.ID, "error", err.Error())
			return
		}
		slog.Warn("jobs failed, reprogrammed",
			"job", job.ID, "attempt", job.Attempts, "max", job.MaxAttempts,
			"next", next, "error", errMsg)
		return
	}
	if err := w.repo.MarkFailed(ctx, job.ID, errMsg, durationMs); err != nil {
		slog.Warn("jobs mark failed failed", "job", job.ID, "error", err.Error())
	}
	slog.Warn("jobs failed, max attempts reached",
		"job", job.ID, "attempts", job.Attempts, "error", errMsg)
}

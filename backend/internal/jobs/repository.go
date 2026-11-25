package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// defaultMaxAttempts es el límite de reintentos cuando no se especifica.
const defaultMaxAttempts = 5

type repository struct {
	db *gorm.DB
}

// NewRepository construye el repository GORM sobre app.jobs (cco_app).
func NewRepository(db *gorm.DB) Repository {
	// GORM model TableName() is "jobs" for SQLite; scope Postgres to app.jobs.
	if db.Dialector.Name() == "postgres" {
		db = db.Table("app.jobs")
	}
	return &repository{db: db}
}

func (r *repository) tableName() string {
	// app.jobs en Postgres; jobs en SQLite (sin schema).
	if r.db.Dialector.Name() == "postgres" {
		return "app.jobs"
	}
	return "jobs"
}

func (r *repository) Create(ctx context.Context, job *Job) error {
	if job == nil {
		return ErrInvalidInput
	}
	now := time.Now().UTC()
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	if job.Status == "" {
		job.Status = JobStatusQueued
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = defaultMaxAttempts
	}
	job.CreatedAt = now
	job.UpdatedAt = now

	if job.IdempotencyKey != nil && *job.IdempotencyKey != "" {
		// Idempotencia: no duplicar un job ya encolado por la misma clave.
		var existing Job
		err := r.db.WithContext(ctx).
			Where("idempotency_key = ?", *job.IdempotencyKey).
			Order("created_at DESC").
			First(&existing).Error
		if err == nil {
			*job = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := r.db.WithContext(ctx).Create(job).Error; err != nil {
			// Condición de carrera: otro proceso insertó la misma clave entre
			// el select y el create. Intenta devolver el existente.
			if isUniqueViolation(err) {
				var existing Job
				gerr := r.db.WithContext(ctx).
					Where("idempotency_key = ?", *job.IdempotencyKey).
					Order("created_at DESC").
					First(&existing).Error
				if gerr == nil {
					*job = existing
					return nil
				}
				return gerr
			}
			return err
		}
		return nil
	}
	return r.db.WithContext(ctx).Create(job).Error
}

// Claim reclama un job elegible con FOR UPDATE SKIP LOCKED (Postgres).
// En drivers no Postgres devuelve ErrNoJobsAvailable para no fallar en dev.
func (r *repository) Claim(ctx context.Context) (*Job, error) {
	if r.db.Dialector.Name() != "postgres" {
		return nil, ErrNoJobsAvailable
	}
	var job Job
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stmt := fmt.Sprintf(`
			UPDATE %s
			SET status = $1, started_at = now(), attempts = attempts + 1, updated_at = now()
			WHERE id = (
				SELECT id FROM %s
				WHERE status = $2
				  AND (scheduled_at IS NULL OR scheduled_at <= now())
				ORDER BY scheduled_at ASC, created_at ASC
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			)
			RETURNING id, job_type, organization_id, project_id, status, attempts,
				max_attempts, payload, idempotency_key, scheduled_at, started_at,
				finished_at, last_error, duration_ms, created_at, updated_at
		`, r.tableName(), r.tableName())
		if err := tx.Raw(stmt, JobStatusRunning, JobStatusQueued).Scan(&job).Error; err != nil {
			return err
		}
		if job.ID == uuid.Nil {
			return ErrNoJobsAvailable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *repository) Update(ctx context.Context, job *Job) error {
	if job == nil || job.ID == uuid.Nil {
		return ErrInvalidInput
	}
	job.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Model(job).
		Select("status", "attempts", "max_attempts", "payload", "idempotency_key",
			"scheduled_at", "started_at", "finished_at", "error", "duration_ms", "updated_at").
		Updates(job).Error
}

func (r *repository) Get(ctx context.Context, id uuid.UUID) (*Job, error) {
	var job Job
	if err := r.db.WithContext(ctx).First(&job, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	return &job, nil
}

func (r *repository) List(ctx context.Context, f ListFilter) ([]Job, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	tx := r.db.WithContext(ctx)
	if f.Status != "" {
		tx = tx.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		tx = tx.Where("job_type = ?", f.Type)
	}
	if f.OrganizationID != nil && *f.OrganizationID != uuid.Nil {
		tx = tx.Where("organization_id = ?", *f.OrganizationID)
	}
	if f.ProjectID != nil && *f.ProjectID != uuid.Nil {
		tx = tx.Where("project_id = ?", *f.ProjectID)
	}
	var jobs []Job
	if err := tx.Order("created_at DESC").Limit(limit).Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *repository) markTerminal(ctx context.Context, id uuid.UUID) (*Job, error) {
	var job Job
	if err := r.db.WithContext(ctx).First(&job, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	if EsTerminal(job.Status) {
		return nil, ErrJobTerminal
	}
	return &job, nil
}

func (r *repository) MarkCompleted(ctx context.Context, id uuid.UUID, durationMs int64) error {
	if _, err := r.markTerminal(ctx, id); err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&Job{}).
		Where("id = ? AND status IN (?, ?)", id, JobStatusQueued, JobStatusRunning).
		Updates(map[string]any{
			"status":      JobStatusCompleted,
			"finished_at": now,
			"duration_ms": durationMs,
			"updated_at":  now,
		}).Error
}

func (r *repository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, durationMs int64) error {
	if _, err := r.markTerminal(ctx, id); err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&Job{}).
		Where("id = ? AND status IN (?, ?)", id, JobStatusQueued, JobStatusRunning).
		Updates(map[string]any{
			"status":      JobStatusFailed,
			"last_error":  &errMsg,
			"finished_at": now,
			"duration_ms": durationMs,
			"updated_at":  now,
		}).Error
}

func (r *repository) MarkCancelled(ctx context.Context, id uuid.UUID) error {
	if _, err := r.markTerminal(ctx, id); err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&Job{}).
		Where("id = ? AND status IN (?, ?)", id, JobStatusQueued, JobStatusRunning).
		Updates(map[string]any{
			"status":      JobStatusCancelled,
			"finished_at": now,
			"updated_at":  now,
		}).Error
}

func (r *repository) Reprogram(ctx context.Context, job *Job) error {
	if job == nil || job.ID == uuid.Nil {
		return ErrInvalidInput
	}
	now := time.Now().UTC()
	job.UpdatedAt = now
	// Nombres de campo Go: GORM aplica el tag de columna (Error -> last_error).
	return r.db.WithContext(ctx).Model(job).
		Select("Status", "ScheduledAt", "Error", "UpdatedAt").
		Updates(job).Error
}

// isUniqueViolation detecta violación de unique index (Postgres).
func isUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique_violation")
}

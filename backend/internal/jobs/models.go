package jobs

import (
	"time"

	"github.com/google/uuid"
)

// Estados del ciclo de vida de un job.
const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

// EsTerminal devuelve true si el estado no admite más transiciones.
func EsTerminal(status string) bool {
	switch status {
	case JobStatusCompleted, JobStatusFailed, JobStatusCancelled:
		return true
	}
	return false
}

// Job es la fila durable de app.jobs (cco_app). El payload es JSON crudo;
// su interpretación corresponde al handler registrado para el tipo.
type Job struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	Type           string     `gorm:"column:job_type;not null;index" json:"type"`
	OrganizationID *uuid.UUID `gorm:"column:organization_id;type:uuid;index" json:"organization_id,omitempty"`
	ProjectID      *uuid.UUID `gorm:"column:project_id;type:uuid;index" json:"project_id,omitempty"`
	Status         string     `gorm:"column:status;not null;default:'queued';index" json:"status"`
	Attempts       int        `gorm:"column:attempts;not null;default:0" json:"attempts"`
	MaxAttempts    int        `gorm:"column:max_attempts;not null;default:5" json:"max_attempts"`
	Payload        string     `gorm:"column:payload" json:"payload,omitempty"`
	IdempotencyKey *string    `gorm:"column:idempotency_key;uniqueIndex;size:255" json:"idempotency_key,omitempty"`
	ScheduledAt    *time.Time `gorm:"column:scheduled_at" json:"scheduled_at,omitempty"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt     *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	Error          *string    `gorm:"column:last_error;type:text" json:"error,omitempty"`
	DurationMs     *int64     `gorm:"column:duration_ms" json:"duration_ms,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (Job) TableName() string { return "jobs" }

// JobDTO es la vista de API de un job.
type JobDTO struct {
	ID             uuid.UUID  `json:"id"`
	Type           string     `json:"type"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"`
	ProjectID      *uuid.UUID `json:"project_id,omitempty"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Error          *string    `json:"error,omitempty"`
	DurationMs     *int64     `json:"duration_ms,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (j *Job) ToDTO() JobDTO {
	return JobDTO{
		ID:             j.ID,
		Type:           j.Type,
		OrganizationID: j.OrganizationID,
		ProjectID:      j.ProjectID,
		Status:         j.Status,
		Attempts:       j.Attempts,
		MaxAttempts:    j.MaxAttempts,
		ScheduledAt:    j.ScheduledAt,
		StartedAt:      j.StartedAt,
		FinishedAt:     j.FinishedAt,
		Error:          j.Error,
		DurationMs:     j.DurationMs,
		CreatedAt:      j.CreatedAt,
	}
}

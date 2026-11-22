package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// HandlerFunc ejecuta la lógica de un job de un tipo dado. El payload es el
// JSON crudo persistido en la fila. Debe ser idempotente: un mismo payload
// puede ejecutarse más de una vez sin efectos secundarios duplicados.
type HandlerFunc func(ctx context.Context, payload []byte) error

// Repository accede a la tabla duradera de jobs.
type Repository interface {
	// Create inserta un job. Respeta idempotency_key: si ya existe un job no
	// terminal con esa clave, lo devuelve sin duplicar.
	Create(ctx context.Context, job *Job) error
	// Claim reclama un job elegible (FOR UPDATE SKIP LOCKED). Devuelve
	// ErrNoJobsAvailable cuando no hay ninguno.
	Claim(ctx context.Context) (*Job, error)
	// Update persiste cambios sobre una fila existente.
	Update(ctx context.Context, job *Job) error
	// Get devuelve un job por id; ErrJobNotFound cuando no existe.
	Get(ctx context.Context, id uuid.UUID) (*Job, error)
	// List devuelve jobs filtrados, más recientes primero.
	List(ctx context.Context, f ListFilter) ([]Job, error)
	// Marca terminales.
	MarkCompleted(ctx context.Context, id uuid.UUID, durationMs int64) error
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, durationMs int64) error
	MarkCancelled(ctx context.Context, id uuid.UUID) error
	// Reprograma un job fallido para reintento (backoff manejado por el
	// llamante); deja status queued y actualiza scheduled_at.
	Reprogram(ctx context.Context, job *Job) error
}

// ListFilter filtra la listado de jobs.
type ListFilter struct {
	Status      string
	Type        string
	OrganizationID *uuid.UUID
	ProjectID   *uuid.UUID
	Limit       int // <=0 => 50; max 500
}

// Service expone operaciones de encolar, consultar estado y cancelar.
type Service interface {
	// Enqueue crea un job y devuelve su fila (o la existente por idempotencia).
	Enqueue(ctx context.Context, in EnqueueInput) (*Job, error)
	// GetStatus devuelve el DTO de un job.
	GetStatus(ctx context.Context, id uuid.UUID) (JobDTO, error)
	// Cancel marca un job queued/running como cancelled.
	Cancel(ctx context.Context, id uuid.UUID) error
	// List devuelve jobs del caller según filtro.
	List(ctx context.Context, f ListFilter) ([]JobDTO, error)
}

// EnqueueInput son los datos para crear un job.
type EnqueueInput struct {
	Type           string
	Payload        []byte
	OrganizationID *uuid.UUID
	ProjectID      *uuid.UUID
	IdempotencyKey *string
	MaxAttempts    int // <=0 => default 5
	ScheduleIn     *time.Duration // retrasa la primera ejecución
}

// Worker ejecuta jobs reclamándolos de la tabla duradera.
type Worker interface {
	// Run bloquea hasta que ctx se cancele.
	Run(ctx context.Context) error
	// RegisterHandler asocia un tipo de job a su función de ejecución.
	RegisterHandler(jobType string, fn HandlerFunc)
}

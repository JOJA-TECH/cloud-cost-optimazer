package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// service implements Service over Repository.
type service struct {
	repo Repository
}

// NewService builds the jobs service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// Enqueue creates a durable job (or returns the existing one by idempotency key).
func (s *service) Enqueue(ctx context.Context, in EnqueueInput) (*Job, error) {
	if in.Type == "" {
		return nil, ErrInvalidInput
	}
	if in.Payload == nil {
		in.Payload = []byte("null")
	}
	if _, err := json.Marshal(in.Payload); err != nil {
		return nil, fmt.Errorf("%w: payload not serializable: %w", ErrInvalidInput, err)
	}
	job := &Job{
		Type:           in.Type,
		Payload:        string(in.Payload),
		OrganizationID: in.OrganizationID,
		ProjectID:      in.ProjectID,
		IdempotencyKey: in.IdempotencyKey,
		MaxAttempts:    in.MaxAttempts,
		Status:         JobStatusQueued,
	}
	if in.ScheduleIn != nil && *in.ScheduleIn > 0 {
		t := time.Now().UTC().Add(*in.ScheduleIn)
		job.ScheduledAt = &t
	}
	if err := s.repo.Create(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

// GetStatus returns the DTO of a job.
func (s *service) GetStatus(ctx context.Context, id uuid.UUID) (JobDTO, error) {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		return JobDTO{}, err
	}
	return job.ToDTO(), nil
}

// Cancel marks a queued/running job as cancelled.
func (s *service) Cancel(ctx context.Context, id uuid.UUID) error {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if EsTerminal(job.Status) {
		return ErrJobTerminal
	}
	return s.repo.MarkCancelled(ctx, id)
}

// List returns jobs as DTOs according to the filter.
func (s *service) List(ctx context.Context, f ListFilter) ([]JobDTO, error) {
	rows, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]JobDTO, 0, len(rows))
	for _, j := range rows {
		out = append(out, j.ToDTO())
	}
	return out, nil
}

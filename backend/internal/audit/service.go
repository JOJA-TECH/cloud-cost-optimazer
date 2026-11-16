package audit

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Service struct {
	repository AuditRepository
	logger     AuditLogger
}

func NewService(repository AuditRepository, logger AuditLogger) (*Service, error) {
	if repository == nil {
		return nil, ErrAuditRepository
	}
	if logger == nil {
		return nil, ErrAuditLogger
	}
	return &Service{
		repository: repository,
		logger:     logger,
	}, nil
}

func (s *Service) Record(ctx context.Context, event AuditEvent) error {
	if s == nil || s.repository == nil {
		return ErrAuditRepository
	}
	if s.logger == nil {
		return ErrAuditLogger
	}

	normalized, err := normalizeEvent(event)
	if err != nil {
		return err
	}
	enriched, err := enrichFromContext(ctx, normalized)
	if err != nil {
		return err
	}
	if err := validateEvent(enriched); err != nil {
		return err
	}

	persisted, err := s.repository.InsertEvent(ctx, enriched)
	if err != nil {
		if failureLogger, ok := s.logger.(AuditFailureLogger); ok {
			failureLogger.PersistenceFailed(ctx, enriched, err)
		}
		return fmt.Errorf("persist audit event: %w", err)
	}
	if err := s.logger.Record(ctx, persisted); err != nil {
		return fmt.Errorf("log persisted audit event: %w", err)
	}
	return nil
}

func (s *Service) GetEvent(ctx context.Context, eventID int64) (AuditEvent, error) {
	if s == nil || s.repository == nil {
		return AuditEvent{}, ErrAuditRepository
	}
	return s.repository.GetEvent(ctx, eventID)
}

func (s *Service) ListEvents(ctx context.Context, filter AuditFilter) ([]AuditEvent, error) {
	if s == nil || s.repository == nil {
		return nil, ErrAuditRepository
	}
	return s.repository.ListEvents(ctx, filter)
}

func (s *Service) GetResourceHistory(
	ctx context.Context,
	resource ResourceReference,
	filter AuditFilter,
) ([]AuditEvent, error) {
	if s == nil || s.repository == nil {
		return nil, ErrAuditRepository
	}
	return s.repository.ListByResource(ctx, resource, filter)
}

func (s *Service) GetCorrelationHistory(ctx context.Context, correlationID uuid.UUID) ([]AuditEvent, error) {
	if s == nil || s.repository == nil {
		return nil, ErrAuditRepository
	}
	return s.repository.ListByCorrelationID(ctx, correlationID)
}

func enrichFromContext(ctx context.Context, event AuditEvent) (AuditEvent, error) {
	identity, err := AuditIdentityContextFromContext(ctx)
	if err == nil {
		if event.Actor.ActorType != "" && event.Actor.ActorType != identity.Actor.ActorType {
			return AuditEvent{}, fmt.Errorf("%w: actor type conflicts with IAM identity", ErrInvalidAuditEvent)
		}
		if event.Actor.UserID != nil && (identity.Actor.UserID == nil || *event.Actor.UserID != *identity.Actor.UserID) {
			return AuditEvent{}, fmt.Errorf("%w: actor user ID conflicts with IAM identity", ErrInvalidAuditEvent)
		}
		if event.Actor.AuthMethod != "" && event.Actor.AuthMethod != identity.Actor.AuthMethod {
			return AuditEvent{}, fmt.Errorf("%w: actor auth method conflicts with IAM identity", ErrInvalidAuditEvent)
		}
		principalID := event.Actor.PrincipalID
		event.Actor = identity.Actor
		event.Actor.PrincipalID = principalID
		if identity.OrganizationID != nil {
			event.OrganizationID = identity.OrganizationID
		}
		event.Trace = identity.Trace
		return event, nil
	}
	if !errors.Is(err, ErrIdentityContextMissing) {
		return AuditEvent{}, err
	}
	if trace, ok := CorrelationContextFromContext(ctx); ok {
		event.Trace = trace
	}
	return event, nil
}

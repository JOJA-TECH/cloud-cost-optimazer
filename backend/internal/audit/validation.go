package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func validateEvent(event AuditEvent) error {
	if event.EventID != 0 {
		return ErrEventAlreadyPersisted
	}
	if strings.TrimSpace(event.Action) == "" {
		return fmt.Errorf("%w: action must not be blank", ErrInvalidAuditEvent)
	}
	if utf8.RuneCountInString(event.Action) > 80 {
		return fmt.Errorf("%w: action exceeds 80 characters", ErrInvalidAuditEvent)
	}
	if event.OrganizationID != nil && *event.OrganizationID == uuid.Nil {
		return fmt.Errorf("%w: organization ID must not be nil", ErrInvalidAuditEvent)
	}
	if event.Actor.UserID != nil && *event.Actor.UserID == uuid.Nil {
		return fmt.Errorf("%w: user ID must not be nil", ErrInvalidAuditEvent)
	}

	switch event.Actor.ActorType {
	case ActorTypeUser:
		if !validActorID(event.Actor.UserID) {
			return fmt.Errorf("%w: user actor requires a valid user ID", ErrInvalidAuditEvent)
		}
	case ActorTypeServicePrincipal, ActorTypeManagedIdentity:
		if !validActorID(event.Actor.UserID) && !validPrincipalID(event.Actor.PrincipalID) {
			return fmt.Errorf("%w: %s actor requires an identity", ErrInvalidAuditEvent, event.Actor.ActorType)
		}
	case ActorTypeSystem, ActorTypeAnonymous:
		if event.Actor.UserID != nil {
			return fmt.Errorf("%w: %s actor must not have a user ID", ErrInvalidAuditEvent, event.Actor.ActorType)
		}
	default:
		return fmt.Errorf("%w: unsupported actor type %q", ErrInvalidAuditEvent, event.Actor.ActorType)
	}

	if event.Resource.Type == "" {
		if event.Resource.ID != nil || event.Resource.ExternalID != nil {
			return fmt.Errorf("%w: resource identifiers require a resource type", ErrInvalidAuditEvent)
		}
	} else {
		if utf8.RuneCountInString(event.Resource.Type) > 80 {
			return fmt.Errorf("%w: resource type exceeds 80 characters", ErrInvalidAuditEvent)
		}
		if event.Resource.ID != nil && *event.Resource.ID == uuid.Nil {
			return fmt.Errorf("%w: resource ID must not be nil", ErrInvalidAuditEvent)
		}
		if !validResourceID(event.Resource.ID) && !validPrincipalID(event.Resource.ExternalID) {
			return fmt.Errorf("%w: resource reference requires an ID", ErrInvalidAuditEvent)
		}
	}

	if event.Trace.OperationID != nil && *event.Trace.OperationID == uuid.Nil {
		return fmt.Errorf("%w: operation ID must not be nil", ErrInvalidAuditEvent)
	}
	return nil
}

func normalizeEvent(event AuditEvent) (AuditEvent, error) {
	event.Action = strings.TrimSpace(event.Action)
	event.Resource.Type = strings.TrimSpace(event.Resource.Type)
	if event.Resource.ExternalID != nil {
		externalID := strings.TrimSpace(*event.Resource.ExternalID)
		if externalID == "" {
			event.Resource.ExternalID = nil
		} else {
			event.Resource.ExternalID = &externalID
		}
	}
	if event.Actor.ActorType != "" {
		event.Actor.ActorType = strings.TrimSpace(event.Actor.ActorType)
	}
	if event.Actor.AuthMethod != "" {
		event.Actor.AuthMethod = strings.TrimSpace(event.Actor.AuthMethod)
	}
	if event.Actor.PrincipalID != nil {
		principalID := strings.TrimSpace(*event.Actor.PrincipalID)
		if principalID == "" {
			event.Actor.PrincipalID = nil
		} else {
			event.Actor.PrincipalID = &principalID
		}
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	if event.Trace.RequestID != uuid.Nil && event.Trace.CorrelationID == uuid.Nil {
		event.Trace.CorrelationID = event.Trace.RequestID
	}

	metadata, err := normalizeMetadata(event.Metadata)
	if err != nil {
		return AuditEvent{}, fmt.Errorf("%w: metadata: %v", ErrInvalidAuditEvent, err)
	}
	event.Metadata = metadata
	return event, nil
}

func normalizeMetadata(metadata map[string]any) (map[string]any, error) {
	if metadata == nil {
		return map[string]any{}, nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode metadata as JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var normalized map[string]any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("decode normalized metadata: %w", err)
	}
	if normalized == nil {
		return nil, fmt.Errorf("metadata must be a JSON object")
	}
	return normalized, nil
}

func validActorID(id *uuid.UUID) bool {
	return id != nil && *id != uuid.Nil
}

func validResourceID(id *uuid.UUID) bool {
	return id != nil && *id != uuid.Nil
}

func validPrincipalID(id *string) bool {
	return id != nil && strings.TrimSpace(*id) != ""
}

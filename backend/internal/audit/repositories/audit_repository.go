package repositories

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultAuditLimit = 50
	maximumAuditLimit = 200
	// _audit_context stores domain fields that have no dedicated table column.
	auditMetadataKey = "_audit_context"
)

var (
	ErrInvalidAuditFilter       = errors.New("invalid audit filter")
	ErrAuditMetadataKeyReserved = errors.New("audit metadata key is reserved")
)

// GormAuditRepository persists audit events in the application audit schema.
type GormAuditRepository struct {
	db *gorm.DB
}

var _ audit.AuditRepository = (*GormAuditRepository)(nil)

func NewAuditRepository(db *gorm.DB) *GormAuditRepository {
	return &GormAuditRepository{db: db}
}

func (r *GormAuditRepository) InsertEvent(ctx context.Context, event audit.AuditEvent) (audit.AuditEvent, error) {
	model, err := auditEventToModel(event)
	if err != nil {
		return audit.AuditEvent{}, err
	}

	query, err := r.table(ctx)
	if err != nil {
		return audit.AuditEvent{}, err
	}
	if err := query.Create(&model).Error; err != nil {
		return audit.AuditEvent{}, fmt.Errorf("insert audit event: %w", err)
	}
	persisted, err := auditEventFromModel(model)
	if err != nil {
		return audit.AuditEvent{}, fmt.Errorf("map inserted audit event: %w", err)
	}
	return persisted, nil
}

func (r *GormAuditRepository) GetEvent(ctx context.Context, eventID int64) (audit.AuditEvent, error) {
	if eventID <= 0 {
		return audit.AuditEvent{}, fmt.Errorf("%w: event ID must be positive", ErrInvalidAuditFilter)
	}
	query, err := r.table(ctx)
	if err != nil {
		return audit.AuditEvent{}, err
	}

	var model database.AuditEventModel
	if err := query.Where("event_id = ?", uint64(eventID)).Take(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return audit.AuditEvent{}, audit.ErrAuditEventNotFound
		}
		return audit.AuditEvent{}, fmt.Errorf("get audit event: %w", err)
	}
	event, err := auditEventFromModel(model)
	if err != nil {
		return audit.AuditEvent{}, fmt.Errorf("map audit event: %w", err)
	}
	return event, nil
}

func (r *GormAuditRepository) ListEvents(ctx context.Context, filter audit.AuditFilter) ([]audit.AuditEvent, error) {
	filter = normalizeAuditFilter(filter)
	query, err := r.table(ctx)
	if err != nil {
		return nil, err
	}
	query, err = applyAuditFilter(query, filter)
	if err != nil {
		return nil, err
	}
	return r.list(query, filter.Limit, filter.Offset)
}

func (r *GormAuditRepository) ListByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]audit.AuditEvent, error) {
	if correlationID == uuid.Nil {
		return nil, fmt.Errorf("%w: correlation ID must be a non-zero UUID", ErrInvalidAuditFilter)
	}
	query, err := r.table(ctx)
	if err != nil {
		return nil, err
	}
	query = query.Where("correlation_id = ?", correlationID)
	return r.listAll(query)
}

func (r *GormAuditRepository) ListByResource(ctx context.Context, resource audit.ResourceReference, filter audit.AuditFilter) ([]audit.AuditEvent, error) {
	resource.Type = strings.TrimSpace(resource.Type)
	if resource.Type == "" || (resource.ID == nil && resource.ExternalID == nil) {
		return nil, fmt.Errorf("%w: resource type and identifier are required", ErrInvalidAuditFilter)
	}
	if resource.ID != nil && *resource.ID == uuid.Nil {
		return nil, fmt.Errorf("%w: resource ID must be a non-zero UUID", ErrInvalidAuditFilter)
	}
	if resource.ExternalID != nil {
		externalID := strings.TrimSpace(*resource.ExternalID)
		if externalID == "" {
			return nil, fmt.Errorf("%w: external resource ID must not be blank", ErrInvalidAuditFilter)
		}
		resource.ExternalID = &externalID
	}

	filter = normalizeAuditFilter(filter)
	query, err := r.table(ctx)
	if err != nil {
		return nil, err
	}
	query = query.Where("entity_type = ?", resource.Type)
	if resource.ID != nil {
		query = query.Where("entity_id = ?", *resource.ID)
	}
	if resource.ExternalID != nil {
		query, err = applyExternalResourceIDFilter(query, r.db.Dialector.Name(), *resource.ExternalID)
		if err != nil {
			return nil, err
		}
	}
	query, err = applyAuditFilter(query, filter)
	if err != nil {
		return nil, err
	}
	return r.list(query, filter.Limit, filter.Offset)
}

func (r *GormAuditRepository) table(ctx context.Context) (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, audit.ErrAuditRepository
	}
	return r.db.WithContext(ctx).Table(database.TableName(r.db, "audit", "events")), nil
}

func (r *GormAuditRepository) list(query *gorm.DB, limit, offset int) ([]audit.AuditEvent, error) {
	query = query.Limit(limit).Offset(offset)
	return r.listAll(query)
}

func (r *GormAuditRepository) listAll(query *gorm.DB) ([]audit.AuditEvent, error) {
	var models []database.AuditEventModel
	err := query.
		Order("occurred_at DESC").
		Order("event_id DESC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}

	events := make([]audit.AuditEvent, 0, len(models))
	for _, model := range models {
		event, err := auditEventFromModel(model)
		if err != nil {
			return nil, fmt.Errorf("map audit event: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}

func normalizeAuditFilter(filter audit.AuditFilter) audit.AuditFilter {
	if filter.Limit <= 0 {
		filter.Limit = defaultAuditLimit
	} else if filter.Limit > maximumAuditLimit {
		filter.Limit = maximumAuditLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

func applyAuditFilter(query *gorm.DB, filter audit.AuditFilter) (*gorm.DB, error) {
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return nil, fmt.Errorf("%w: from time is after to time", ErrInvalidAuditFilter)
	}
	if filter.OrganizationID != nil {
		if *filter.OrganizationID == uuid.Nil {
			return nil, fmt.Errorf("%w: organization ID must be a non-zero UUID", ErrInvalidAuditFilter)
		}
		query = query.Where("organization_id = ?", *filter.OrganizationID)
	}
	if filter.UserID != nil {
		if *filter.UserID == uuid.Nil {
			return nil, fmt.Errorf("%w: user ID must be a non-zero UUID", ErrInvalidAuditFilter)
		}
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if action := strings.TrimSpace(filter.Action); action != "" {
		query = query.Where("action = ?", action)
	}
	if entityType := strings.TrimSpace(filter.EntityType); entityType != "" {
		query = query.Where("entity_type = ?", entityType)
	}
	if filter.EntityID != nil {
		if *filter.EntityID == uuid.Nil {
			return nil, fmt.Errorf("%w: entity ID must be a non-zero UUID", ErrInvalidAuditFilter)
		}
		query = query.Where("entity_id = ?", *filter.EntityID)
	}
	if filter.CorrelationID != nil {
		if *filter.CorrelationID == uuid.Nil {
			return nil, fmt.Errorf("%w: correlation ID must be a non-zero UUID", ErrInvalidAuditFilter)
		}
		query = query.Where("correlation_id = ?", *filter.CorrelationID)
	}
	if filter.OperationID != nil {
		if *filter.OperationID == uuid.Nil {
			return nil, fmt.Errorf("%w: operation ID must be a non-zero UUID", ErrInvalidAuditFilter)
		}
		query = query.Where("operation_id = ?", *filter.OperationID)
	}
	if filter.From != nil {
		query = query.Where("occurred_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		query = query.Where("occurred_at < ?", filter.To.UTC())
	}
	return query, nil
}

func applyExternalResourceIDFilter(query *gorm.DB, dialect, externalID string) (*gorm.DB, error) {
	switch dialect {
	case "postgres":
		return query.Where("metadata->'_audit_context'->>'resource_external_id' = ?", externalID), nil
	case "sqlite":
		return query.Where("json_extract(metadata, '$._audit_context.resource_external_id') = ?", externalID), nil
	default:
		return nil, fmt.Errorf("filter external resource ID: unsupported database dialect %q", dialect)
	}
}

func auditEventToModel(event audit.AuditEvent) (database.AuditEventModel, error) {
	if event.EventID != 0 {
		return database.AuditEventModel{}, audit.ErrEventAlreadyPersisted
	}

	metadataCapacity := len(event.Metadata)
	if metadataCapacity < int(^uint(0)>>1) {
		metadataCapacity++
	}
	metadata := make(map[string]any, metadataCapacity)
	for key, value := range event.Metadata {
		metadata[key] = value
	}
	if _, exists := metadata[auditMetadataKey]; exists {
		return database.AuditEventModel{}, fmt.Errorf("%w: %q", ErrAuditMetadataKeyReserved, auditMetadataKey)
	}
	storedContext := make(map[string]any, 3)
	if event.Actor.AuthMethod != "" {
		storedContext["actor_auth_method"] = event.Actor.AuthMethod
	}
	if event.Actor.PrincipalID != nil {
		storedContext["actor_principal_id"] = *event.Actor.PrincipalID
	}
	if event.Resource.ExternalID != nil {
		storedContext["resource_external_id"] = *event.Resource.ExternalID
	}
	if len(storedContext) > 0 {
		metadata[auditMetadataKey] = storedContext
	}
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return database.AuditEventModel{}, fmt.Errorf("encode audit metadata: %w", err)
	}

	return database.AuditEventModel{
		OrganizationID: cloneUUID(event.OrganizationID),
		UserID:         cloneUUID(event.Actor.UserID),
		ActorType:      event.Actor.ActorType,
		Action:         event.Action,
		EntityType:     stringPointerOrNil(event.Resource.Type),
		EntityID:       cloneUUID(event.Resource.ID),
		OccurredAt:     event.OccurredAt.UTC(),
		RequestID:      uuidPointerOrNil(event.Trace.RequestID),
		CorrelationID:  uuidPointerOrNil(event.Trace.CorrelationID),
		OperationID:    cloneUUID(event.Trace.OperationID),
		IPAddress:      cloneString(event.IPAddress),
		UserAgent:      cloneString(event.UserAgent),
		EventVersion:   1,
		Metadata:       datatypes.JSON(encodedMetadata),
	}, nil
}

func auditEventFromModel(model database.AuditEventModel) (audit.AuditEvent, error) {
	if model.EventID > uint64(1<<63-1) {
		return audit.AuditEvent{}, fmt.Errorf("audit event ID %d exceeds int64 range", model.EventID)
	}
	metadata, err := decodeAuditMetadata(model.Metadata)
	if err != nil {
		return audit.AuditEvent{}, err
	}

	var authMethod string
	var principalID, externalID *string
	if value, exists := metadata[auditMetadataKey]; exists {
		storedContext, ok := value.(map[string]any)
		if !ok {
			return audit.AuditEvent{}, fmt.Errorf("decode audit metadata: %q must be an object", auditMetadataKey)
		}
		authMethod, err = metadataString(storedContext, "actor_auth_method")
		if err != nil {
			return audit.AuditEvent{}, err
		}
		principalID, err = metadataStringPointer(storedContext, "actor_principal_id")
		if err != nil {
			return audit.AuditEvent{}, err
		}
		externalID, err = metadataStringPointer(storedContext, "resource_external_id")
		if err != nil {
			return audit.AuditEvent{}, err
		}
		delete(metadata, auditMetadataKey)
	}

	var entityType string
	if model.EntityType != nil {
		entityType = *model.EntityType
	}
	return audit.AuditEvent{
		EventID:        int64(model.EventID),
		OrganizationID: cloneUUID(model.OrganizationID),
		Actor: audit.Actor{
			UserID:      cloneUUID(model.UserID),
			ActorType:   model.ActorType,
			AuthMethod:  authMethod,
			PrincipalID: principalID,
		},
		Action: model.Action,
		Resource: audit.ResourceReference{
			Type:       entityType,
			ID:         cloneUUID(model.EntityID),
			ExternalID: externalID,
		},
		OccurredAt: model.OccurredAt.UTC(),
		Trace: audit.CorrelationContext{
			RequestID:     uuidValue(model.RequestID),
			CorrelationID: uuidValue(model.CorrelationID),
			OperationID:   cloneUUID(model.OperationID),
		},
		Metadata:  metadata,
		IPAddress: cloneString(model.IPAddress),
		UserAgent: cloneString(model.UserAgent),
	}, nil
}

func decodeAuditMetadata(raw datatypes.JSON) (map[string]any, error) {
	metadata := make(map[string]any)
	if len(raw) == 0 {
		return metadata, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode audit metadata: %w", err)
	}
	if metadata == nil {
		return nil, errors.New("decode audit metadata: expected an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("decode audit metadata: unexpected trailing value")
		}
		return nil, fmt.Errorf("decode audit metadata: %w", err)
	}
	return metadata, nil
}

func metadataString(values map[string]any, key string) (string, error) {
	value, exists := values[key]
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("decode audit metadata: %q must be a string", key)
	}
	return text, nil
}

func metadataStringPointer(values map[string]any, key string) (*string, error) {
	value, exists := values[key]
	if !exists {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("decode audit metadata: %q must be a string", key)
	}
	return &text, nil
}

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func uuidPointerOrNil(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	copy := value
	return &copy
}

func uuidValue(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func stringPointerOrNil(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

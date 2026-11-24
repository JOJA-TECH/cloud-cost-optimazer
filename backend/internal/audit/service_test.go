package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

type auditRepositoryStub struct {
	AuditRepository
	insert func(context.Context, AuditEvent) (AuditEvent, error)
}

func (r auditRepositoryStub) InsertEvent(ctx context.Context, event AuditEvent) (AuditEvent, error) {
	return r.insert(ctx, event)
}

type auditLoggerStub struct {
	recorded       []AuditEvent
	recordErr      error
	failureEvent   *AuditEvent
	persistenceErr error
}

func (l *auditLoggerStub) Record(_ context.Context, event AuditEvent) error {
	l.recorded = append(l.recorded, event)
	return l.recordErr
}

func (l *auditLoggerStub) PersistenceFailed(_ context.Context, event AuditEvent, err error) {
	l.failureEvent = &event
	l.persistenceErr = err
}

func TestServiceRecordEnrichesPersistsAndLogsEvent(t *testing.T) {
	userID := uuid.New()
	organizationID := uuid.New()
	requestID := uuid.New()
	correlationID := uuid.New()
	operationID := uuid.New()
	ctx := WithCorrelationContext(context.Background(), CorrelationContext{
		RequestID:     uuid.New(),
		CorrelationID: uuid.New(),
		OperationID:   &operationID,
	})
	ctx = iam.WithIdentityContext(ctx, iam.IdentityContext{
		UserID:        userID,
		OrgID:         organizationID,
		AuthMethod:    "password",
		RequestID:     requestID.String(),
		CorrelationID: correlationID.String(),
	})

	originalMetadata := map[string]any{"count": 3, "details": map[string]any{"source": "test"}}
	var inserted AuditEvent
	repository := auditRepositoryStub{
		insert: func(_ context.Context, event AuditEvent) (AuditEvent, error) {
			inserted = event
			event.EventID = 42
			return event, nil
		},
	}
	logger := &auditLoggerStub{}
	service, err := NewService(repository, logger)
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}

	event := AuditEvent{
		Action:   "  resource.read  ",
		Resource: ResourceReference{Type: "resource", ID: uuidPtr(uuid.New())},
		Metadata: originalMetadata,
	}
	if err := service.Record(ctx, event); err != nil {
		t.Fatalf("record audit event: %v", err)
	}
	if inserted.EventID != 0 {
		t.Fatalf("repository insert should receive an unpersisted event, got ID %d", inserted.EventID)
	}
	if inserted.Action != "resource.read" {
		t.Fatalf("action was not normalized: %q", inserted.Action)
	}
	if inserted.OccurredAt.IsZero() || inserted.OccurredAt.Location() != time.UTC {
		t.Fatalf("timestamp was not completed in UTC: %v", inserted.OccurredAt)
	}
	if inserted.Actor.ActorType != ActorTypeUser || inserted.Actor.UserID == nil || *inserted.Actor.UserID != userID {
		t.Fatalf("actor was not enriched from IAM: %#v", inserted.Actor)
	}
	if inserted.Actor.AuthMethod != "password" || inserted.OrganizationID == nil || *inserted.OrganizationID != organizationID {
		t.Fatalf("identity fields were not enriched from IAM: %#v", inserted)
	}
	if inserted.Trace.RequestID != requestID || inserted.Trace.CorrelationID != correlationID {
		t.Fatalf("trace IDs were not sourced from IAM: %#v", inserted.Trace)
	}
	if inserted.Trace.OperationID == nil || *inserted.Trace.OperationID != operationID {
		t.Fatalf("operation ID was not preserved: %#v", inserted.Trace.OperationID)
	}
	if inserted.Metadata == nil {
		t.Fatal("nil metadata was not normalized to an object")
	}
	originalMetadata["count"] = 99
	if got, ok := inserted.Metadata["count"].(json.Number); !ok || got != "3" {
		t.Fatalf("normalized metadata should be an independent copy, got %#v", got)
	}
	if len(logger.recorded) != 1 || logger.recorded[0].EventID != 42 {
		t.Fatalf("persisted event was not logged: %#v", logger.recorded)
	}
	if logger.failureEvent != nil {
		t.Fatalf("unexpected persistence failure log: %#v", logger.failureEvent)
	}
}

func TestServiceRecordRejectsInvalidEventsBeforePersistence(t *testing.T) {
	var insertCalls int
	repository := auditRepositoryStub{
		insert: func(context.Context, AuditEvent) (AuditEvent, error) {
			insertCalls++
			return AuditEvent{}, nil
		},
	}
	service, err := NewService(repository, &auditLoggerStub{})
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}

	err = service.Record(context.Background(), AuditEvent{
		Action: "   ",
		Actor:  Actor{ActorType: ActorTypeSystem},
	})
	if !errors.Is(err, ErrInvalidAuditEvent) {
		t.Fatalf("got %v, want ErrInvalidAuditEvent", err)
	}
	if insertCalls != 0 {
		t.Fatalf("invalid event reached repository %d times", insertCalls)
	}
}

func TestServiceRecordRejectsExistingEventID(t *testing.T) {
	var insertCalls int
	repository := auditRepositoryStub{
		insert: func(context.Context, AuditEvent) (AuditEvent, error) {
			insertCalls++
			return AuditEvent{}, nil
		},
	}
	service, err := NewService(repository, &auditLoggerStub{})
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}

	err = service.Record(context.Background(), AuditEvent{
		EventID: 7,
		Action:  "system.maintenance",
		Actor:   Actor{ActorType: ActorTypeSystem},
	})
	if !errors.Is(err, ErrEventAlreadyPersisted) {
		t.Fatalf("got %v, want ErrEventAlreadyPersisted", err)
	}
	if insertCalls != 0 {
		t.Fatalf("persisted event was inserted again %d times", insertCalls)
	}
}

func TestServiceRecordReturnsAndLogsPersistenceFailure(t *testing.T) {
	persistenceErr := errors.New("database unavailable")
	repository := auditRepositoryStub{
		insert: func(context.Context, AuditEvent) (AuditEvent, error) {
			return AuditEvent{}, persistenceErr
		},
	}
	logger := &auditLoggerStub{}
	service, err := NewService(repository, logger)
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}

	err = service.Record(context.Background(), AuditEvent{
		Action: "system.maintenance",
		Actor:  Actor{ActorType: ActorTypeSystem},
	})
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("got %v, want wrapped persistence error", err)
	}
	if logger.persistenceErr != persistenceErr || logger.failureEvent == nil {
		t.Fatalf("persistence failure was not logged: %#v", logger.failureEvent)
	}
	if len(logger.recorded) != 0 {
		t.Fatalf("unpersisted event was logged as recorded: %#v", logger.recorded)
	}
}

func TestServiceRecordRejectsNonJSONMetadata(t *testing.T) {
	var insertCalls int
	repository := auditRepositoryStub{
		insert: func(context.Context, AuditEvent) (AuditEvent, error) {
			insertCalls++
			return AuditEvent{}, nil
		},
	}

	service, err := NewService(repository, &auditLoggerStub{})
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}

	err = service.Record(context.Background(), AuditEvent{
		Action:   "system.maintenance",
		Actor:    Actor{ActorType: ActorTypeSystem},
		Metadata: map[string]any{"invalid": make(chan int)},
	})
	if !errors.Is(err, ErrInvalidAuditEvent) {
		t.Fatalf("got %v, want ErrInvalidAuditEvent", err)
	}
	if insertCalls != 0 {
		t.Fatalf("invalid metadata reached repository %d times", insertCalls)
	}
}

func TestServiceRecordRejectsSensitiveMetadataKeys(t *testing.T) {
	var insertCalls int
	repository := auditRepositoryStub{
		insert: func(context.Context, AuditEvent) (AuditEvent, error) {
			insertCalls++
			return AuditEvent{}, nil
		},
	}
	service, err := NewService(repository, &auditLoggerStub{})
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}

	err = service.Record(context.Background(), AuditEvent{
		Action: "user.login",
		Actor:  Actor{ActorType: ActorTypeAnonymous},
		Metadata: map[string]any{
			"details": map[string]any{"access_token": "must-not-be-persisted"},
		},
	})
	if !errors.Is(err, ErrInvalidAuditEvent) {
		t.Fatalf("got %v, want ErrInvalidAuditEvent", err)
	}
	if strings.Contains(err.Error(), "must-not-be-persisted") {
		t.Fatalf("validation error leaked a sensitive value: %v", err)
	}
	if insertCalls != 0 {
		t.Fatalf("sensitive metadata reached repository %d times", insertCalls)
	}
}

func uuidPtr(id uuid.UUID) *uuid.UUID {
	return &id
}

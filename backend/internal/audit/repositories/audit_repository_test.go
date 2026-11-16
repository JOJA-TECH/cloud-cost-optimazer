package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestGormAuditRepositoryFiltersAndPaginates(t *testing.T) {
	db := openSQLiteAuditRepository(t)
	repository := NewAuditRepository(db)
	organizationID := uuid.New()
	userID := uuid.New()
	resourceID := uuid.New()
	correlationID := uuid.New()
	operationID := uuid.New()
	externalID := "resource-external-123"
	occurredAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	inserted := make([]audit.AuditEvent, 0, 3)
	for range 3 {
		event, err := repository.InsertEvent(t.Context(), audit.AuditEvent{
			OrganizationID: &organizationID,
			Actor: audit.Actor{
				UserID:    &userID,
				ActorType: audit.ActorTypeUser,
			},
			Action: "recommendation.generated",
			Resource: audit.ResourceReference{
				Type:       "recommendation",
				ID:         &resourceID,
				ExternalID: &externalID,
			},
			OccurredAt: occurredAt,
			Trace: audit.CorrelationContext{
				RequestID:     uuid.New(),
				CorrelationID: correlationID,
				OperationID:   &operationID,
			},
			Metadata: map[string]any{"source": "test"},
		})
		if err != nil {
			t.Fatalf("insert audit event: %v", err)
		}
		inserted = append(inserted, event)
	}

	from := occurredAt
	to := occurredAt.Add(time.Minute)
	events, err := repository.ListEvents(t.Context(), audit.AuditFilter{
		OrganizationID: &organizationID,
		UserID:         &userID,
		Action:         "recommendation.generated",
		EntityType:     "recommendation",
		EntityID:       &resourceID,
		CorrelationID:  &correlationID,
		OperationID:    &operationID,
		From:           &from,
		To:             &to,
		Limit:          2,
		Offset:         1,
	})
	if err != nil {
		t.Fatalf("list filtered audit events: %v", err)
	}
	if len(events) != 2 || events[0].EventID != inserted[1].EventID || events[1].EventID != inserted[0].EventID {
		t.Fatalf("unexpected stable page: %#v", events)
	}

	events, err = repository.ListByResource(t.Context(), audit.ResourceReference{
		Type:       "recommendation",
		ExternalID: &externalID,
	}, audit.AuditFilter{OrganizationID: &organizationID})
	if err != nil {
		t.Fatalf("list resource history by external ID: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d resource events, want 3", len(events))
	}

	events, err = repository.ListByCorrelationID(t.Context(), correlationID)
	if err != nil {
		t.Fatalf("list correlation history: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d correlation events, want 3", len(events))
	}

	events, err = repository.ListEvents(t.Context(), audit.AuditFilter{Action: "' OR 1=1 --"})
	if err != nil {
		t.Fatalf("list using quoted filter value: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("filter value was not treated as data: %#v", events)
	}
}

func TestAuditEventModelMappingPreservesDomainFields(t *testing.T) {
	organizationID := uuid.New()
	userID := uuid.New()
	resourceID := uuid.New()
	requestID := uuid.New()
	correlationID := uuid.New()
	operationID := uuid.New()
	principalID := "spn-123"
	externalID := "/subscriptions/example/resourceGroups/rg/providers/example"
	ipAddress := "192.0.2.1"
	userAgent := "audit-test"
	occurredAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.FixedZone("test", 2*60*60))
	event := audit.AuditEvent{
		OrganizationID: &organizationID,
		Actor: audit.Actor{
			UserID:      &userID,
			ActorType:   audit.ActorTypeServicePrincipal,
			AuthMethod:  "managed_identity",
			PrincipalID: &principalID,
		},
		Action: "recommendation.generated",
		Resource: audit.ResourceReference{
			Type:       "recommendation",
			ID:         &resourceID,
			ExternalID: &externalID,
		},
		OccurredAt: occurredAt,
		Trace: audit.CorrelationContext{
			RequestID:     requestID,
			CorrelationID: correlationID,
			OperationID:   &operationID,
		},
		Metadata:  map[string]any{"count": json.Number("9007199254740993")},
		IPAddress: &ipAddress,
		UserAgent: &userAgent,
	}

	model, err := auditEventToModel(event)
	if err != nil {
		t.Fatalf("map event to model: %v", err)
	}
	model.EventID = 19
	encodedMetadata, err := json.Marshal(model.Metadata)
	if err != nil {
		t.Fatalf("encode model metadata: %v", err)
	}
	model.Metadata = datatypes.JSON(encodedMetadata)

	got, err := auditEventFromModel(model)
	if err != nil {
		t.Fatalf("map model to event: %v", err)
	}
	if got.EventID != 19 || got.OrganizationID == nil || *got.OrganizationID != organizationID {
		t.Fatalf("event identity did not round-trip: %#v", got)
	}
	if got.Actor.UserID == nil || *got.Actor.UserID != userID ||
		got.Actor.ActorType != event.Actor.ActorType ||
		got.Actor.AuthMethod != event.Actor.AuthMethod ||
		got.Actor.PrincipalID == nil || *got.Actor.PrincipalID != principalID {
		t.Fatalf("actor did not round-trip: %#v", got.Actor)
	}
	if got.Resource.Type != event.Resource.Type || got.Resource.ID == nil ||
		*got.Resource.ID != resourceID || got.Resource.ExternalID == nil ||
		*got.Resource.ExternalID != externalID {
		t.Fatalf("resource did not round-trip: %#v", got.Resource)
	}
	if got.Trace.RequestID != requestID || got.Trace.CorrelationID != correlationID ||
		got.Trace.OperationID == nil || *got.Trace.OperationID != operationID {
		t.Fatalf("trace did not round-trip: %#v", got.Trace)
	}
	if !got.OccurredAt.Equal(occurredAt.UTC()) || got.OccurredAt.Location() != time.UTC {
		t.Fatalf("timestamp did not round-trip in UTC: %v", got.OccurredAt)
	}
	if got.IPAddress == nil || *got.IPAddress != ipAddress || got.UserAgent == nil || *got.UserAgent != userAgent {
		t.Fatalf("request metadata did not round-trip: %#v", got)
	}
	if got.Metadata["count"] != json.Number("9007199254740993") {
		t.Fatalf("user metadata did not round-trip precisely: %#v", got.Metadata)
	}
	if _, exists := got.Metadata[auditMetadataKey]; exists {
		t.Fatalf("repository metadata leaked into domain metadata: %#v", got.Metadata)
	}
}

func TestAuditEventMappingRejectsReservedMetadataKey(t *testing.T) {
	externalID := "external-resource"
	_, err := auditEventToModel(audit.AuditEvent{
		Actor:    audit.Actor{AuthMethod: "oidc"},
		Resource: audit.ResourceReference{ExternalID: &externalID},
		Metadata: map[string]any{auditMetadataKey: map[string]any{"user": "value"}},
	})
	if !errors.Is(err, ErrAuditMetadataKeyReserved) {
		t.Fatalf("got %v, want ErrAuditMetadataKeyReserved", err)
	}
}

func TestNormalizeAuditFilterBoundsPagination(t *testing.T) {
	tests := []struct {
		name       string
		filter     audit.AuditFilter
		wantLimit  int
		wantOffset int
	}{
		{name: "default values", wantLimit: defaultAuditLimit},
		{
			name:       "negative values",
			filter:     audit.AuditFilter{Limit: -1, Offset: -10},
			wantLimit:  defaultAuditLimit,
			wantOffset: 0,
		},
		{
			name:       "maximum limit",
			filter:     audit.AuditFilter{Limit: maximumAuditLimit + 1, Offset: 5},
			wantLimit:  maximumAuditLimit,
			wantOffset: 5,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeAuditFilter(test.filter)
			if got.Limit != test.wantLimit || got.Offset != test.wantOffset {
				t.Fatalf("got limit=%d offset=%d, want limit=%d offset=%d",
					got.Limit, got.Offset, test.wantLimit, test.wantOffset)
			}
		})
	}
}

func TestAuditEventMappingRejectsEventIDOutsideDomainRange(t *testing.T) {
	_, err := auditEventFromModel(database.AuditEventModel{EventID: uint64(1 << 63)})
	if err == nil {
		t.Fatal("expected out-of-range event ID to fail")
	}
}

func openSQLiteAuditRepository(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(database.Config{
		Driver: database.DriverSQLite,
		DSN:    fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString()),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "requires cgo") {
			t.Skipf("SQLite driver unavailable: %v", err)
		}
		t.Fatalf("open in-memory SQLite database: %v", err)
	}
	if err := db.AutoMigrate(&database.AuditEventModel{}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "requires cgo") {
			t.Skipf("SQLite driver unavailable: %v", err)
		}
		t.Fatalf("migrate in-memory SQLite database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQLite connection pool: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return db
}

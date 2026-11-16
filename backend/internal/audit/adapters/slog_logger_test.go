package adapters

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/google/uuid"
)

func TestSlogAuditLoggerWritesEventsAndPersistenceFailures(t *testing.T) {
	var output bytes.Buffer
	logger := NewSlogAuditLogger(slog.New(slog.NewJSONHandler(&output, nil)))
	event := audit.AuditEvent{
		EventID: 7,
		Action:  "system.maintenance",
		Actor:   audit.Actor{ActorType: audit.ActorTypeSystem},
		Trace: audit.CorrelationContext{
			RequestID:     uuid.New(),
			CorrelationID: uuid.New(),
		},
	}

	if err := logger.Record(context.Background(), event); err != nil {
		t.Fatalf("log audit event: %v", err)
	}
	logger.PersistenceFailed(context.Background(), event, errors.New("database unavailable"))

	logs := output.String()
	for _, expected := range []string{
		`"msg":"audit event recorded"`,
		`"event_id":7`,
		`"action":"system.maintenance"`,
		`"msg":"audit event persistence failed"`,
		`"error":"audit event persistence failed"`,
		`"error_type":"*errors.errorString"`,
	} {
		if !strings.Contains(logs, expected) {
			t.Errorf("log output missing %s: %s", expected, logs)
		}
	}
}

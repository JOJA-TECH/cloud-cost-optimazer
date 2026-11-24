package adapters

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

const recommendationTraceEventLimit = 200

type AuditTraceReader struct {
	query                audit.AuditQueryService
	decisionTraceService audit.ScopedDecisionTraceService
}

var _ recommendation.RecommendationTraceReader = (*AuditTraceReader)(nil)

func NewAuditTraceReader(
	query audit.AuditQueryService,
	decisionTraceServices ...audit.ScopedDecisionTraceService,
) *AuditTraceReader {
	reader := &AuditTraceReader{query: query}
	if len(decisionTraceServices) > 0 {
		reader.decisionTraceService = decisionTraceServices[0]
	}
	return reader
}

func (r *AuditTraceReader) ListRecommendationTraceEvents(
	ctx context.Context,
	recommendationUID uuid.UUID,
	filter recommendation.RecommendationTraceFilter,
) ([]recommendation.RecommendationTraceEvent, error) {
	if r == nil || r.query == nil {
		return nil, errors.New("audit trace query service is unavailable")
	}
	if recommendationUID == uuid.Nil {
		return nil, errors.New("recommendation UID must be a non-zero UUID")
	}

	baseFilter := audit.AuditFilter{
		OrganizationID: filter.OrganizationID,
		UserID:         filter.UserID,
		Limit:          recommendationTraceEventLimit,
	}
	recommendationID := recommendationUID
	baseFilter.EntityType = "recommendation"
	baseFilter.EntityID = &recommendationID
	var events []audit.AuditEvent
	if r.decisionTraceService != nil {
		trace, err := r.decisionTraceService.GetRecommendationTraceScoped(ctx, recommendationUID, baseFilter)
		if err != nil {
			return nil, fmt.Errorf("get scoped recommendation decision trace: %w", err)
		}
		events = trace.Events
	} else {
		var err error
		events, err = r.query.ListEvents(ctx, baseFilter)
		if err != nil {
			return nil, fmt.Errorf("list recommendation audit events: %w", err)
		}
	}

	combined := make(map[int64]audit.AuditEvent, len(events))
	correlations := make(map[uuid.UUID]struct{})
	operations := make(map[uuid.UUID]struct{})
	for _, event := range events {
		combined[event.EventID] = event
		if event.Trace.CorrelationID != uuid.Nil {
			correlations[event.Trace.CorrelationID] = struct{}{}
		}
		if event.Trace.OperationID != nil && *event.Trace.OperationID != uuid.Nil {
			operations[*event.Trace.OperationID] = struct{}{}
		}
	}

	correlationIDs := make([]uuid.UUID, 0, len(correlations))
	for id := range correlations {
		correlationIDs = append(correlationIDs, id)
	}
	sort.Slice(correlationIDs, func(i, j int) bool {
		return correlationIDs[i].String() < correlationIDs[j].String()
	})
	for _, correlationID := range correlationIDs {
		if len(combined) >= recommendationTraceEventLimit {
			break
		}
		historyFilter := baseFilter
		historyFilter.EntityType = ""
		historyFilter.EntityID = nil
		historyFilter.CorrelationID = uuidPointer(correlationID)
		history, err := r.query.ListEvents(ctx, historyFilter)
		if err != nil {
			return nil, fmt.Errorf("list recommendation correlation history: %w", err)
		}
		addTraceEvents(combined, history)
	}
	operationIDs := make([]uuid.UUID, 0, len(operations))
	for id := range operations {
		operationIDs = append(operationIDs, id)
	}
	sort.Slice(operationIDs, func(i, j int) bool {
		return operationIDs[i].String() < operationIDs[j].String()
	})
	for _, operationID := range operationIDs {
		if len(combined) >= recommendationTraceEventLimit {
			break
		}
		historyFilter := baseFilter
		historyFilter.EntityType = ""
		historyFilter.EntityID = nil
		historyFilter.OperationID = uuidPointer(operationID)
		history, err := r.query.ListEvents(ctx, historyFilter)
		if err != nil {
			return nil, fmt.Errorf("list recommendation operation history: %w", err)
		}
		addTraceEvents(combined, history)
	}

	allEvents := make([]audit.AuditEvent, 0, len(combined))
	for _, event := range combined {
		allEvents = append(allEvents, event)
	}
	sort.Slice(allEvents, func(i, j int) bool {
		if allEvents[i].OccurredAt.Equal(allEvents[j].OccurredAt) {
			return allEvents[i].EventID < allEvents[j].EventID
		}
		return allEvents[i].OccurredAt.Before(allEvents[j].OccurredAt)
	})
	if len(allEvents) > recommendationTraceEventLimit {
		allEvents = allEvents[:recommendationTraceEventLimit]
	}

	result := make([]recommendation.RecommendationTraceEvent, 0, len(allEvents))
	for _, event := range allEvents {
		result = append(result, recommendation.RecommendationTraceEvent{
			EventID: event.EventID, Action: event.Action, OccurredAt: event.OccurredAt.UTC(),
		})
	}
	return result, nil
}

func addTraceEvents(events map[int64]audit.AuditEvent, newEvents []audit.AuditEvent) {
	for _, event := range newEvents {
		if len(events) >= recommendationTraceEventLimit {
			return
		}
		events[event.EventID] = event
	}
}

package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

const reportTraceEventLimit = 200

type AuditTraceReader struct {
	reader audit.ScopedDecisionTraceService
}

func NewAuditTraceReader(reader audit.ScopedDecisionTraceService) *AuditTraceReader {
	return &AuditTraceReader{reader: reader}
}

var _ reporting.AuditTraceReader = (*AuditTraceReader)(nil)

func (r *AuditTraceReader) GetRecommendationTrace(
	ctx context.Context,
	recommendationUID uuid.UUID,
) (reporting.RecommendationTraceData, error) {
	if r == nil || r.reader == nil {
		return reporting.RecommendationTraceData{}, reporting.ErrReportDependencyUnavailable
	}
	if recommendationUID == uuid.Nil {
		return reporting.RecommendationTraceData{}, reporting.ErrInvalidReportRequest
	}
	identity, ok := iam.IdentityContextFromContext(ctx)
	if !ok {
		return reporting.RecommendationTraceData{}, errors.New("report audit trace requires an authenticated identity")
	}
	filter := audit.AuditFilter{Limit: reportTraceEventLimit}
	if !hasReportSystemRole(identity, "platform_admin") {
		if identity.OrgID != uuid.Nil {
			filter.OrganizationID = uuidPointer(identity.OrgID)
		} else if identity.UserID != uuid.Nil {
			filter.UserID = uuidPointer(identity.UserID)
		} else {
			return reporting.RecommendationTraceData{}, errors.New("report audit trace requires an organization or user scope")
		}
	}
	trace, err := r.reader.GetRecommendationTraceScoped(ctx, recommendationUID, filter)
	if err != nil {
		return reporting.RecommendationTraceData{}, fmt.Errorf("get scoped recommendation trace: %w", err)
	}
	result := reporting.RecommendationTraceData{
		RecommendationUID: trace.RecommendationUID,
		DecisionTraces:    make([]reporting.RecommendationDecisionTrace, 0, len(trace.DecisionTraces)),
		Events:            make([]reporting.ReportTraceEvent, 0, len(trace.Events)),
	}
	for _, decision := range trace.DecisionTraces {
		result.DecisionTraces = append(result.DecisionTraces, reporting.RecommendationDecisionTrace{
			TraceUID:           decision.TraceID,
			Action:             decision.Action,
			Status:             decision.Status,
			RiskScore:          decision.RiskScore,
			RiskLevel:          decision.RiskLevel,
			ConfidenceScore:    decision.ConfidenceScore,
			EstimatedSaving:    decision.EstimatedSaving,
			EvidenceCount:      decision.EvidenceCount,
			EvidenceReferences: append([]string(nil), decision.EvidenceReferences...),
			Decision:           decision.Decision,
			Rationale:          decision.Rationale,
			CreatedAt:          decision.CreatedAt,
		})
	}
	for _, event := range trace.Events {
		result.Events = append(result.Events, reporting.ReportTraceEvent{
			EventID:       event.EventID,
			Action:        event.Action,
			OccurredAt:    event.OccurredAt,
			RequestID:     uuidPointer(event.Trace.RequestID),
			CorrelationID: uuidPointer(event.Trace.CorrelationID),
			OperationID:   cloneUUID(event.Trace.OperationID),
		})
	}
	return result, nil
}

func hasReportSystemRole(identity iam.IdentityContext, role string) bool {
	for _, candidate := range identity.SystemRoles {
		if candidate == role {
			return true
		}
	}
	return false
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	return &value
}

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

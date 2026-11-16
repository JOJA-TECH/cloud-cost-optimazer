package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

type RecommendationReportReader struct {
	reader recommendation.RecommendationEngine
}

func NewRecommendationReportReader(reader recommendation.RecommendationEngine) *RecommendationReportReader {
	return &RecommendationReportReader{reader: reader}
}

var _ reporting.RecommendationReader = (*RecommendationReportReader)(nil)

func (r *RecommendationReportReader) GetRecommendation(
	ctx context.Context,
	recommendationUID uuid.UUID,
) (reporting.RecommendationReportData, error) {
	if r == nil || r.reader == nil {
		return reporting.RecommendationReportData{}, reporting.ErrReportDependencyUnavailable
	}
	value, err := r.reader.GetRecommendation(ctx, recommendationUID)
	if err != nil {
		return reporting.RecommendationReportData{}, fmt.Errorf("get recommendation for report: %w", err)
	}
	return recommendationToReportData(value), nil
}

func (r *RecommendationReportReader) ListRecommendations(
	ctx context.Context,
	filter reporting.RecommendationFilter,
) ([]reporting.RecommendationReportData, error) {
	if r == nil || r.reader == nil {
		return nil, reporting.ErrReportDependencyUnavailable
	}
	var resourceUID *uuid.UUID
	if filter.ResourceUID != nil {
		resourceUID = filter.ResourceUID
	}
	values, err := r.reader.ListRecommendations(ctx, recommendation.RecommendationFilter{
		ResourceUID: resourceUID,
		Status:      recommendation.RecommendationStatus(filter.Status),
		Limit:       filter.Limit,
		Offset:      filter.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list recommendations for report: %w", err)
	}
	result := make([]reporting.RecommendationReportData, 0, len(values))
	for _, value := range values {
		result = append(result, recommendationToReportData(value))
	}
	return result, nil
}

func recommendationToReportData(value recommendation.Recommendation) reporting.RecommendationReportData {
	reportView := reporting.ReportRecommendation{
		RecommendationUID: value.UID,
		Action:            string(value.Action.Type),
		Description:       value.Action.Description,
		Status:            string(value.Status),
		EstimatedSaving: reporting.EstimatedSavingView{
			CurrentMonthlyCost:  value.EstimatedSaving.CurrentMonthlyCost,
			ProposedMonthlyCost: value.EstimatedSaving.ProposedMonthlyCost,
			MonthlyAmount:       value.EstimatedSaving.MonthlyAmount,
			MonthlyPercentage:   value.EstimatedSaving.MonthlyPercentage,
			Currency:            value.EstimatedSaving.Currency,
			PeriodStart:         value.EstimatedSaving.PeriodStart,
			PeriodEnd:           value.EstimatedSaving.PeriodEnd,
		},
		Risk:          reporting.RiskView{Score: value.Risk.Score, Level: string(value.Risk.Level)},
		Confidence:    reporting.ConfidenceView{Score: value.Confidence.Score},
		Rationale:     value.Rationale.Summary,
		EvidenceCount: len(value.Evidence),
	}
	result := reporting.RecommendationReportData{
		ReportRecommendation: reportView,
		ResourceUID:          value.ResourceUID,
		Period:               reporting.ReportPeriod{Start: value.WindowStart, End: value.WindowEnd},
		Evidence:             make([]reporting.ReportEvidence, 0, len(value.Evidence)),
	}
	for index, evidence := range value.Evidence {
		reference := fmt.Sprintf("recommendation.recommendation_evidence/%s/%d", value.UID, index)
		result.Evidence = append(result.Evidence, reporting.ReportEvidence{
			Type:        evidence.Type,
			MetricCode:  evidence.MetricCode,
			Value:       evidence.Value,
			Unit:        evidence.Unit,
			Source:      reporting.ReportSourceRecommendation,
			WindowStart: cloneTime(evidence.WindowStartUTC),
			WindowEnd:   cloneTime(evidence.WindowEndUTC),
			Reference:   reference,
			Context:     evidence.Context,
		})
	}
	return result
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

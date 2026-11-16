package adapters

import (
	"context"
	"errors"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

var (
	errAnalyticsReportReaderUnavailable = errors.New("analytics report reader is unavailable")
	errRecommendationReaderUnavailable  = errors.New("recommendation reader is unavailable")
	errAuditTraceReaderUnavailable      = errors.New("audit trace reader is unavailable")
)

type CompositeReportDataProvider struct {
	analytics       reporting.AnalyticsReportReader
	recommendations reporting.RecommendationReader
	audit           reporting.AuditTraceReader
}

var _ reporting.ReportDataProvider = (*CompositeReportDataProvider)(nil)

func NewCompositeReportDataProvider(
	analytics reporting.AnalyticsReportReader,
	recommendations reporting.RecommendationReader,
	audit reporting.AuditTraceReader,
) *CompositeReportDataProvider {
	return &CompositeReportDataProvider{
		analytics:       analytics,
		recommendations: recommendations,
		audit:           audit,
	}
}

func (p *CompositeReportDataProvider) GetAnalytics(
	ctx context.Context,
	input reporting.AnalyticsReportQuery,
) (reporting.AnalyticsReportData, error) {
	if p == nil || p.analytics == nil {
		return reporting.AnalyticsReportData{}, errAnalyticsReportReaderUnavailable
	}
	return p.analytics.BuildReportData(ctx, input.ResourceUID, input.From, input.To)
}

func (p *CompositeReportDataProvider) GetRecommendation(
	ctx context.Context,
	recommendationUID uuid.UUID,
) (reporting.RecommendationReportData, error) {
	if p == nil || p.recommendations == nil {
		return reporting.RecommendationReportData{}, errRecommendationReaderUnavailable
	}
	result, err := p.recommendations.GetRecommendation(ctx, recommendationUID)
	if errors.Is(err, recommendation.ErrRecommendationNotFound) {
		return reporting.RecommendationReportData{}, reporting.ErrReportRecommendationNotFound
	}
	return result, err
}

func (p *CompositeReportDataProvider) GetRecommendations(
	ctx context.Context,
	input reporting.RecommendationReportQuery,
) ([]reporting.RecommendationReportData, error) {
	if p == nil || p.recommendations == nil {
		return nil, errRecommendationReaderUnavailable
	}
	return p.recommendations.ListRecommendations(ctx, reporting.RecommendationFilter{
		ResourceUID: input.ResourceUID,
		Status:      input.Status,
		Limit:       input.Limit,
		Offset:      input.Offset,
	})
}

func (p *CompositeReportDataProvider) GetRecommendationTrace(
	ctx context.Context,
	recommendationUID uuid.UUID,
) (reporting.RecommendationTraceData, error) {
	if p == nil || p.audit == nil {
		return reporting.RecommendationTraceData{}, errAuditTraceReaderUnavailable
	}
	return p.audit.GetRecommendationTrace(ctx, recommendationUID)
}

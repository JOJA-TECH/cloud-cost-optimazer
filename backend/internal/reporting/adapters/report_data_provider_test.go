package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

type analyticsReportReaderStub struct {
	resourceUID uuid.UUID
	from        time.Time
	to          time.Time
	result      reporting.AnalyticsReportData
}

func (s *analyticsReportReaderStub) BuildReportData(
	_ context.Context,
	resourceUID uuid.UUID,
	from, to time.Time,
) (reporting.AnalyticsReportData, error) {
	s.resourceUID, s.from, s.to = resourceUID, from, to
	return s.result, nil
}

type recommendationReaderStub struct {
	filter reporting.RecommendationFilter
	result []reporting.RecommendationReportData
}

func (s *recommendationReaderStub) GetRecommendation(
	context.Context,
	uuid.UUID,
) (reporting.RecommendationReportData, error) {
	return reporting.RecommendationReportData{}, nil
}

func (s *recommendationReaderStub) ListRecommendations(
	_ context.Context,
	filter reporting.RecommendationFilter,
) ([]reporting.RecommendationReportData, error) {
	s.filter = filter
	return s.result, nil
}

type auditTraceReaderStub struct {
	recommendationUID uuid.UUID
	result            reporting.RecommendationTraceData
}

func (s *auditTraceReaderStub) GetRecommendationTrace(
	_ context.Context,
	recommendationUID uuid.UUID,
) (reporting.RecommendationTraceData, error) {
	s.recommendationUID = recommendationUID
	return s.result, nil
}

func TestCompositeReportDataProviderDelegatesWithoutTransformingResults(t *testing.T) {
	resourceUID := uuid.New()
	recommendationUID := uuid.New()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	status := "accepted"
	analyticsData := reporting.AnalyticsReportData{ResourceUID: resourceUID}
	recommendationData := []reporting.RecommendationReportData{{ResourceUID: resourceUID}}
	traceData := reporting.RecommendationTraceData{RecommendationUID: recommendationUID}

	analytics := &analyticsReportReaderStub{result: analyticsData}
	recommendations := &recommendationReaderStub{result: recommendationData}
	audit := &auditTraceReaderStub{result: traceData}
	provider := NewCompositeReportDataProvider(analytics, recommendations, audit)
	ctx := context.Background()

	gotAnalytics, err := provider.GetAnalytics(ctx, reporting.AnalyticsReportQuery{
		ResourceUID: resourceUID,
		From:        from,
		To:          to,
	})
	if err != nil {
		t.Fatalf("GetAnalytics() error = %v", err)
	}
	if gotAnalytics.ResourceUID != analyticsData.ResourceUID ||
		analytics.resourceUID != resourceUID ||
		!analytics.from.Equal(from) ||
		!analytics.to.Equal(to) {
		t.Fatal("GetAnalytics() did not forward the query and reader result")
	}

	gotRecommendations, err := provider.GetRecommendations(ctx, reporting.RecommendationReportQuery{
		ResourceUID: &resourceUID,
		Status:      status,
		Limit:       10,
		Offset:      2,
	})
	if err != nil {
		t.Fatalf("GetRecommendations() error = %v", err)
	}
	if len(gotRecommendations) != len(recommendationData) ||
		recommendations.filter.ResourceUID == nil ||
		*recommendations.filter.ResourceUID != resourceUID ||
		recommendations.filter.Status != status ||
		recommendations.filter.Limit != 10 ||
		recommendations.filter.Offset != 2 {
		t.Fatal("GetRecommendations() did not forward the filter and reader result")
	}

	gotTrace, err := provider.GetRecommendationTrace(ctx, recommendationUID)
	if err != nil {
		t.Fatalf("GetRecommendationTrace() error = %v", err)
	}
	if gotTrace.RecommendationUID != traceData.RecommendationUID ||
		audit.recommendationUID != recommendationUID {
		t.Fatal("GetRecommendationTrace() did not forward the UID and reader result")
	}
}

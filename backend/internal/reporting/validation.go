package reporting

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxReportPeriod                  = 365 * 24 * time.Hour
	maxExplicitReportRecommendations = 100
	maxReportTitleLength             = 200
)

func ValidateGenerateReportInput(input GenerateReportInput) error {
	if input.ReportType != ReportTypeOptimization {
		return fmt.Errorf("%w: only optimization reports are available", ErrInvalidReportRequest)
	}
	if input.ProjectID == uuid.Nil {
		return fmt.Errorf("%w: project is required", ErrInvalidReportRequest)
	}
	if input.ResourceUID == nil || *input.ResourceUID == uuid.Nil {
		return fmt.Errorf("%w: resource is required", ErrInvalidReportRequest)
	}
	if input.RequestedBy == uuid.Nil {
		return fmt.Errorf("%w: requesting actor is required", ErrInvalidReportRequest)
	}
	if err := validateReportPeriod(ReportPeriod{Start: input.From, End: input.To}); err != nil {
		return err
	}
	if err := ValidateReportFormat(input.Format); err != nil {
		return err
	}
	if err := validateRecommendationUIDs(input.RecommendationUIDs); err != nil {
		return err
	}
	return validateReportTitle(input.Title)
}

func ValidateOptimizationReportInput(input OptimizationReportInput) error {
	if input.ProjectID == uuid.Nil {
		return fmt.Errorf("%w: project is required", ErrInvalidReportRequest)
	}
	if input.ResourceUID == uuid.Nil {
		return fmt.Errorf("%w: resource is required", ErrInvalidReportRequest)
	}
	if input.RequestedBy == uuid.Nil {
		return fmt.Errorf("%w: requesting actor is required", ErrInvalidReportRequest)
	}
	if err := validateReportPeriod(ReportPeriod{Start: input.From, End: input.To}); err != nil {
		return err
	}
	if err := ValidateReportFormat(input.Format); err != nil {
		return err
	}
	if err := validateRecommendationUIDs(input.RecommendationUIDs); err != nil {
		return err
	}
	return validateReportTitle(input.Title)
}

func ValidateReportFormat(format ReportFormat) error {
	switch ReportFormat(strings.ToLower(strings.TrimSpace(string(format)))) {
	case ReportFormatJSON, ReportFormatCSV, ReportFormatHTML:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrReportFormatUnsupported, format)
	}
}

func validateReportPeriod(period ReportPeriod) error {
	if period.Start.IsZero() || period.End.IsZero() || !period.Start.Before(period.End) {
		return fmt.Errorf("%w: period start must be before period end", ErrInvalidReportRequest)
	}
	if period.End.Sub(period.Start) > maxReportPeriod {
		return fmt.Errorf("%w: period cannot exceed %d days", ErrInvalidReportRequest, int(maxReportPeriod/(24*time.Hour)))
	}
	return nil
}

func validateRecommendationUIDs(recommendationUIDs []uuid.UUID) error {
	if len(recommendationUIDs) > maxExplicitReportRecommendations {
		return fmt.Errorf("%w: no more than %d recommendations may be selected", ErrInvalidReportRequest, maxExplicitReportRecommendations)
	}
	seen := make(map[uuid.UUID]struct{}, len(recommendationUIDs))
	for _, recommendationUID := range recommendationUIDs {
		if recommendationUID == uuid.Nil {
			return fmt.Errorf("%w: recommendation IDs must be non-zero UUIDs", ErrInvalidReportRequest)
		}
		if _, exists := seen[recommendationUID]; exists {
			return fmt.Errorf("%w: recommendation IDs must be unique", ErrInvalidReportRequest)
		}
		seen[recommendationUID] = struct{}{}
	}
	return nil
}

func validateReportTitle(title string) error {
	if utf8.RuneCountInString(strings.TrimSpace(title)) > maxReportTitleLength {
		return fmt.Errorf("%w: title cannot exceed %d characters", ErrInvalidReportRequest, maxReportTitleLength)
	}
	return nil
}

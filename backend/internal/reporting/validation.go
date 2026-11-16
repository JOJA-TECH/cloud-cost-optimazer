package reporting

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

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
	if err := validateReportPeriod(input.Period); err != nil {
		return err
	}
	if err := ValidateReportFormat(input.Format); err != nil {
		return err
	}
	return nil
}

func ValidateReportFormat(format ReportFormat) error {
	switch ReportFormat(strings.ToLower(strings.TrimSpace(string(format)))) {
	case ReportFormatJSON, ReportFormatCSV:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrReportFormatUnsupported, format)
	}
}

func validateReportPeriod(period ReportPeriod) error {
	if period.Start.IsZero() || period.End.IsZero() || !period.Start.Before(period.End) {
		return fmt.Errorf("%w: period start must be before period end", ErrInvalidReportRequest)
	}
	return nil
}

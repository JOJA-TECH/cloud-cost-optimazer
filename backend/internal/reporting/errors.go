package reporting

import "errors"

var (
	ErrInvalidReportRequest        = errors.New("invalid report request")
	ErrReportDependencyUnavailable = errors.New("report dependency unavailable")
	ErrReportRepository            = errors.New("report repository unavailable")
	ErrReportResourceNotFound      = errors.New("report resource not found in project")
	ErrReportNotFound              = errors.New("report not found")
	ErrReportFormatUnsupported     = errors.New("report format is unsupported")
	ErrReportGenerationFailed      = errors.New("report generation failed")
)

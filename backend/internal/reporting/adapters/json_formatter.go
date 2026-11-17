package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
)

type JSONFormatter struct{}

var _ reporting.ReportFormatter = JSONFormatter{}

func NewJSONFormatter() JSONFormatter {
	return JSONFormatter{}
}

func (JSONFormatter) Supports(format reporting.ReportFormat) bool {
	return format == reporting.ReportFormatJSON
}

func (f JSONFormatter) Format(ctx context.Context, report reporting.Report) (reporting.FormattedReport, error) {
	if err := ctx.Err(); err != nil {
		return reporting.FormattedReport{}, err
	}
	if !f.Supports(report.Format) {
		return reporting.FormattedReport{}, fmt.Errorf("%w: %q", reporting.ErrReportFormatUnsupported, report.Format)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return reporting.FormattedReport{}, fmt.Errorf("encode report as JSON: %w", err)
	}
	checksum := sha256.Sum256(body)
	return reporting.FormattedReport{
		Format:      reporting.ReportFormatJSON,
		ContentType: "application/json",
		Filename:    "report-" + report.UID.String() + ".json",
		Body:        body,
		SizeBytes:   int64(len(body)),
		Checksum:    hex.EncodeToString(checksum[:]),
	}, nil
}

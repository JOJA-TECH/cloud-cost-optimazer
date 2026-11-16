package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
)

type CSVFormatter struct{}

var _ reporting.ReportFormatter = CSVFormatter{}

func NewCSVFormatter() CSVFormatter {
	return CSVFormatter{}
}

func (CSVFormatter) Supports(format reporting.ReportFormat) bool {
	return format == reporting.ReportFormatCSV
}

func (f CSVFormatter) Format(ctx context.Context, report reporting.Report) (reporting.FormattedReport, error) {
	if err := ctx.Err(); err != nil {
		return reporting.FormattedReport{}, err
	}
	if !f.Supports(report.Format) {
		return reporting.FormattedReport{}, fmt.Errorf("%w: %q", reporting.ErrReportFormatUnsupported, report.Format)
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	header := []string{
		"report_uid", "project_id", "resource_uid", "report_type",
		"period_start", "period_end", "section_key", "section_title",
		"record_type", "code", "label", "value", "unit", "source",
		"reference", "recommendation_uid", "status", "action", "description", "details",
	}
	if err := writer.Write(header); err != nil {
		return reporting.FormattedReport{}, fmt.Errorf("write CSV header: %w", err)
	}

	resourceUID := ""
	if report.ResourceUID != nil {
		resourceUID = report.ResourceUID.String()
	}
	base := []string{
		report.UID.String(), report.ProjectID.String(), resourceUID, string(report.ReportType),
		report.Period.Start.Format("2006-01-02T15:04:05Z07:00"),
		report.Period.End.Format("2006-01-02T15:04:05Z07:00"),
	}
	for _, section := range report.Sections {
		for _, metric := range section.Metrics {
			row := append(append([]string(nil), base...), section.Key, section.Title, "metric",
				metric.Code, metric.Label, csvValue(metric.Value), metric.Unit, string(metric.Source),
				"", "", "", "", "", csvJSON(map[string]any{"period": metric.Period, "quality": metric.Quality}))
			if err := writer.Write(row); err != nil {
				return reporting.FormattedReport{}, fmt.Errorf("write CSV metric row: %w", err)
			}
		}
		for _, recommendation := range section.Recommendations {
			row := append(append([]string(nil), base...), section.Key, section.Title, "recommendation",
				"", "", "", "", string(reporting.ReportSourceRecommendation), "",
				recommendation.RecommendationUID.String(), recommendation.Status, recommendation.Action,
				recommendation.Description, csvJSON(recommendation))
			if err := writer.Write(row); err != nil {
				return reporting.FormattedReport{}, fmt.Errorf("write CSV recommendation row: %w", err)
			}
		}
		for _, evidence := range section.Evidence {
			row := append(append([]string(nil), base...), section.Key, section.Title, "evidence",
				valueOrEmpty(evidence.MetricCode), "", valueOrEmpty(evidence.Value),
				valueOrEmpty(evidence.Unit), string(evidence.Source), evidence.Reference,
				"", "", "", "", csvJSON(evidence))
			if err := writer.Write(row); err != nil {
				return reporting.FormattedReport{}, fmt.Errorf("write CSV evidence row: %w", err)
			}
		}
		if len(section.Data) > 0 {
			row := append(append([]string(nil), base...), section.Key, section.Title, "section_data",
				"", "", "", "", "", "", "", "", "", "", csvJSON(section.Data))
			if err := writer.Write(row); err != nil {
				return reporting.FormattedReport{}, fmt.Errorf("write CSV section data row: %w", err)
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return reporting.FormattedReport{}, fmt.Errorf("flush CSV report: %w", err)
	}
	body := buffer.Bytes()
	checksum := sha256.Sum256(body)
	return reporting.FormattedReport{
		Format:      reporting.ReportFormatCSV,
		ContentType: "text/csv; charset=utf-8",
		Filename:    "report-" + report.UID.String() + ".csv",
		Body:        body,
		SizeBytes:   int64(len(body)),
		Checksum:    hex.EncodeToString(checksum[:]),
	}, nil
}

func csvValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return safeSpreadsheetText(text)
	}
	return fmt.Sprint(value)
}

func csvJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return safeSpreadsheetText(fmt.Sprint(value))
	}
	return safeSpreadsheetText(string(encoded))
}

func valueOrEmpty[T any](value *T) string {
	if value == nil {
		return ""
	}
	return csvValue(*value)
}

func safeSpreadsheetText(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

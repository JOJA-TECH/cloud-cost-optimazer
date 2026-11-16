package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
)

// CSVFormatter exporta una vista tabular del reporte: una fila por
// recomendación. No intenta representar la jerarquía completa del reporte en
// una sola fila; JSON es el formato canónico para la estructura completa.
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
		"resource_uid",
		"recommendation_uid",
		"action",
		"status",
		"estimated_saving",
		"currency",
		"risk_level",
		"risk_score",
		"confidence_score",
		"created_at",
	}
	if err := writer.Write(header); err != nil {
		return reporting.FormattedReport{}, fmt.Errorf("write CSV header: %w", err)
	}

	resourceUID := ""
	if report.ResourceUID != nil {
		resourceUID = report.ResourceUID.String()
	}
	createdAt := report.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")

	for _, section := range report.Sections {
		for _, rec := range section.Recommendations {
			estimatedSaving := ""
			if rec.EstimatedSaving.Currency != "" {
				estimatedSaving = fmt.Sprintf("%.2f", rec.EstimatedSaving.MonthlyAmount)
			}
			riskLevel := ""
			riskScore := ""
			if rec.Risk != nil {
				riskLevel = rec.Risk.Level
				riskScore = fmt.Sprintf("%.2f", rec.Risk.Score)
			}
			row := []string{
				safeSpreadsheetText(resourceUID),
				safeSpreadsheetText(rec.RecommendationUID.String()),
				safeSpreadsheetText(rec.Action),
				safeSpreadsheetText(rec.Status),
				safeSpreadsheetText(estimatedSaving),
				safeSpreadsheetText(rec.EstimatedSaving.Currency),
				safeSpreadsheetText(riskLevel),
				safeSpreadsheetText(riskScore),
				safeSpreadsheetText(fmt.Sprintf("%.2f", rec.Confidence.Score)),
				safeSpreadsheetText(createdAt),
			}
			if err := writer.Write(row); err != nil {
				return reporting.FormattedReport{}, fmt.Errorf("write CSV recommendation row: %w", err)
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

func safeSpreadsheetText(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

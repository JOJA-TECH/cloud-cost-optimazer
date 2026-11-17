package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
)

// HTMLFormatter renders a Report as an HTML document. It only presents data
// already assembled by the service; it performs no new calculations.
type HTMLFormatter struct{}

var _ reporting.ReportFormatter = HTMLFormatter{}

func NewHTMLFormatter() HTMLFormatter {
	return HTMLFormatter{}
}

func (HTMLFormatter) Supports(format reporting.ReportFormat) bool {
	return format == reporting.ReportFormatHTML
}

func (f HTMLFormatter) Format(ctx context.Context, report reporting.Report) (reporting.FormattedReport, error) {
	if err := ctx.Err(); err != nil {
		return reporting.FormattedReport{}, err
	}
	if !f.Supports(report.Format) {
		return reporting.FormattedReport{}, fmt.Errorf("%w: %q", reporting.ErrReportFormatUnsupported, report.Format)
	}

	var builder strings.Builder
	builder.WriteString(`<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>` + html.EscapeString(report.Title) + `</title>
<style>
  body { font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif; margin: 24px; color: #1f2937; }
  h1 { font-size: 1.5rem; margin-bottom: 8px; }
  h2 { font-size: 1.125rem; border-bottom: 1px solid #e5e7eb; padding-bottom: 4px; margin-top: 24px; }
  table { border-collapse: collapse; width: 100%; margin-top: 8px; margin-bottom: 16px; font-size: 0.875rem; }
  th, td { border: 1px solid #d1d5db; text-align: left; padding: 6px 8px; }
  th { background: #f3f4f6; font-weight: 600; }
  tr:nth-child(even) td { background: #f9fafb; }
  .meta td:first-child, .section-data td:first-child { font-weight: 600; width: 30%; white-space: nowrap; }
  .badge { display: inline-block; padding: 2px 6px; border-radius: 999px; font-size: 0.75rem; background: #e5e7eb; }
  .footer { margin-top: 32px; font-size: 0.75rem; color: #6b7280; border-top: 1px solid #e5e7eb; padding-top: 12px; }
  .nodata { color: #6b7280; font-style: italic; }
</style>
</head>
<body>
<h1>` + html.EscapeString(report.Title) + `</h1>
`)

	builder.WriteString(`<table class="meta">
<tr><td>Tipo de reporte</td><td>` + html.EscapeString(string(report.ReportType)) + `</td></tr>
<tr><td>Formato</td><td>` + html.EscapeString(string(report.Format)) + `</td></tr>
<tr><td>Proyecto</td><td>` + report.ProjectID.String() + `</td></tr>
`)
	if report.ResourceUID != nil {
		builder.WriteString(`<tr><td>Recurso</td><td>` + report.ResourceUID.String() + `</td></tr>
`)
	}
	builder.WriteString(fmt.Sprintf(
		`<tr><td>Periodo</td><td>%s a %s</td></tr>
<tr><td>Estado</td><td><span class="badge">%s</span></td></tr>
<tr><td>Generado por</td><td>%s</td></tr>
<tr><td>Generado en</td><td>%s</td></tr>
</table>`,
		html.EscapeString(report.Period.Start.Format("2006-01-02 15:04:05 UTC")),
		html.EscapeString(report.Period.End.Format("2006-01-02 15:04:05 UTC")),
		html.EscapeString(string(report.Status)),
		html.EscapeString(report.GeneratedBy.String()),
		html.EscapeString(report.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC")),
	))

	builder.WriteString(renderDataQuality(report.DataQuality))

	sections := make([]reporting.ReportSection, len(report.Sections))
	copy(sections, report.Sections)
	sortSectionsByOrder(sections)

	for _, section := range sections {
		builder.WriteString(`<h2>` + html.EscapeString(section.Title) + `</h2>
`)
		if section.Description != "" {
			builder.WriteString(`<p>` + html.EscapeString(section.Description) + `</p>
`)
		}
		if len(section.Metrics) > 0 {
			builder.WriteString(`<table>
<tr><th>Código</th><th>Etiqueta</th><th>Valor</th><th>Unidad</th><th>Fuente</th><th>Periodo inicio</th><th>Periodo fin</th></tr>
`)
			for _, metric := range section.Metrics {
				builder.WriteString(fmt.Sprintf(
					`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>
`,
					html.EscapeString(metric.Code),
					html.EscapeString(metric.Label),
					html.EscapeString(renderAny(metric.Value)),
					html.EscapeString(metric.Unit),
					html.EscapeString(string(metric.Source)),
					html.EscapeString(metric.Period.Start.Format("2006-01-02 15:04:05")),
					html.EscapeString(metric.Period.End.Format("2006-01-02 15:04:05")),
				))
			}
			builder.WriteString(`</table>
`)
		}
		if len(section.Recommendations) > 0 {
			builder.WriteString(`<table>
<tr><th>UID recomendación</th><th>Acción</th><th>Descripción</th><th>Estado</th><th>Ahorro mensual</th><th>Riesgo</th><th>Confianza</th><th>Evidencias</th><th>Trazabilidad</th></tr>
`)
			for _, rec := range section.Recommendations {
				riskLevel := ""
				if rec.Risk != nil {
					riskLevel = rec.Risk.Level
				}
				saving := ""
				if rec.EstimatedSaving.Currency != "" {
					saving = fmt.Sprintf("%.2f %s", rec.EstimatedSaving.MonthlyAmount, rec.EstimatedSaving.Currency)
				}
				traceAvailable := "No"
				if rec.TraceAvailable {
					traceAvailable = "Sí"
				}
				builder.WriteString(fmt.Sprintf(
					`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%.2f</td><td>%d</td><td>%s</td></tr>
`,
					html.EscapeString(rec.RecommendationUID.String()),
					html.EscapeString(rec.Action),
					html.EscapeString(rec.Description),
					html.EscapeString(rec.Status),
					html.EscapeString(saving),
					html.EscapeString(riskLevel),
					rec.Confidence.Score,
					rec.EvidenceCount,
					html.EscapeString(traceAvailable),
				))
			}
			builder.WriteString(`</table>
`)
			if recHasRationale(section.Recommendations) {
				builder.WriteString(`<details><summary>Rationale de recomendaciones</summary>
`)
				for _, rec := range section.Recommendations {
					if rec.Rationale != "" {
						builder.WriteString(fmt.Sprintf(`<p><strong>%s:</strong> %s</p>
`,
							html.EscapeString(rec.RecommendationUID.String()),
							html.EscapeString(rec.Rationale)))
					}
				}
				builder.WriteString(`</details>
`)
			}
		}
		if len(section.Evidence) > 0 {
			builder.WriteString(`<table>
<tr><th>Tipo</th><th>Métrica</th><th>Valor</th><th>Unidad</th><th>Fuente</th><th>Ventana inicio</th><th>Ventana fin</th><th>Referencia</th></tr>
`)
			for _, ev := range section.Evidence {
				builder.WriteString(fmt.Sprintf(
					`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>
`,
					html.EscapeString(ev.Type),
					html.EscapeString(optionalString(ev.MetricCode)),
					html.EscapeString(optionalFloat(ev.Value)),
					html.EscapeString(optionalString(ev.Unit)),
					html.EscapeString(string(ev.Source)),
					html.EscapeString(optionalTime(ev.WindowStart)),
					html.EscapeString(optionalTime(ev.WindowEnd)),
					html.EscapeString(ev.Reference),
				))
			}
			builder.WriteString(`</table>
`)
		}
		if len(section.Data) > 0 {
			builder.WriteString(`<table class="section-data">
`)
			for key, value := range section.Data {
				builder.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td></tr>
`,
					html.EscapeString(key),
					html.EscapeString(renderAny(value)),
				))
			}
			builder.WriteString(`</table>
`)
		}
		if len(section.Metrics) == 0 && len(section.Recommendations) == 0 && len(section.Evidence) == 0 && len(section.Data) == 0 {
			builder.WriteString(`<p class="nodata">Sin datos disponibles en esta sección.</p>
`)
		}
	}

	builder.WriteString(`<div class="footer">
Reporte generado por Cloud Cost Optimizer Reporting. Los valores provienen de Analytics, Recommendation y Audit &amp; Traceability; Reporting no recalcula métricas, ahorros, riesgos ni recomendaciones.
</div>
</body>
</html>`)

	body := []byte(builder.String())
	checksum := sha256.Sum256(body)
	return reporting.FormattedReport{
		Format:      reporting.ReportFormatHTML,
		ContentType: "text/html; charset=utf-8",
		Filename:    "report-" + report.UID.String() + ".html",
		Body:        body,
		SizeBytes:   int64(len(body)),
		Checksum:    hex.EncodeToString(checksum[:]),
	}, nil
}

func renderDataQuality(quality reporting.ReportDataQuality) string {
	if quality.Status == "" {
		return ""
	}
	missing := ""
	if len(quality.Missing) > 0 {
		items := make([]string, 0, len(quality.Missing))
		for _, m := range quality.Missing {
			items = append(items, html.EscapeString(m))
		}
		missing = `<tr><td>Faltantes</td><td>` + strings.Join(items, ", ") + `</td></tr>`
	}
	return `<table class="meta">
<tr><td>Calidad de datos</td><td><span class="badge">` + html.EscapeString(quality.Status) + `</span></td></tr>
` + missing + `</table>
`
}

func sortSectionsByOrder(sections []reporting.ReportSection) {
	for i := 1; i < len(sections); i++ {
		key := sections[i]
		j := i - 1
		for j >= 0 && sections[j].Order > key.Order {
			sections[j+1] = sections[j]
			j--
		}
		sections[j+1] = key
	}
}

func recHasRationale(recommendations []reporting.ReportRecommendation) bool {
	for _, rec := range recommendations {
		if rec.Rationale != "" {
			return true
		}
	}
	return false
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%g", *value)
}

func optionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04:05")
}

func renderAny(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case bool:
		return fmt.Sprintf("%t", v)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", v)
	case time.Time:
		return v.UTC().Format(time.RFC3339)
	case *time.Time:
		if v == nil {
			return ""
		}
		return v.UTC().Format(time.RFC3339)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(encoded)
	}
}

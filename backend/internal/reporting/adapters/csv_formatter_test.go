package adapters

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

func TestCSVFormatterIncludesDataQuality(t *testing.T) {
	formatted, err := NewCSVFormatter().Format(context.Background(), reporting.Report{
		UID:    uuid.New(),
		Format: reporting.ReportFormatCSV,
		DataQuality: reporting.ReportDataQuality{
			Status:  "degraded",
			Missing: []string{"cost_analysis", "forecast"},
		},
	})
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(formatted.Body)))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read formatted CSV: %v", err)
	}
	for _, row := range rows[1:] {
		if row[9] != "data_quality" {
			continue
		}
		var quality reporting.ReportDataQuality
		if err := json.Unmarshal([]byte(row[20]), &quality); err != nil {
			t.Fatalf("decode CSV data quality: %v", err)
		}
		if quality.Status != "degraded" ||
			len(quality.Missing) != 2 ||
			quality.Missing[0] != "cost_analysis" ||
			quality.Missing[1] != "forecast" {
			t.Fatalf("CSV data quality = %#v", quality)
		}
		return
	}
	t.Fatal("CSV output does not contain a data_quality row")
}

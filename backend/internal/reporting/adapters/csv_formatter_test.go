package adapters

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

func TestCSVFormatterRecommendationTabularView(t *testing.T) {
	resourceUID := uuid.New()
	recUID := uuid.New()
	now := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	report := reporting.Report{
		UID:         uuid.New(),
		ResourceUID: &resourceUID,
		ReportType:  reporting.ReportTypeOptimization,
		Format:      reporting.ReportFormatCSV,
		CreatedAt:   now,
		Sections: []reporting.ReportSection{
			{
				Key:   "recommendations",
				Title: "Recomendaciones de optimización",
				Recommendations: []reporting.ReportRecommendation{
					{
						RecommendationUID: recUID,
						Action:            "resize_down",
						Status:            "accepted",
						EstimatedSaving: reporting.EstimatedSavingView{
							MonthlyAmount: 125.50,
							Currency:      "USD",
						},
						Risk: &reporting.RiskView{
							Level: "medium",
							Score: 0.45,
						},
						Confidence: reporting.ConfidenceView{Score: 0.82},
					},
					{
						RecommendationUID: uuid.New(),
						Action:            "resize_down",
						Status:            "pending",
						EstimatedSaving: reporting.EstimatedSavingView{
							MonthlyAmount: 50.0,
							Currency:      "USD",
						},
						Confidence: reporting.ConfidenceView{Score: 0.9},
					},
				},
			},
		},
	}

	formatted, err := NewCSVFormatter().Format(context.Background(), report)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(formatted.Body)))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read formatted CSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (header + 2 recommendations), got %d", len(rows))
	}
	wantHeader := []string{
		"resource_uid", "recommendation_uid", "action", "status",
		"estimated_saving", "currency", "risk_level", "risk_score",
		"confidence_score", "created_at",
	}
	for i, col := range wantHeader {
		if rows[0][i] != col {
			t.Fatalf("header column %d = %q, want %q", i, rows[0][i], col)
		}
	}
	// Primera fila de recomendación.
	if rows[1][1] != recUID.String() ||
		rows[1][2] != "resize_down" ||
		rows[1][3] != "accepted" ||
		rows[1][4] != "125.50" ||
		rows[1][5] != "USD" ||
		rows[1][6] != "medium" ||
		rows[1][7] != "0.45" ||
		rows[1][8] != "0.82" ||
		rows[1][9] != "2026-01-15T10:30:00Z" {
		t.Fatalf("first recommendation row = %#v", rows[1])
	}
	if rows[1][0] != resourceUID.String() {
		t.Fatalf("resource_uid = %q, want %q", rows[1][0], resourceUID.String())
	}
}

func TestCSVFormatterNoRecommendations(t *testing.T) {
	formatted, err := NewCSVFormatter().Format(context.Background(), reporting.Report{
		UID:    uuid.New(),
		Format: reporting.ReportFormatCSV,
	})
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(formatted.Body)))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read formatted CSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only header row, got %d rows", len(rows))
	}
	if string(formatted.Body) == "" {
		t.Fatal("CSV body is empty")
	}
}

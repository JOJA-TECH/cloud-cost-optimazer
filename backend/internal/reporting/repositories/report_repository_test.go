package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

func TestReportRepositoryRejectsMissingDatabase(t *testing.T) {
	repository := NewReportRepository(nil)
	ctx := context.Background()

	if _, err := repository.CreateJob(ctx, reporting.ReportGenerationJob{}); !errors.Is(err, reporting.ErrReportRepository) {
		t.Fatalf("CreateJob() error = %v, want ErrReportRepository", err)
	}
	if err := repository.UpdateJob(ctx, reporting.ReportGenerationJob{}); !errors.Is(err, reporting.ErrReportRepository) {
		t.Fatalf("UpdateJob() error = %v, want ErrReportRepository", err)
	}
	if _, err := repository.GetJob(ctx, uuid.New()); !errors.Is(err, reporting.ErrReportRepository) {
		t.Fatalf("GetJob() error = %v, want ErrReportRepository", err)
	}
	if _, err := repository.SaveArtifact(ctx, uuid.New(), reporting.ReportArtifact{}); !errors.Is(err, reporting.ErrReportRepository) {
		t.Fatalf("SaveArtifact() error = %v, want ErrReportRepository", err)
	}
	if _, err := repository.GetArtifact(ctx, uuid.New()); !errors.Is(err, reporting.ErrReportRepository) {
		t.Fatalf("GetArtifact() error = %v, want ErrReportRepository", err)
	}
}

func TestResourceProjectRepositoryRejectsMissingDatabase(t *testing.T) {
	repository := NewResourceProjectRepository(nil)
	_, err := repository.IsResourceInProject(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, reporting.ErrReportRepository) {
		t.Fatalf("IsResourceInProject() error = %v, want ErrReportRepository", err)
	}
}

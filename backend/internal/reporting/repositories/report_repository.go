package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReportRepository struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) *ReportRepository {
	return &ReportRepository{db: db}
}

var _ reporting.ReportRepository = (*ReportRepository)(nil)

func (r *ReportRepository) CreateJob(ctx context.Context, job reporting.ReportGenerationJob) (reporting.ReportGenerationJob, error) {
	if r == nil || r.db == nil {
		return reporting.ReportGenerationJob{}, reporting.ErrReportRepository
	}
	if job.UID == uuid.Nil {
		job.UID = uuid.New()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	if job.Status == "" {
		job.Status = reporting.ReportJobStatusQueued
	}
	parameters, err := json.Marshal(nonNilParameters(job.Parameters))
	if err != nil {
		return reporting.ReportGenerationJob{}, fmt.Errorf("encode report parameters: %w", err)
	}
	model := database.ReportRequestModel{
		ReportRequestID: job.UID,
		ProjectID:       job.ProjectID,
		RequestedBy:     job.RequestedBy,
		ReportType:      string(job.ReportType),
		Parameters:      parameters,
		Status:          string(job.Status),
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		CompletedAt:     job.CompletedAt,
		ErrorMessage:    job.ErrorMessage,
	}
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "app", "report_requests")).
		Create(&model).Error; err != nil {
		return reporting.ReportGenerationJob{}, fmt.Errorf("create report request: %w", err)
	}
	job.ReportUID = uuidPointer(job.UID)
	return job, nil
}

func (r *ReportRepository) UpdateJob(ctx context.Context, job reporting.ReportGenerationJob) error {
	if r == nil || r.db == nil {
		return reporting.ErrReportRepository
	}
	if job.UID == uuid.Nil || job.Status == "" {
		return reporting.ErrInvalidReportRequest
	}
	result := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "app", "report_requests")).
		Where("report_request_id = ?", job.UID).
		Updates(map[string]any{
			"status":        string(job.Status),
			"started_at":    job.StartedAt,
			"completed_at":  job.CompletedAt,
			"error_message": job.ErrorMessage,
		})
	if result.Error != nil {
		return fmt.Errorf("update report request: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return reporting.ErrReportNotFound
	}
	return nil
}

func (r *ReportRepository) GetJob(ctx context.Context, jobUID uuid.UUID) (reporting.ReportGenerationJob, error) {
	if r == nil || r.db == nil {
		return reporting.ReportGenerationJob{}, reporting.ErrReportRepository
	}
	var model database.ReportRequestModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "app", "report_requests")).
		Where("report_request_id = ?", jobUID).
		Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reporting.ReportGenerationJob{}, reporting.ErrReportNotFound
	}
	if err != nil {
		return reporting.ReportGenerationJob{}, fmt.Errorf("get report request: %w", err)
	}
	var parameters map[string]any
	if err := json.Unmarshal(model.Parameters, &parameters); err != nil {
		return reporting.ReportGenerationJob{}, fmt.Errorf("decode report parameters: %w", err)
	}
	if parameters == nil {
		parameters = make(map[string]any)
	}
	job := reporting.ReportGenerationJob{
		UID:          model.ReportRequestID,
		ProjectID:    model.ProjectID,
		RequestedBy:  model.RequestedBy,
		ReportType:   reporting.ReportType(model.ReportType),
		Parameters:   parameters,
		Status:       reporting.ReportJobStatus(model.Status),
		ReportUID:    uuidPointer(model.ReportRequestID),
		CreatedAt:    model.CreatedAt,
		StartedAt:    model.StartedAt,
		CompletedAt:  model.CompletedAt,
		ErrorMessage: model.ErrorMessage,
	}
	var artifact database.ReportArtifactModel
	err = r.db.WithContext(ctx).
		Table(database.TableName(r.db, "app", "report_artifacts")).
		Where("report_request_id = ?", jobUID).
		Order("created_at DESC").
		Take(&artifact).Error
	if err == nil {
		job.ArtifactUID = uuidPointer(artifact.ReportArtifactID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return reporting.ReportGenerationJob{}, fmt.Errorf("get report artifact reference: %w", err)
	}
	return job, nil
}

func (r *ReportRepository) SaveArtifact(
	ctx context.Context,
	reportRequestUID uuid.UUID,
	artifact reporting.ReportArtifact,
) (reporting.ReportArtifact, error) {
	if r == nil || r.db == nil {
		return reporting.ReportArtifact{}, reporting.ErrReportRepository
	}
	if reportRequestUID == uuid.Nil || artifact.StorageURI == "" || artifact.MIMEType == "" {
		return reporting.ReportArtifact{}, reporting.ErrInvalidReportRequest
	}
	if artifact.UID == uuid.Nil {
		artifact.UID = uuid.New()
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	}
	model := database.ReportArtifactModel{
		ReportArtifactID: artifact.UID,
		ReportRequestID:  reportRequestUID,
		StorageURI:       artifact.StorageURI,
		MimeType:         artifact.MIMEType,
		FileSizeBytes:    artifact.FileSizeBytes,
		ChecksumSHA256:   artifact.ChecksumSHA256,
		CreatedAt:        artifact.CreatedAt,
		ExpiresAt:        artifact.ExpiresAt,
	}
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "app", "report_artifacts")).
		Create(&model).Error; err != nil {
		return reporting.ReportArtifact{}, fmt.Errorf("save report artifact: %w", err)
	}
	return artifact, nil
}

func (r *ReportRepository) GetArtifact(ctx context.Context, artifactUID uuid.UUID) (reporting.ReportArtifact, error) {
	if r == nil || r.db == nil {
		return reporting.ReportArtifact{}, reporting.ErrReportRepository
	}
	var model database.ReportArtifactModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "app", "report_artifacts")).
		Where("report_artifact_id = ?", artifactUID).
		Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reporting.ReportArtifact{}, reporting.ErrReportNotFound
	}
	if err != nil {
		return reporting.ReportArtifact{}, fmt.Errorf("get report artifact: %w", err)
	}
	return reporting.ReportArtifact{
		UID:            model.ReportArtifactID,
		StorageURI:     model.StorageURI,
		MIMEType:       model.MimeType,
		FileSizeBytes:  model.FileSizeBytes,
		ChecksumSHA256: model.ChecksumSHA256,
		CreatedAt:      model.CreatedAt,
		ExpiresAt:      model.ExpiresAt,
	}, nil
}

func nonNilParameters(parameters map[string]any) map[string]any {
	if parameters != nil {
		return parameters
	}
	return map[string]any{}
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	return &value
}

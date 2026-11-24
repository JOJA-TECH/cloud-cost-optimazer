package repositories

import (
	"context"
	"fmt"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ResourceProjectRepository struct {
	db *gorm.DB
}

func NewResourceProjectRepository(db *gorm.DB) *ResourceProjectRepository {
	return &ResourceProjectRepository{db: db}
}

var _ reporting.ReportResourceScope = (*ResourceProjectRepository)(nil)

func (r *ResourceProjectRepository) IsResourceInProject(
	ctx context.Context,
	projectID, resourceUID uuid.UUID,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, reporting.ErrReportRepository
	}
	var count int64
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resource_bindings")).
		Where("project_id = ? AND data_resource_uid = ?", projectID, resourceUID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check resource project binding: %w", err)
	}
	return count > 0, nil
}

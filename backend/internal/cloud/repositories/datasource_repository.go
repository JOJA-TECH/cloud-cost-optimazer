package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type datasourceRepository struct{ db *gorm.DB }

func NewDataSourceRepository(db *gorm.DB) cloud.DataSourceRepository {
	return &datasourceRepository{db: db}
}

func (r *datasourceRepository) CreateBinding(ctx context.Context, input cloud.CreateBindingInput) (cloud.BindingDTO, error) {
	// Guard against duplicate bindings. The DB unique constraints enforce this
	// in Postgres; these checks cover SQLite dev/tests where GORM AutoMigrate
	// does not create composite unique indexes.
	dupErr := cloud.ErrBindingExists
	checkUnique := func(cols string, vals ...interface{}) error {
		var existing database.ResourceBindingModel
		err := r.db.WithContext(ctx).
			Table(database.TableName(r.db, "cloud", "resource_bindings")).
			Where(cols, vals...).
			First(&existing).Error
		if err == nil {
			return dupErr
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return nil
	}
	if err := checkUnique("project_id = ? AND data_resource_uid = ?", input.ProjectID, input.DataResourceUID); err != nil {
		return cloud.BindingDTO{}, err
	}
	if err := checkUnique("cloud_account_id = ? AND data_resource_uid = ?", input.CloudAccountID, input.DataResourceUID); err != nil {
		return cloud.BindingDTO{}, err
	}

	now := time.Now().UTC()
	model := database.ResourceBindingModel{
		ResourceBindingID: uuid.New(),
		ProjectID:         input.ProjectID,
		CloudAccountID:    input.CloudAccountID,
		DataResourceUID:   input.DataResourceUID,
		DisplayName:       strPtr(input.DisplayName),
		IsMonitored:       true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resource_bindings")).
		Create(&model).Error
	if err != nil {
		if isUniqueViolation(err) {
			return cloud.BindingDTO{}, cloud.ErrBindingExists
		}
		return cloud.BindingDTO{}, err
	}
	return toBindingDTO(model), nil
}

func (r *datasourceRepository) GetBinding(ctx context.Context, id uuid.UUID) (cloud.BindingDTO, error) {
	var model database.ResourceBindingModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resource_bindings")).
		Where("resource_binding_id = ?", id).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return cloud.BindingDTO{}, cloud.ErrBindingNotFound
		}
		return cloud.BindingDTO{}, err
	}
	return toBindingDTO(model), nil
}

func (r *datasourceRepository) ListBindings(ctx context.Context, projectID *uuid.UUID) ([]cloud.BindingDTO, error) {
	var models []database.ResourceBindingModel
	q := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resource_bindings"))
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	err := q.Order("created_at DESC").Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]cloud.BindingDTO, len(models))
	for i, m := range models {
		dtos[i] = toBindingDTO(m)
	}
	return dtos, nil
}

func (r *datasourceRepository) SetBindingMonitored(ctx context.Context, id uuid.UUID, monitored bool) error {
	updates := map[string]interface{}{
		"is_monitored": monitored,
		"updated_at":   time.Now().UTC(),
	}
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resource_bindings")).
		Where("resource_binding_id = ?", id).
		Updates(updates).Error
	if err != nil {
		return err
	}
	return nil
}

// ProjectOrgID returns the organization that owns a project. Not found ->
// ErrProjectNotFound so callers can map it to 404 (never reveal the project
// exists in another org).
func (r *datasourceRepository) ProjectOrgID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	var project database.ProjectModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "projects")).
		Where("project_id = ?", projectID).
		First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, cloud.ErrProjectNotFound
		}
		return uuid.Nil, err
	}
	return project.OrganizationID, nil
}

func toBindingDTO(m database.ResourceBindingModel) cloud.BindingDTO {
	return cloud.BindingDTO{
		ResourceBindingID: m.ResourceBindingID,
		ProjectID:         m.ProjectID,
		CloudAccountID:    m.CloudAccountID,
		DataResourceUID:   m.DataResourceUID,
		DisplayName:       strVal(m.DisplayName),
		IsMonitored:       m.IsMonitored,
		CreatedAt:         m.CreatedAt.UnixMilli(),
		UpdatedAt:         m.UpdatedAt.UnixMilli(),
	}
}

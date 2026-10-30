package repositories

import (
	"context"
	"errors"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type projectRepository struct{ db *gorm.DB }

func NewProjectRepository(db *gorm.DB) iam.ProjectRepository {
	return &projectRepository{db: db}
}

func (r *projectRepository) FindProjectByID(ctx context.Context, id uuid.UUID) (iam.ProjectDTO, error) {
	var model database.ProjectModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "projects")).
		Where("project_id = ?", id).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return iam.ProjectDTO{}, iam.ErrProjectNotFound
		}
		return iam.ProjectDTO{}, err
	}
	return toProjectDTO(model), nil
}

func (r *projectRepository) ListProjectsByOrg(ctx context.Context, orgID uuid.UUID) ([]iam.ProjectDTO, error) {
	var models []database.ProjectModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "projects")).
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.ProjectDTO, len(models))
	for i, m := range models {
		dtos[i] = toProjectDTO(m)
	}
	return dtos, nil
}

// CheckUserProjectAccess reports whether the user is an active member of the
// project's organization. Report 1.0 has no per-project membership table yet.
// shortcut: project access = org membership; upgrade: add project_members table
// when per-project ACLs become a requirement.
func (r *projectRepository) CheckUserProjectAccess(ctx context.Context, userID, projectID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "projects")).
		Joins("JOIN "+database.TableName(r.db, "organization", "members")+" ON members.organization_id = projects.organization_id").
		Where("projects.project_id = ? AND members.user_id = ? AND members.status = ?", projectID, userID, "active").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func toProjectDTO(m database.ProjectModel) iam.ProjectDTO {
	var desc *string
	if m.Description != nil {
		d := *m.Description
		desc = &d
	}
	return iam.ProjectDTO{
		ProjectID:      m.ProjectID,
		OrganizationID: m.OrganizationID,
		Name:           m.Name,
		Description:    desc,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt.UnixMilli(),
		UpdatedAt:      m.UpdatedAt.UnixMilli(),
	}
}

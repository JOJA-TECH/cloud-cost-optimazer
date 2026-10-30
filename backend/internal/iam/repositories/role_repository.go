package repositories

import (
	"context"
	"errors"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"gorm.io/gorm"
)

type roleRepository struct{ db *gorm.DB }

func NewRoleRepository(db *gorm.DB) iam.RoleRepository {
	return &roleRepository{db: db}
}

func (r *roleRepository) FindRoleByCode(ctx context.Context, code string) (iam.RoleDTO, error) {
	var model database.RoleModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "roles")).
		Where("role_code = ?", code).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return iam.RoleDTO{}, iam.ErrRoleNotFound
		}
		return iam.RoleDTO{}, err
	}
	return toRoleDTO(model), nil
}

func (r *roleRepository) FindRoleByID(ctx context.Context, id int16) (iam.RoleDTO, error) {
	var model database.RoleModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "roles")).
		Where("role_id = ?", id).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return iam.RoleDTO{}, iam.ErrRoleNotFound
		}
		return iam.RoleDTO{}, err
	}
	return toRoleDTO(model), nil
}

func (r *roleRepository) ListRoles(ctx context.Context) ([]iam.RoleDTO, error) {
	var models []database.RoleModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "roles")).
		Order("role_id").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.RoleDTO, len(models))
	for i, m := range models {
		dtos[i] = toRoleDTO(m)
	}
	return dtos, nil
}

func toRoleDTO(m database.RoleModel) iam.RoleDTO {
	var desc *string
	if m.Description != nil {
		d := *m.Description
		desc = &d
	}
	return iam.RoleDTO{
		RoleID:       m.RoleID,
		RoleCode:     m.RoleCode,
		Description:  desc,
		IsSystemRole: m.IsSystemRole,
		CreatedAt:    m.CreatedAt.UnixMilli(),
	}
}

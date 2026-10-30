package repositories

import (
	"context"
	"errors"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"gorm.io/gorm"
)

type permissionRepository struct{ db *gorm.DB }

func NewPermissionRepository(db *gorm.DB) iam.PermissionRepository {
	return &permissionRepository{db: db}
}

func (r *permissionRepository) FindPermissionByCode(ctx context.Context, code string) (iam.PermissionDTO, error) {
	var model database.PermissionModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "permissions")).
		Where("permission_code = ?", code).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return iam.PermissionDTO{}, iam.ErrPermissionNotFound
		}
		return iam.PermissionDTO{}, err
	}
	return toPermissionDTO(model), nil
}

func (r *permissionRepository) FindPermissionByID(ctx context.Context, id int16) (iam.PermissionDTO, error) {
	var model database.PermissionModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "permissions")).
		Where("permission_id = ?", id).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return iam.PermissionDTO{}, iam.ErrPermissionNotFound
		}
		return iam.PermissionDTO{}, err
	}
	return toPermissionDTO(model), nil
}

func (r *permissionRepository) ListPermissions(ctx context.Context) ([]iam.PermissionDTO, error) {
	var models []database.PermissionModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "permissions")).
		Order("permission_id").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.PermissionDTO, len(models))
	for i, m := range models {
		dtos[i] = toPermissionDTO(m)
	}
	return dtos, nil
}

func (r *permissionRepository) ListPermissionsByRole(ctx context.Context, roleID int16) ([]iam.PermissionDTO, error) {
	var models []database.PermissionModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "permissions")).
		Joins("JOIN "+database.TableName(r.db, "iam", "role_permissions")+" ON permission_id = permissions.permission_id").
		Where("role_id = ?", roleID).
		Order("permission_id").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.PermissionDTO, len(models))
	for i, m := range models {
		dtos[i] = toPermissionDTO(m)
	}
	return dtos, nil
}

func toPermissionDTO(m database.PermissionModel) iam.PermissionDTO {
	var desc *string
	if m.Description != nil {
		d := *m.Description
		desc = &d
	}
	return iam.PermissionDTO{
		PermissionID:   m.PermissionID,
		PermissionCode: m.PermissionCode,
		Description:    desc,
		CreatedAt:      m.CreatedAt.UnixMilli(),
	}
}

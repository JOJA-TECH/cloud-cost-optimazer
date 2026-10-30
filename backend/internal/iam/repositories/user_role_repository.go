package repositories

import (
	"context"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type userRoleRepository struct{ db *gorm.DB }

func NewUserRoleRepository(db *gorm.DB) iam.UserRoleRepository {
	return &userRoleRepository{db: db}
}

func (r *userRoleRepository) AssignRole(ctx context.Context, userID uuid.UUID, roleID int16, assignedBy *uuid.UUID) error {
	now := time.Now().UTC()
	model := database.UserRoleModel{
		UserID:     userID,
		RoleID:     roleID,
		AssignedAt: now,
		AssignedBy: assignedBy,
	}
	return r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "user_roles")).
		Create(&model).Error
}

func (r *userRoleRepository) RevokeRole(ctx context.Context, userID uuid.UUID, roleID int16) error {
	return r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "user_roles")).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Delete(&database.UserRoleModel{}).Error
}

func (r *userRoleRepository) ListUserRolesByUser(ctx context.Context, userID uuid.UUID) ([]iam.UserRoleDTO, error) {
	var models []database.UserRoleModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "user_roles")).
		Where("user_id = ?", userID).
		Order("assigned_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.UserRoleDTO, len(models))
	for i, m := range models {
		dtos[i] = toUserRoleDTO(m)
	}
	return dtos, nil
}

func (r *userRoleRepository) ListUserRolesByRole(ctx context.Context, roleID int16) ([]iam.UserRoleDTO, error) {
	var models []database.UserRoleModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "user_roles")).
		Where("role_id = ?", roleID).
		Order("assigned_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.UserRoleDTO, len(models))
	for i, m := range models {
		dtos[i] = toUserRoleDTO(m)
	}
	return dtos, nil
}

func toUserRoleDTO(m database.UserRoleModel) iam.UserRoleDTO {
	return iam.UserRoleDTO{
		UserID:     m.UserID,
		RoleID:     m.RoleID,
		AssignedAt: m.AssignedAt.UnixMilli(),
		AssignedBy: m.AssignedBy,
	}
}

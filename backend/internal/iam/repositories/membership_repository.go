package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type membershipRepository struct{ db *gorm.DB }

func NewMembershipRepository(db *gorm.DB) iam.MembershipRepository {
	return &membershipRepository{db: db}
}

var validMembershipRoles = map[string]bool{"owner": true, "admin": true, "member": true, "viewer": true}

func (r *membershipRepository) AddMember(ctx context.Context, input iam.AddMemberInput) (iam.MembershipDTO, error) {
	if !validMembershipRoles[input.MembershipRole] {
		return iam.MembershipDTO{}, iam.ErrInvalidMembershipRole
	}

	now := time.Now().UTC()
	model := database.MemberModel{
		OrganizationID: input.OrgID,
		UserID:         input.UserID,
		MembershipRole: input.MembershipRole,
		Status:         "active",
		JoinedAt:       now,
		InvitedBy:      input.InvitedBy,
	}
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "members")).
		Create(&model).Error
	if err != nil {
		return iam.MembershipDTO{}, err
	}
	return toMembershipDTO(model), nil
}

func (r *membershipRepository) UpdateMember(ctx context.Context, orgID, userID uuid.UUID, input iam.UpdateMemberInput) (iam.MembershipDTO, error) {
	updates := make(map[string]interface{})
	if input.MembershipRole != nil {
		if !validMembershipRoles[*input.MembershipRole] {
			return iam.MembershipDTO{}, iam.ErrInvalidMembershipRole
		}
		updates["membership_role"] = *input.MembershipRole
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}
	if len(updates) == 0 {
		return r.FindMembership(ctx, orgID, userID)
	}

	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "members")).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Updates(updates).Error
	if err != nil {
		return iam.MembershipDTO{}, err
	}
	return r.FindMembership(ctx, orgID, userID)
}

// RemoveMember deletes a membership. An owner cannot be removed while being
// the last active owner of the organization.
func (r *membershipRepository) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	mem, err := r.FindMembership(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if mem.MembershipRole == "owner" {
		var count int64
		err := r.db.WithContext(ctx).
			Table(database.TableName(r.db, "organization", "members")).
			Where("organization_id = ? AND membership_role = ? AND status = ?", orgID, "owner", "active").
			Count(&count).Error
		if err != nil {
			return err
		}
		if count <= 1 {
			return iam.ErrLastOwner
		}
	}

	return r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "members")).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Delete(&database.MemberModel{}).Error
}

func (r *membershipRepository) ListMembershipsByOrg(ctx context.Context, orgID uuid.UUID) ([]iam.MembershipDTO, error) {
	var models []database.MemberModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "members")).
		Where("organization_id = ?", orgID).
		Order("joined_at").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.MembershipDTO, len(models))
	for i, m := range models {
		dtos[i] = toMembershipDTO(m)
	}
	return dtos, nil
}

func (r *membershipRepository) ListMembershipsByUser(ctx context.Context, userID uuid.UUID) ([]iam.MembershipDTO, error) {
	var models []database.MemberModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "members")).
		Where("user_id = ?", userID).
		Order("joined_at").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.MembershipDTO, len(models))
	for i, m := range models {
		dtos[i] = toMembershipDTO(m)
	}
	return dtos, nil
}

func (r *membershipRepository) FindMembership(ctx context.Context, orgID, userID uuid.UUID) (iam.MembershipDTO, error) {
	var model database.MemberModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "organization", "members")).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return iam.MembershipDTO{}, iam.ErrNotMember
		}
		return iam.MembershipDTO{}, err
	}
	return toMembershipDTO(model), nil
}

func toMembershipDTO(m database.MemberModel) iam.MembershipDTO {
	return iam.MembershipDTO{
		OrgID:          m.OrganizationID,
		UserID:         m.UserID,
		MembershipRole: m.MembershipRole,
		Status:         m.Status,
		JoinedAt:       m.JoinedAt.UnixMilli(),
		InvitedBy:      m.InvitedBy,
	}
}

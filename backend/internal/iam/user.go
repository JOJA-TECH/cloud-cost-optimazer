package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type userRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) CreateUser(ctx context.Context, input CreateUserInput) (UserDTO, error) {
	if input.Email == "" || input.DisplayName == "" {
		return UserDTO{}, errors.New("email and display name are required")
	}
	// shortcut: no unique constraint on email in dev SQLite; checking here
	// prevents duplicates at the application layer. Upgrade: add a unique
	// index on iam.users(email) and let the DB reject duplicates.
	var existing database.UserModel
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users")).
		Where("email = ?", input.Email).First(&existing).Error; err == nil {
		return UserDTO{}, ErrUserExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return UserDTO{}, fmt.Errorf("check existing user: %w", err)
	}

	now := time.Now().UTC()
	model := database.UserModel{
		UserID:       uuid.New(),
		Email:        input.Email,
		DisplayName:  input.DisplayName,
		Status:       "active",
		PasswordHash: input.PasswordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users")).Create(&model).Error; err != nil {
		return UserDTO{}, fmt.Errorf("create user: %w", err)
	}
	return toDTO(model), nil
}

func (r *userRepository) FindUserByID(ctx context.Context, id uuid.UUID) (UserDTO, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users")).Where("user_id = ?", id).First(&model).Error; err != nil {
		return UserDTO{}, normalizeNotFound(err)
	}
	return toDTO(model), nil
}

func (r *userRepository) FindUserByEmail(ctx context.Context, email string) (UserDTO, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users")).Where("email = ?", email).First(&model).Error; err != nil {
		return UserDTO{}, normalizeNotFound(err)
	}
	return toDTO(model), nil
}

func (r *userRepository) FindPasswordHash(ctx context.Context, userID uuid.UUID) (string, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "users")).
		Select("password_hash").
		Where("user_id = ?", userID).
		First(&model).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	if model.PasswordHash == nil {
		return "", errors.New("no password hash")
	}
	return *model.PasswordHash, nil
}

func (r *userRepository) UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (UserDTO, error) {
	updates := make(map[string]interface{})
	if input.DisplayName != nil {
		updates["display_name"] = *input.DisplayName
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}
	if input.EmailVerified != nil {
		updates["email_verified"] = *input.EmailVerified
	}
	if len(updates) == 0 {
		return r.FindUserByID(ctx, id)
	}
	updates["updated_at"] = time.Now().UTC()

	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "users")).
		Where("user_id = ?", id).
		Updates(updates).Error
	if err != nil {
		return UserDTO{}, err
	}
	return r.FindUserByID(ctx, id)
}

func (r *userRepository) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "users")).
		Where("user_id = ?", id).
		Delete(&database.UserModel{}).Error
}

func (r *userRepository) ListUsers(ctx context.Context, filter UserListFilter) ([]UserDTO, error) {
	var models []database.UserModel
	query := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users"))

	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.OrgID != nil {
		query = query.Joins("JOIN "+database.TableName(r.db, "organization", "members")+" ON members.user_id = users.user_id").
			Where("members.organization_id = ?", *filter.OrgID)
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	err := query.Order("created_at DESC").Find(&models).Error
	if err != nil {
		return nil, err
	}

	dtos := make([]UserDTO, len(models))
	for i, m := range models {
		dtos[i] = toDTO(m)
	}
	return dtos, nil
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("user not found: %w", err)
	}
	return err
}

func toDTO(m database.UserModel) UserDTO {
	var lastLogin *int64
	if m.LastLoginAt != nil {
		v := m.LastLoginAt.UnixMilli()
		lastLogin = &v
	}
	return UserDTO{
		ID:            m.UserID,
		Email:         m.Email,
		DisplayName:   m.DisplayName,
		Status:        m.Status,
		EmailVerified: m.EmailVerified,
		LastLoginAt:   lastLogin,
		CreatedAt:     m.CreatedAt.UnixMilli(),
		UpdatedAt:     m.UpdatedAt.UnixMilli(),
	}
}

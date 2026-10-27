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

// UserDTO is the application contract. It deliberately has no GORM tags.
type UserDTO struct {
	ID            uuid.UUID
	Email         string
	DisplayName   string
	Status        string
	EmailVerified bool
	LastLoginAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateUserInput struct {
	Email       string
	DisplayName string
}

type UserRepository interface {
	Create(ctx context.Context, input CreateUserInput) (UserDTO, error)
	FindByID(ctx context.Context, id uuid.UUID) (UserDTO, error)
	FindByEmail(ctx context.Context, email string) (UserDTO, error)
}

type userRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, input CreateUserInput) (UserDTO, error) {
	if input.Email == "" || input.DisplayName == "" {
		return UserDTO{}, errors.New("email and display name are required")
	}
	now := time.Now().UTC()
	model := database.UserModel{UserID: uuid.New(), Email: input.Email, DisplayName: input.DisplayName, Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users")).Create(&model).Error; err != nil {
		return UserDTO{}, fmt.Errorf("create user: %w", err)
	}
	return toDTO(model), nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (UserDTO, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users")).Where("user_id = ?", id).First(&model).Error; err != nil {
		return UserDTO{}, normalizeNotFound(err)
	}
	return toDTO(model), nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (UserDTO, error) {
	var model database.UserModel
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "iam", "users")).Where("email = ?", email).First(&model).Error; err != nil {
		return UserDTO{}, normalizeNotFound(err)
	}
	return toDTO(model), nil
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("user not found: %w", err)
	}
	return err
}
func toDTO(m database.UserModel) UserDTO {
	return UserDTO{ID: m.UserID, Email: m.Email, DisplayName: m.DisplayName, Status: m.Status, EmailVerified: m.EmailVerified, LastLoginAt: m.LastLoginAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

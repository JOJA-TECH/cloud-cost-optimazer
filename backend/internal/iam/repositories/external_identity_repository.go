package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type externalIdentityRepository struct{ db *gorm.DB }

func NewExternalIdentityRepository(db *gorm.DB) iam.ExternalIdentityRepository {
	return &externalIdentityRepository{db: db}
}

func (r *externalIdentityRepository) LinkExternalIdentity(ctx context.Context, input iam.CreateExternalIdentityInput) (iam.ExternalIdentityDTO, error) {
	table := database.TableName(r.db, "iam", "external_identities")

	var existing database.ExternalIdentityModel
	err := r.db.WithContext(ctx).
		Table(table).
		Where("provider = ? AND subject = ?", input.Provider, input.Subject).
		Take(&existing).Error
	if err == nil {
		return toExternalIdentityDTO(existing), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return iam.ExternalIdentityDTO{}, err
	}

	model := database.ExternalIdentityModel{
		ExternalIdentityID: uuid.New(),
		UserID:             input.UserID,
		Provider:           input.Provider,
		Subject:            input.Subject,
		CreatedAt:          time.Now().UTC(),
	}
	err = r.db.WithContext(ctx).
		Table(table).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "provider"}, {Name: "subject"}},
			DoNothing: true,
		}).
		Create(&model).Error
	if err != nil {
		return iam.ExternalIdentityDTO{}, err
	}
	if err := r.db.WithContext(ctx).
		Table(table).
		Where("provider = ? AND subject = ?", input.Provider, input.Subject).
		Take(&existing).Error; err != nil {
		return iam.ExternalIdentityDTO{}, err
	}
	return toExternalIdentityDTO(existing), nil
}

func (r *externalIdentityRepository) FindExternalIdentityByProviderSubject(ctx context.Context, provider, subject string) (iam.ExternalIdentityDTO, error) {
	var model database.ExternalIdentityModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "external_identities")).
		Where("provider = ? AND subject = ?", provider, subject).
		First(&model).Error
	if err != nil {
		return iam.ExternalIdentityDTO{}, err
	}
	return toExternalIdentityDTO(model), nil
}

func (r *externalIdentityRepository) FindExternalIdentitiesByUser(ctx context.Context, userID uuid.UUID) ([]iam.ExternalIdentityDTO, error) {
	var models []database.ExternalIdentityModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "external_identities")).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]iam.ExternalIdentityDTO, len(models))
	for i, m := range models {
		dtos[i] = toExternalIdentityDTO(m)
	}
	return dtos, nil
}

func (r *externalIdentityRepository) UpdateExternalIdentityLastSeen(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Table(database.TableName(r.db, "iam", "external_identities")).
		Where("external_identity_id = ?", id).
		Update("last_seen_at", now).Error
}

func toExternalIdentityDTO(m database.ExternalIdentityModel) iam.ExternalIdentityDTO {
	return iam.ExternalIdentityDTO{
		ExternalIdentityID: m.ExternalIdentityID,
		UserID:             m.UserID,
		Provider:           m.Provider,
		Subject:            m.Subject,
		CreatedAt:          m.CreatedAt.UnixMilli(),
		LastSeenAt:         ptrTimeMilli(m.LastSeenAt),
	}
}

func ptrTimeMilli(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.UnixMilli()
	return &v
}

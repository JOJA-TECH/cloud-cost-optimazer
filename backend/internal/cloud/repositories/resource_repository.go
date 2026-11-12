package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type resourceRepository struct{ db *gorm.DB }

func NewResourceRepository(db *gorm.DB) cloud.ResourceRepository {
	return &resourceRepository{db: db}
}

func (r *resourceRepository) UpsertResources(ctx context.Context, accountID uuid.UUID, resources []cloud.ResourceDTO) ([]cloud.ResourceDTO, error) {
	now := time.Now().UTC()
	models := make([]database.ResourceModel, 0, len(resources))
	for _, res := range resources {
		uid := res.ResourceUID
		if uid == uuid.Nil {
			uid = uuid.New()
		}
		kind := res.Kind
		var kindPtr *string
		if kind != nil {
			k := *kind
			kindPtr = &k
		}
		models = append(models, database.ResourceModel{
			ResourceUID:     uid,
			CloudAccountID:  accountID,
			ProviderCode:    res.ProviderCode,
			SubscriptionID:  res.SubscriptionID,
			ResourceGroup:   res.ResourceGroup,
			ServerName:      res.ServerName,
			DatabaseName:    res.DatabaseName,
			AzureResourceID: res.AzureResourceID,
			RegionCode:      res.RegionCode,
			Kind:            kindPtr,
			LifecycleStatus: res.LifecycleStatus,
			RegisteredAt:    now,
			LastSeenAt:      res.LastSeenAt,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "azure_resource_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"subscription_id", "server_name", "database_name", "region_code",
			"kind", "lifecycle_status", "last_seen_at", "updated_at",
		}),
	}).Table(database.TableName(r.db, "cloud", "resources")).
		Create(models).Error
	if err != nil {
		return nil, err
	}
	// Return the persisted rows so ResourceUIDs reflect what is stored (stable
	// across re-syncs); GORM does not populate back the PK on ON CONFLICT.
	return r.ListResources(ctx, accountID)
}

func (r *resourceRepository) ListResources(ctx context.Context, accountID uuid.UUID) ([]cloud.ResourceDTO, error) {
	var models []database.ResourceModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resources")).
		Where("cloud_account_id = ?", accountID).
		Order("server_name, database_name").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]cloud.ResourceDTO, len(models))
	for i, m := range models {
		dtos[i] = toResourceDTO(m)
	}
	return dtos, nil
}

func (r *resourceRepository) GetResource(ctx context.Context, accountID, resourceUID uuid.UUID) (cloud.ResourceDTO, error) {
	var model database.ResourceModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resources")).
		Where("cloud_account_id = ? AND resource_uid = ?", accountID, resourceUID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return cloud.ResourceDTO{}, cloud.ErrResourceNotFound
		}
		return cloud.ResourceDTO{}, err
	}
	return toResourceDTO(model), nil
}

func toResourceDTO(m database.ResourceModel) cloud.ResourceDTO {
	var kind *string
	if m.Kind != nil {
		k := *m.Kind
		kind = &k
	}
	return cloud.ResourceDTO{
		ResourceUID:     m.ResourceUID,
		CloudAccountID:  m.CloudAccountID,
		ProviderCode:    m.ProviderCode,
		SubscriptionID:  m.SubscriptionID,
		ResourceGroup:   m.ResourceGroup,
		ServerName:      m.ServerName,
		DatabaseName:    m.DatabaseName,
		AzureResourceID: m.AzureResourceID,
		RegionCode:      m.RegionCode,
		Kind:            kind,
		LifecycleStatus: m.LifecycleStatus,
		LastSeenAt:      m.LastSeenAt,
	}
}

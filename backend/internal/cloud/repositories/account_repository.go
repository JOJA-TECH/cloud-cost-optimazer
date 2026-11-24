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

type accountRepository struct{ db *gorm.DB }

func NewAccountRepository(db *gorm.DB) cloud.AccountRepository {
	return &accountRepository{db: db}
}

func (r *accountRepository) CreateAccount(ctx context.Context, input cloud.CreateAccountInput) (cloud.AccountDTO, error) {
	// Guard against duplicate provider+subscription. The DB unique constraint
	// enforces this in Postgres; this check covers SQLite dev/tests where
	// GORM AutoMigrate does not create composite unique indexes.
	var existing database.CloudAccountModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "accounts")).
		Where("provider_code = ? AND subscription_id = ?", input.ProviderCode, input.SubscriptionID).
		First(&existing).Error
	if err == nil {
		return cloud.AccountDTO{}, cloud.ErrAccountExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return cloud.AccountDTO{}, err
	}

	now := time.Now().UTC()
	model := database.CloudAccountModel{
		CloudAccountID:      uuid.New(),
		OrganizationID:      input.OrgID,
		ProviderCode:        input.ProviderCode,
		DisplayName:         input.DisplayName,
		SubscriptionID:      input.SubscriptionID,
		TenantID:            strPtr(input.TenantID),
		CredentialReference: strPtr(input.CredentialReference),
		Status:              "pending",
		ClientID:            input.ClientID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	err = r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "accounts")).
		Create(&model).Error
	if err != nil {
		if isUniqueViolation(err) {
			return cloud.AccountDTO{}, cloud.ErrAccountExists
		}
		return cloud.AccountDTO{}, err
	}
	return toAccountDTO(model), nil
}

func (r *accountRepository) ListAccounts(ctx context.Context, orgID uuid.UUID) ([]cloud.AccountDTO, error) {
	var models []database.CloudAccountModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "accounts")).
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	dtos := make([]cloud.AccountDTO, len(models))
	for i, m := range models {
		dtos[i] = toAccountDTO(m)
	}
	return dtos, nil
}

func (r *accountRepository) GetAccount(ctx context.Context, id uuid.UUID) (cloud.AccountDTO, error) {
	var model database.CloudAccountModel
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "accounts")).
		Where("cloud_account_id = ?", id).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return cloud.AccountDTO{}, cloud.ErrAccountNotFound
		}
		return cloud.AccountDTO{}, err
	}
	return toAccountDTO(model), nil
}

func (r *accountRepository) UpdateAccountStatus(ctx context.Context, id uuid.UUID, status string, connectionError *string) error {
	updates := map[string]interface{}{
		"status":                status,
		"last_connection_error": connectionError,
		"updated_at":            time.Now().UTC(),
	}
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "accounts")).
		Where("cloud_account_id = ?", id).
		Updates(updates).Error
	if err != nil {
		return err
	}
	return nil
}

func toAccountDTO(m database.CloudAccountModel) cloud.AccountDTO {
	var tenantID, credRef, clientID, lastErr *string
	tenantID = m.TenantID
	credRef = m.CredentialReference
	clientID = m.ClientID
	lastErr = m.LastConnectionError
	return cloud.AccountDTO{
		CloudAccountID:      m.CloudAccountID,
		OrganizationID:      m.OrganizationID,
		ProviderCode:        m.ProviderCode,
		DisplayName:         m.DisplayName,
		SubscriptionID:      m.SubscriptionID,
		TenantID:            strVal(tenantID),
		ClientID:            strVal(clientID),
		CredentialReference: strVal(credRef),
		Status:              m.Status,
		LastConnectionError: lastErr,
		CreatedAt:           m.CreatedAt.UnixMilli(),
		UpdatedAt:           m.UpdatedAt.UnixMilli(),
	}
}

func strPtr(s string) *string { return &s }

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isUniqueViolation detects unique constraint violations across drivers.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// SQLite (go-sqlite3): "UNIQUE constraint failed"
	// Postgres (pgx): code 23505
	return contains(msg, "UNIQUE constraint failed") ||
		contains(msg, "duplicate key") ||
		contains(msg, "23505")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

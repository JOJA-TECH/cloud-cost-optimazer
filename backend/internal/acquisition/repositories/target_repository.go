// Package repositories implements the acquisition persistence layer: reads
// monitored targets and credentials from cco_app and writes acquisition data
// to cco_data.
package repositories

import (
	"context"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// targetRepository reads monitored targets and account credentials from cco_app.
type targetRepository struct{ db *gorm.DB }

// NewTargetRepository builds a TargetRepository backed by cco_app (cloud schema).
func NewTargetRepository(db *gorm.DB) acquisition.TargetRepository {
	return &targetRepository{db: db}
}

// ListMonitoredTargets returns targets for bindings with is_monitored=true,
// joined with cloud.resources. When orgID is non-nil it scopes to the account's
// organization via cloud.accounts.
func (r *targetRepository) ListMonitoredTargets(ctx context.Context, orgID *uuid.UUID) ([]acquisition.Target, error) {
	var bindings []database.ResourceBindingModel
	q := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resource_bindings")).
		Where("is_monitored = ?", true)
	if orgID != nil {
		q = q.Joins("JOIN "+database.TableName(r.db, "cloud", "accounts")+" a ON a.cloud_account_id = resource_bindings.cloud_account_id").
			Where("a.organization_id = ?", *orgID)
	}
	if err := q.Find(&bindings).Error; err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return []acquisition.Target{}, nil
	}

	// Collect the data resource UIDs and load the matching resources.
	uidSet := map[uuid.UUID]struct{}{}
	for _, b := range bindings {
		uidSet[b.DataResourceUID] = struct{}{}
	}
	uids := make([]uuid.UUID, 0, len(uidSet))
	for u := range uidSet {
		uids = append(uids, u)
	}

	var resources []database.ResourceModel
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "resources")).
		Where("resource_uid IN ?", uids).
		Find(&resources).Error; err != nil {
		return nil, err
	}
	resourceByUID := make(map[uuid.UUID]database.ResourceModel, len(resources))
	for _, res := range resources {
		resourceByUID[res.ResourceUID] = res
	}

	targets := make([]acquisition.Target, 0, len(bindings))
	for _, b := range bindings {
		res, ok := resourceByUID[b.DataResourceUID]
		if !ok {
			// Binding references a missing resource: skip it.
			continue
		}
		targets = append(targets, acquisition.Target{
			ResourceUID:     res.ResourceUID,
			CloudAccountID:  b.CloudAccountID,
			AzureResourceID: res.AzureResourceID,
			SubscriptionID:  res.SubscriptionID,
			ResourceGroup:   res.ResourceGroup,
			ServerName:      res.ServerName,
			DatabaseName:    res.DatabaseName,
			RegionCode:      res.RegionCode,
			Kind:            strPtrDeref(res.Kind),
		})
	}
	return targets, nil
}

// ResolveCredentials loads the account and resolves its credential_reference
// ("env:VAR" or bare "VAR") against the environment.
func (r *targetRepository) ResolveCredentials(ctx context.Context, cloudAccountID uuid.UUID) (cloud.ConnectionCredentials, error) {
	var account database.CloudAccountModel
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "cloud", "accounts")).
		Where("cloud_account_id = ?", cloudAccountID).
		First(&account).Error; err != nil {
		return cloud.ConnectionCredentials{}, err
	}

	if account.CredentialReference == nil || *account.CredentialReference == "" {
		return cloud.ConnectionCredentials{}, cloud.ErrCredentialsUnavailable
	}
	secret, err := cloud.ResolveSecretRef(*account.CredentialReference)
	if err != nil {
		return cloud.ConnectionCredentials{}, err
	}

	return cloud.ConnectionCredentials{
		TenantID:       strVal(account.TenantID),
		ClientID:       strVal(account.ClientID),
		ClientSecret:   secret,
		SubscriptionID: account.SubscriptionID,
	}, nil
}

func strPtrDeref(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

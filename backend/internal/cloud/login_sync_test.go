package cloud

import (
	"context"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

type loginSyncFake struct {
	orgID     uuid.UUID
	projectID uuid.UUID
	account   AccountDTO
	resources []ResourceDTO
	bindings  []BindingDTO
}

func (f *loginSyncFake) CreateAccount(_ context.Context, in CreateAccountInput) (AccountDTO, error) {
	f.account = AccountDTO{
		CloudAccountID:      uuid.New(),
		OrganizationID:      in.OrgID,
		ProviderCode:        in.ProviderCode,
		SubscriptionID:      in.SubscriptionID,
		TenantID:            in.TenantID,
		ClientID:            strValFake(in.ClientID),
		CredentialReference: in.CredentialReference,
	}
	return f.account, nil
}

func (f *loginSyncFake) ListAccounts(_ context.Context, _ uuid.UUID) ([]AccountDTO, error) {
	if f.account.CloudAccountID == uuid.Nil {
		return nil, nil
	}
	return []AccountDTO{f.account}, nil
}

func (f *loginSyncFake) GetAccount(_ context.Context, id uuid.UUID) (AccountDTO, error) {
	if f.account.CloudAccountID != id {
		return AccountDTO{}, ErrAccountNotFound
	}
	return f.account, nil
}

func (f *loginSyncFake) UpdateAccountStatus(_ context.Context, _ uuid.UUID, status string, _ *string) error {
	f.account.Status = status
	return nil
}

func (f *loginSyncFake) UpsertResources(_ context.Context, _ uuid.UUID, in []ResourceDTO) ([]ResourceDTO, error) {
	for _, r := range in {
		if r.ResourceUID == uuid.Nil {
			r.ResourceUID = uuid.New()
		}
		found := false
		for i, existing := range f.resources {
			if existing.AzureResourceID == r.AzureResourceID {
				r.ResourceUID = existing.ResourceUID
				f.resources[i] = r
				found = true
				break
			}
		}
		if !found {
			f.resources = append(f.resources, r)
		}
	}
	return f.resources, nil
}

func (f *loginSyncFake) ListResources(_ context.Context, _ uuid.UUID) ([]ResourceDTO, error) {
	return f.resources, nil
}

func (f *loginSyncFake) GetResource(_ context.Context, _, uid uuid.UUID) (ResourceDTO, error) {
	for _, r := range f.resources {
		if r.ResourceUID == uid {
			return r, nil
		}
	}
	return ResourceDTO{}, ErrResourceNotFound
}

func (f *loginSyncFake) CreateBinding(_ context.Context, in CreateBindingInput) (BindingDTO, error) {
	f.bindings = append(f.bindings, BindingDTO{
		ProjectID:       in.ProjectID,
		CloudAccountID:  in.CloudAccountID,
		DataResourceUID: in.DataResourceUID,
		IsMonitored:     true,
	})
	return f.bindings[len(f.bindings)-1], nil
}

func (f *loginSyncFake) GetBinding(_ context.Context, _ uuid.UUID) (BindingDTO, error) {
	return BindingDTO{}, ErrBindingNotFound
}

func (f *loginSyncFake) ListBindings(_ context.Context, _ *uuid.UUID) ([]BindingDTO, error) {
	return f.bindings, nil
}

func (f *loginSyncFake) SetBindingMonitored(context.Context, uuid.UUID, bool) error { return nil }

func (f *loginSyncFake) ProjectOrgID(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return f.orgID, nil
}

func (f *loginSyncFake) ListProjectsByOrg(context.Context, uuid.UUID) ([]iam.ProjectDTO, error) {
	return []iam.ProjectDTO{{ProjectID: f.projectID, OrganizationID: f.orgID}}, nil
}

func (f *loginSyncFake) ListSQLDatabases(context.Context, ConnectionCredentials) ([]ResourceDTO, error) {
	return []ResourceDTO{{ServerName: "sql", DatabaseName: "db", AzureResourceID: "/subscriptions/sub/db"}}, nil
}

func (f *loginSyncFake) ValidateConnection(context.Context, ConnectionCredentials) (ConnectionResult, error) {
	return ConnectionResult{Status: "active"}, nil
}

func strValFake(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestSyncOnLoginRegistersAccountResourcesAndBinding(t *testing.T) {
	ctx := context.Background()
	orgID, projectID, userID := uuid.New(), uuid.New(), uuid.New()
	f := &loginSyncFake{orgID: orgID, projectID: projectID}
	svc := NewService(f, f, f, f, f)
	cfg := LoginSyncConfig{
		TenantID:            "tenant",
		SubscriptionID:      "sub",
		ClientID:            "client",
		CredentialReference: "env:SECRET",
	}
	t.Setenv("SECRET", "secret")

	for i := 0; i < 2; i++ {
		if err := svc.SyncOnLogin(ctx, orgID, userID, f, cfg); err != nil {
			t.Fatalf("SyncOnLogin #%d: %v", i+1, err)
		}
	}
	if f.account.CloudAccountID == uuid.Nil || f.account.OrganizationID != orgID || f.account.Status != "active" {
		t.Fatalf("account not registered: %+v", f.account)
	}
	if len(f.resources) != 1 || len(f.bindings) != 1 {
		t.Fatalf("got %d resources and %d bindings, want 1 each", len(f.resources), len(f.bindings))
	}
	if b := f.bindings[0]; b.ProjectID != projectID || b.DataResourceUID != f.resources[0].ResourceUID || !b.IsMonitored {
		t.Fatalf("unexpected binding: %+v", b)
	}
}

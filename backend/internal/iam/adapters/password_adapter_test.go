package adapters_test

import (
	"context"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/adapters"
	"github.com/google/uuid"
)

type passwordTestRepo struct {
	iam.IdentityRepository
	user      iam.UserDTO
	hash      string
	userRoles []iam.UserRoleDTO
}

func (r *passwordTestRepo) FindUserByEmail(context.Context, string) (iam.UserDTO, error) {
	return r.user, nil
}

func (r *passwordTestRepo) FindPasswordHash(context.Context, uuid.UUID) (string, error) {
	return r.hash, nil
}

func (r *passwordTestRepo) ListMembershipsByUser(context.Context, uuid.UUID) ([]iam.MembershipDTO, error) {
	return nil, nil
}

func (r *passwordTestRepo) ListUserRolesByUser(context.Context, uuid.UUID) ([]iam.UserRoleDTO, error) {
	return r.userRoles, nil
}

func (r *passwordTestRepo) FindRoleByID(_ context.Context, id int16) (iam.RoleDTO, error) {
	return iam.RoleDTO{RoleID: id, RoleCode: "analyst"}, nil
}

// A password login must carry assigned system roles: the permission checker
// uses them, not the cached permissions claim.
func TestPasswordAdapterLoadsSystemRoles(t *testing.T) {
	hash, err := iam.HashPassword("Test123!")
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.New()
	repo := &passwordTestRepo{
		user:      iam.UserDTO{ID: userID, Email: "user@example.com", DisplayName: "User", Status: "active"},
		hash:      hash,
		userRoles: []iam.UserRoleDTO{{UserID: userID, RoleID: 1}},
	}

	ic, err := adapters.NewPasswordAdapter(repo, nil).Authenticate(context.Background(), iam.Credentials{
		Type: "password", Email: "user@example.com", Password: "Test123!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ic.SystemRoles) != 1 || ic.SystemRoles[0] != "analyst" {
		t.Fatalf("SystemRoles = %v, want [analyst]", ic.SystemRoles)
	}
	ok, err := iam.NewPermissionEngine(nil).Can(context.Background(), ic, "analytics", "read")
	if err != nil || !ok {
		t.Errorf("analyst should read analytics (ok=%v, err=%v)", ok, err)
	}
}

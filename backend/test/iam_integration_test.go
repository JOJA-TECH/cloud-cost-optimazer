package test

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// openTestDB opens a shared in-memory SQLite database and runs migrations.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(database.Config{
		Driver: database.DriverSQLite,
		DSN:    "file::memory:?cache=shared",
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// strPtr returns a pointer to s.
func strPtr(s string) *string {
	return &s
}

// seedRolesAndPermissions inserts platform_admin and viewer roles plus a
// permission, since the migration creates empty tables. Returns role IDs.
func seedRolesAndPermissions(t *testing.T, db *gorm.DB) (int16, int16) {
	t.Helper()
	now := time.Now().UTC()

	for _, r := range []database.RoleModel{
		{RoleID: 1, RoleCode: "platform_admin", Description: strPtr("Full platform access"), IsSystemRole: true, CreatedAt: now},
		{RoleID: 2, RoleCode: "viewer", Description: strPtr("Read-only access"), IsSystemRole: true, CreatedAt: now},
	} {
		if err := db.Table("roles").Create(&r).Error; err != nil {
			t.Fatalf("seed role %s: %v", r.RoleCode, err)
		}
	}

	for _, p := range []database.PermissionModel{
		{PermissionID: 1, PermissionCode: "resources:read", Description: strPtr("Read resources"), CreatedAt: now},
		{PermissionID: 2, PermissionCode: "resources:write", Description: strPtr("Write resources"), CreatedAt: now},
	} {
		if err := db.Table("permissions").Create(&p).Error; err != nil {
			t.Fatalf("seed permission %s: %v", p.PermissionCode, err)
		}
	}

	if err := db.Table("role_permissions").Create(&database.RolePermissionModel{RoleID: 1, PermissionID: 1}).Error; err != nil {
		t.Fatalf("seed role_permission: %v", err)
	}

	return 1, 2
}

func TestIAMStackEndToEnd(t *testing.T) {
	db := openTestDB(t)
	platformAdminID, viewerID := seedRolesAndPermissions(t, db)
	ctx := context.Background()

	// a. Create a user.
	userRepo := iam.NewUserRepository(db)
	user, err := userRepo.CreateUser(ctx, iam.CreateUserInput{
		Email:        "ana@example.com",
		DisplayName:  "Ana",
		PasswordHash: nil,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.Email != "ana@example.com" || user.DisplayName != "Ana" {
		t.Fatalf("unexpected user DTO: %#v", user)
	}

	// b./c. Assign the platform_admin role to the user.
	userRoleRepo := repositories.NewUserRoleRepository(db)
	if err := userRoleRepo.AssignRole(ctx, user.ID, platformAdminID, nil); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	// d. Create an organization and add the user as a member.
	orgID := uuid.New()
	if err := db.Table("organizations").Create(&database.OrganizationModel{
		OrganizationID: orgID,
		Name:           "Acme Corp",
		Slug:           "acme",
		Status:         "active",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}

	membershipRepo := repositories.NewMembershipRepository(db)
	mem, err := membershipRepo.AddMember(ctx, iam.AddMemberInput{
		OrgID:          orgID,
		UserID:         user.ID,
		MembershipRole: "owner",
		InvitedBy:      nil,
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	if mem.MembershipRole != "owner" || mem.Status != "active" {
		t.Fatalf("unexpected membership: %#v", mem)
	}

	// e. Create a project in the organization (ProjectRepository has no Create).
	projectID := uuid.New()
	if err := db.Table("projects").Create(&database.ProjectModel{
		ProjectID:      projectID,
		OrganizationID: orgID,
		Name:           "Acme Project",
		Description:    strPtr("Main project"),
		Status:         "active",
		CreatedBy:      &user.ID,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	// f. Check user project access (org membership grants project access).
	projectRepo := repositories.NewProjectRepository(db)
	hasAccess, err := projectRepo.CheckUserProjectAccess(ctx, user.ID, projectID)
	if err != nil {
		t.Fatalf("check user project access: %v", err)
	}
	if !hasAccess {
		t.Fatal("expected user to have project access via org membership")
	}

	// g. Link an external (OIDC) identity to the user.
	extRepo := repositories.NewExternalIdentityRepository(db)
	extID, err := extRepo.LinkExternalIdentity(ctx, iam.CreateExternalIdentityInput{
		UserID:   user.ID,
		Provider: "oidc",
		Subject:  "google|123456",
	})
	if err != nil {
		t.Fatalf("link external identity: %v", err)
	}
	if extID.Provider != "oidc" || extID.Subject != "google|123456" || extID.UserID != user.ID {
		t.Fatalf("unexpected external identity: %#v", extID)
	}

	// h. Verify PermissionEngine.Can with an identity context built from repo data.
	roleRepo := repositories.NewRoleRepository(db)
	userRoles, err := userRoleRepo.ListUserRolesByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("list user roles: %v", err)
	}
	var systemRoles []string
	for _, ur := range userRoles {
		role, err := roleRepo.FindRoleByID(ctx, ur.RoleID)
		if err == nil {
			systemRoles = append(systemRoles, role.RoleCode)
		}
	}

	ic := iam.IdentityContext{
		UserID:         user.ID,
		Email:          user.Email,
		DisplayName:    user.DisplayName,
		AuthMethod:     "oidc",
		SystemRoles:    systemRoles,
		OrgID:          orgID,
		OrgRole:        mem.MembershipRole,
		ProjectID:      &projectID,
		ProjectRole:    strPtr("owner"),
		Permissions:    []string{},
		RequestID:      "req-1",
		CorrelationID:  "corr-1",
		TokenExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}
	engine := iam.NewPermissionEngine()

	ok, err := engine.Can(context.Background(), ic, "resources", "read")
	if err != nil || !ok {
		t.Error("expected platform_admin wildcard to allow resources:read")
	}
	ok, err = engine.Can(context.Background(), ic, "cloud_accounts", "delete")
	if err != nil || !ok {
		t.Error("expected platform_admin *:* wildcard to allow cloud_accounts:delete")
	}

	// Viewer (org role only) allows read but not write.
	viewerIC := iam.IdentityContext{
		SystemRoles: []string{},
		OrgRole:     "viewer",
	}
	ok, err = engine.Can(context.Background(), viewerIC, "resources", "write")
	if err == nil && ok {
		t.Error("expected viewer to not allow resources:write")
	}
	ok, err = engine.Can(context.Background(), viewerIC, "resources", "read")
	if err != nil || !ok {
		t.Error("expected viewer to allow resources:read")
	}

	// Sanity: viewer role id is unused in this test path but kept for symmetry.
	_ = viewerID
}

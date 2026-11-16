package iam_test

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
)

// TestPermissionEngineLoadsRolesFromDB verifies the engine honors a role that
// exists only in the DB and falls back to the static map without a DB.
func TestPermissionEngineLoadsRolesFromDB(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if err := db.Table("roles").Create(&database.RoleModel{RoleID: 90, RoleCode: "auditor", IsSystemRole: true, CreatedAt: now}).Error; err != nil {
		t.Fatalf("seed role: %v", err)
	}
	if err := db.Table("permissions").Create(&database.PermissionModel{PermissionID: 90, PermissionCode: "audit:read", CreatedAt: now}).Error; err != nil {
		t.Fatalf("seed permission: %v", err)
	}
	if err := db.Table("role_permissions").Create(&database.RolePermissionModel{RoleID: 90, PermissionID: 90}).Error; err != nil {
		t.Fatalf("seed role_permission: %v", err)
	}

	// Engine built with the DB honors a DB-only role.
	engine := iam.NewPermissionEngine(db)
	ic := iam.IdentityContext{SystemRoles: []string{"auditor"}}
	if ok, err := engine.Can(ctx, ic, "audit", "read"); err != nil || !ok {
		t.Errorf("expected DB-only role auditor to allow audit:read (ok=%v, err=%v)", ok, err)
	}
	if ok, err := engine.Can(ctx, ic, "audit", "write"); err != nil || ok {
		t.Errorf("expected auditor to not allow audit:write (ok=%v, err=%v)", ok, err)
	}

	// Engine built without a DB falls back to the static map.
	fallback := iam.NewPermissionEngine(nil)
	adminIC := iam.IdentityContext{SystemRoles: []string{"platform_admin"}}
	if ok, err := fallback.Can(ctx, adminIC, "anything", "delete"); err != nil || !ok {
		t.Errorf("expected platform_admin *:* via static fallback (ok=%v, err=%v)", ok, err)
	}
}

// TestPermissionEngineFailsClosedOnEmptyDB verifies that an empty permissions
// table does not fall back to the static map: platform_admin gets nothing.
func TestPermissionEngineFailsClosedOnEmptyDB(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	engine := iam.NewPermissionEngine(db)
	adminIC := iam.IdentityContext{SystemRoles: []string{"platform_admin"}}
	if ok, err := engine.Can(ctx, adminIC, "anything", "delete"); err != nil || ok {
		t.Errorf("expected empty DB to fail closed (ok=%v, err=%v)", ok, err)
	}
}

func TestSeedRecommendationPermissionsAddsExpectedSystemRoleGrants(t *testing.T) {
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for id, code := range map[int16]string{1: "platform_admin", 2: "db_admin", 3: "analyst", 4: "viewer"} {
		if err := db.Table("roles").Create(&database.RoleModel{
			RoleID: id, RoleCode: code, IsSystemRole: true, CreatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed role %q: %v", code, err)
		}
	}

	if err := iam.SeedRecommendationPermissions(db); err != nil {
		t.Fatalf("SeedRecommendationPermissions() error = %v", err)
	}
	if err := iam.SeedRecommendationPermissions(db); err != nil {
		t.Fatalf("second SeedRecommendationPermissions() error = %v", err)
	}

	engine := iam.NewPermissionEngine(db)
	tests := []struct {
		role, action string
		want         bool
	}{
		{"platform_admin", "write", true},
		{"db_admin", "evaluate", true},
		{"analyst", "write", true},
		{"viewer", "read", true},
		{"viewer", "write", false},
	}
	for _, test := range tests {
		allowed, err := engine.Can(context.Background(), iam.IdentityContext{
			SystemRoles: []string{test.role},
		}, "recommendations", test.action)
		if err != nil {
			t.Fatalf("Can(%q) error = %v", test.role, err)
		}
		if allowed != test.want {
			t.Errorf("Can(%q, recommendations:%s) = %v, want %v", test.role, test.action, allowed, test.want)
		}
	}
}

func TestSeedAuditPermissionsAddsScopedReadGrants(t *testing.T) {
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for id, code := range map[int16]string{1: "platform_admin", 2: "auditor", 3: "analyst"} {
		if err := db.Table("roles").Create(&database.RoleModel{
			RoleID: id, RoleCode: code, IsSystemRole: true, CreatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed role %q: %v", code, err)
		}
	}
	if err := iam.SeedAuditPermissions(db); err != nil {
		t.Fatalf("SeedAuditPermissions() error = %v", err)
	}
	if err := iam.SeedAuditPermissions(db); err != nil {
		t.Fatalf("second SeedAuditPermissions() error = %v", err)
	}

	engine := iam.NewPermissionEngine(db)
	for _, test := range []struct {
		role string
		want bool
	}{
		{role: "platform_admin", want: true},
		{role: "auditor", want: true},
		{role: "analyst", want: false},
	} {
		allowed, err := engine.Can(context.Background(), iam.IdentityContext{
			SystemRoles: []string{test.role},
		}, "audit", "read")
		if err != nil {
			t.Fatalf("Can(%q, audit:read) error = %v", test.role, err)
		}
		if allowed != test.want {
			t.Errorf("Can(%q, audit:read) = %v, want %v", test.role, allowed, test.want)
		}
	}
	if ok, err := engine.Can(context.Background(), iam.IdentityContext{
		SystemRoles: []string{"auditor"},
	}, "audit:read", "organization"); err != nil || !ok {
		t.Errorf("auditor should have organization scope (ok=%v, err=%v)", ok, err)
	}
}

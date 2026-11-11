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

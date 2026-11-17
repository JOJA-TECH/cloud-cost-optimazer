package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func SeedAuditPermissions(db *gorm.DB) error {
	if db == nil {
		return errors.New("database handle is nil")
	}

	grants := map[string][]string{
		"platform_admin": {"audit:read", "audit:read:organization", "audit:read:own"},
		"auditor":        {"audit:read", "audit:read:organization"},
	}
	descriptions := map[string]string{
		"audit:read":              "Read audit events with role-scoped access",
		"audit:read:organization": "Read audit events for the active organization",
		"audit:read:own":          "Read audit events created by the current user",
	}

	return db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		permissionIDs := make(map[string]int16, len(descriptions))
		permissionTable := database.TableName(tx, "iam", "permissions")
		for code, description := range descriptions {
			permission := database.PermissionModel{
				PermissionCode: code, Description: &description, CreatedAt: time.Now().UTC(),
			}
			if err := tx.Table(permissionTable).
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "permission_code"}},
					DoNothing: true,
				}).
				Create(&permission).Error; err != nil {
				return fmt.Errorf("seed permission %q: %w", code, err)
			}
			if err := tx.Table(permissionTable).Where("permission_code = ?", code).Take(&permission).Error; err != nil {
				return fmt.Errorf("load seeded permission %q: %w", code, err)
			}
			permissionIDs[code] = permission.PermissionID
		}

		roleTable := database.TableName(tx, "iam", "roles")
		rolePermissionsTable := database.TableName(tx, "iam", "role_permissions")
		var auditor database.RoleModel
		err := tx.Table(roleTable).Where("role_code = ?", "auditor").Take(&auditor).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			description := "Read audit events for authorized organizations"
			auditor = database.RoleModel{
				RoleCode: "auditor", Description: &description, IsSystemRole: true, CreatedAt: time.Now().UTC(),
			}
			if err := tx.Table(roleTable).Create(&auditor).Error; err != nil {
				return fmt.Errorf("seed auditor role: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("find auditor role: %w", err)
		}
		for roleCode, permissionCodes := range grants {
			var role database.RoleModel
			err := tx.Table(roleTable).Where("role_code = ?", roleCode).Take(&role).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return fmt.Errorf("find role %q: %w", roleCode, err)
			}
			for _, code := range permissionCodes {
				link := database.RolePermissionModel{RoleID: role.RoleID, PermissionID: permissionIDs[code]}
				if err := tx.Table(rolePermissionsTable).
					Clauses(clause.OnConflict{
						Columns:   []clause.Column{{Name: "role_id"}, {Name: "permission_id"}},
						DoNothing: true,
					}).
					Create(&link).Error; err != nil {
					return fmt.Errorf("grant %q to role %q: %w", code, roleCode, err)
				}
			}
		}
		return nil
	})
}

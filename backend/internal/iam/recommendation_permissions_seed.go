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

// SeedRecommendationPermissions adds the recommendation permissions to
// existing system roles without replacing any permissions already assigned.
func SeedRecommendationPermissions(db *gorm.DB) error {
	if db == nil {
		return errors.New("database handle is nil")
	}

	grants := map[string][]string{
		"platform_admin": {"recommendations:read", "recommendations:write", "recommendations:evaluate"},
		"db_admin":       {"recommendations:read", "recommendations:write", "recommendations:evaluate"},
		"analyst":        {"recommendations:read", "recommendations:write", "recommendations:evaluate"},
		"viewer":         {"recommendations:read"},
	}
	descriptions := map[string]string{
		"recommendations:read":     "Read recommendation decisions and evidence",
		"recommendations:write":    "Generate and persist recommendations",
		"recommendations:evaluate": "Evaluate recommendation opportunities without persisting",
	}

	return db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		permissionIDs := make(map[string]int16, len(descriptions))
		for code, description := range descriptions {
			table := database.TableName(tx, "iam", "permissions")
			permission := database.PermissionModel{
				PermissionCode: code, Description: &description, CreatedAt: time.Now().UTC(),
			}
			if err := tx.Table(table).
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "permission_code"}},
					DoNothing: true,
				}).
				Create(&permission).Error; err != nil {
				return fmt.Errorf("seed permission %q: %w", code, err)
			}
			if err := tx.Table(table).Where("permission_code = ?", code).Take(&permission).Error; err != nil {
				return fmt.Errorf("load seeded permission %q: %w", code, err)
			}
			permissionIDs[code] = permission.PermissionID
		}

		roleTable := database.TableName(tx, "iam", "roles")
		rolePermissionsTable := database.TableName(tx, "iam", "role_permissions")
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

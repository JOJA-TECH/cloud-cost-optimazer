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

// SeedJobsPermissions añade los permisos del módulo de jobs (durable) y de
// activación asíncrona de adquisición a los roles del sistema, sin reemplazar
// los permisos ya asignados.
func SeedJobsPermissions(db *gorm.DB) error {
	if db == nil {
		return errors.New("database handle is nil")
	}

	grants := map[string][]string{
		"platform_admin": {"jobs:read", "jobs:cancel", "acquisitions:run", "acquisitions:read", "analytics:compute"},
		"db_admin":       {"jobs:read", "jobs:cancel", "acquisitions:run", "acquisitions:read", "analytics:compute"},
		"analyst":        {"jobs:read", "acquisitions:read", "analytics:compute"},
		"viewer":         {"jobs:read", "acquisitions:read"},
	}
	descriptions := map[string]string{
		"jobs:read":         "Consultar estado y listado de jobs duraderos",
		"jobs:cancel":       "Cancelar jobs duraderos en curso o encolados",
		"acquisitions:run":  "Ejecutar un ciclo de adquisición de telemetría (vía job)",
		"acquisitions:read": "Consultar runs y estado de adquisición",
		"analytics:compute": "Ejecutar operaciones analíticas pesadas (features, percentiles, forecast) como job duradero",
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

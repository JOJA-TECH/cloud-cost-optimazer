package database

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "postgres"
)

type Config struct {
	Driver   Driver
	DSN      string
	LogLevel int
	// Pool configura el pool de conexiones del driver subyacente (database/sql).
	// Cualquier valor <=0 se ignora y el driver conserva sus propios defaults.
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Open(cfg Config) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database DSN is required")
	}
	gormCfg := &gorm.Config{}
	var dialector gorm.Dialector
	switch Driver(strings.ToLower(string(cfg.Driver))) {
	case DriverSQLite:
		dialector = sqlite.Open(cfg.DSN)
	case DriverPostgres:
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, err
	}
	// database/sql ya administra el pool de conexiones (está dentro de GORM);
	// estos knobs solo lo tunnean. Valores <=0 dejan los defaults del driver.
	if sqlDB, err := db.DB(); err == nil {
		if cfg.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
		}
		if cfg.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
		}
		if cfg.ConnMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
		}
	}
	return db, nil
}

// TableName qualifies schemas only on PostgreSQL. SQLite has no schemas, so
// the same model set can be used in local tests and in production.
func TableName(db *gorm.DB, schema, table string) string {
	if db.Dialector.Name() == string(DriverPostgres) {
		return schema + "." + table
	}
	return table
}

// SchemaMigrationOK checks whether all schema migrations have been applied
// successfully. It returns true if all migrations in schemaMigrations have
// been applied with matching checksums, or false otherwise.
func SchemaMigrationOK(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	ordered := make([]struct {
		version  string
		checksum string
		sql      string
	}, len(schemaMigrations))
	copy(ordered, schemaMigrations)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].version < ordered[j].version
	})
	for _, m := range ordered {
		var existing string
		err := db.Raw(fmt.Sprintf(
			"SELECT checksum FROM %s WHERE version = ?",
			schemaMigrationTableName(db),
		), m.version).Scan(&existing).Error
		if err != nil || existing != m.checksum {
			return false
		}
	}
	return true
}

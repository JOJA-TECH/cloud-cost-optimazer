package database

import (
	"fmt"
	"strings"

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
	return gorm.Open(dialector, gormCfg)
}

// TableName qualifies schemas only on PostgreSQL. SQLite has no schemas, so
// the same model set can be used in local tests and in production.
func TableName(db *gorm.DB, schema, table string) string {
	if db.Dialector.Name() == string(DriverPostgres) {
		return schema + "." + table
	}
	return table
}

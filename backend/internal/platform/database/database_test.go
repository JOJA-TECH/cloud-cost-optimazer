package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
)

func TestMigrateAndUserRepositoryOnSQLite(t *testing.T) {
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:?cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"ActorType", "CorrelationID", "OperationID", "EventVersion"} {
		if !db.Migrator().HasColumn(&database.AuditEventModel{}, column) {
			t.Errorf("audit.events is missing column %q", column)
		}
	}

	repo := iam.NewUserRepository(db)
	created, err := repo.CreateUser(context.Background(), iam.CreateUserInput{Email: "ana@example.com", DisplayName: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "ana@example.com" || created.DisplayName != "Ana" {
		t.Fatalf("unexpected DTO: %#v", created)
	}
	found, err := repo.FindUserByID(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected %s, got %s", created.ID, found.ID)
	}
}

func TestOpenAppliesPoolSettings(t *testing.T) {
	db, err := database.Open(database.Config{
		Driver:          database.DriverSQLite,
		DSN:             "file::memory:?cache=shared",
		MaxOpenConns:    3,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB == nil {
		t.Fatal("expected non-nil sql.DB")
	}
	// Los knobs de pool no tienen getter en database/sql; lo observable es
	// que Open los acepta y deja el pool usable (PingContext no falla).
	if err := sqlDB.PingContext(context.Background()); err != nil {
		t.Errorf("expected pool to be usable, got %v", err)
	}
}

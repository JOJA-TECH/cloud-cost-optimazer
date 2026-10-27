package database_test

import (
	"context"
	"testing"

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

	repo := iam.NewUserRepository(db)
	created, err := repo.Create(context.Background(), iam.CreateUserInput{Email: "ana@example.com", DisplayName: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "ana@example.com" || created.DisplayName != "Ana" {
		t.Fatalf("unexpected DTO: %#v", created)
	}
	found, err := repo.FindByID(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected %s, got %s", created.ID, found.ID)
	}
}

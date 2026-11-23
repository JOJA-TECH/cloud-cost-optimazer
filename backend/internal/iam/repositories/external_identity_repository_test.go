package repositories_test

import (
	"context"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
)

func TestLinkExternalIdentityIsIdempotent(t *testing.T) {
	db, err := database.Open(database.Config{Driver: database.DriverSQLite, DSN: "file::memory:"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}

	repo := repositories.NewExternalIdentityRepository(db)
	ctx := context.Background()
	input := iam.CreateExternalIdentityInput{
		UserID:   uuid.New(),
		Provider: "entra_id",
		Subject:  "subject-1",
	}
	first, err := repo.LinkExternalIdentity(ctx, input)
	if err != nil {
		t.Fatalf("first link: %v", err)
	}
	second, err := repo.LinkExternalIdentity(ctx, input)
	if err != nil {
		t.Fatalf("second link: %v", err)
	}
	if first.ExternalIdentityID != second.ExternalIdentityID {
		t.Fatalf("expected the same identity, got %s and %s", first.ExternalIdentityID, second.ExternalIdentityID)
	}

	var count int64
	if err := db.Table(database.TableName(db, "iam", "external_identities")).
		Where("provider = ? AND subject = ?", input.Provider, input.Subject).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 external identity, got %d", count)
	}
}

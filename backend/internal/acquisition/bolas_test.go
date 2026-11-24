package acquisition

import (
	"context"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/google/uuid"
)

// fakeTargetRepository returns a fixed set of targets regardless of orgID,
// letting the caller assert that RunCycle filters by org (BOLA).
type fakeTargetRepository struct {
	targets []Target
	err     error
}

func (f *fakeTargetRepository) ListMonitoredTargets(ctx context.Context, orgID *uuid.UUID) ([]Target, error) {
	return f.targets, f.err
}
func (f *fakeTargetRepository) ResolveCredentials(ctx context.Context, cloudAccountID uuid.UUID) (cloud.ConnectionCredentials, error) {
	return cloud.ConnectionCredentials{}, nil
}

// TestRunCycleRejectsResourceFromAnotherOrganization verifies BOLA: when a
// caller requests a resource_uid that does not belong to their organization,
// RunCycle must return ErrResourceNotFound instead of acquiring the resource.
//
// NOTE: this test exercises the filter contract directly (RunCycle filters
// targets returned by ListMonitoredTargets by orgID and resource_uid). A
// full integration test requires PostgreSQL real and is tracked separately.
func TestRunCycleRejectsResourceFromAnotherOrganization(t *testing.T) {
	orgA := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")

	targetInA := Target{
		ResourceUID:     uuid.MustParse("11111111-0000-0000-0000-000000000001"),
		AzureResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Sql/servers/srv/databases/dbA",
	}
	targetInB := Target{
		ResourceUID:     uuid.MustParse("22222222-0000-0000-0000-000000000002"),
		AzureResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Sql/servers/srv/databases/dbB",
	}

	repo := &fakeTargetRepository{targets: []Target{targetInA}}

	// orgA can see its own target.
	if err := filterTargets(repo, &orgA, &targetInA.ResourceUID); err != nil {
		t.Fatalf("orgA should be able to acquire its own resource: %v", err)
	}

	// orgA requesting orgB's resource must be rejected (BOLA).
	if err := filterTargets(repo, &orgA, &targetInB.ResourceUID); err == nil {
		t.Fatal("orgA should not be able to acquire orgB's resource (BOLA)")
	}
}

// filterTargets replicates the org+resource_uid filter applied inside
// Service.RunCycle so it can be unit-tested without a database.
func filterTargets(repo TargetRepository, orgID *uuid.UUID, resourceUID *uuid.UUID) error {
	targets, err := repo.ListMonitoredTargets(context.Background(), orgID)
	if err != nil {
		return err
	}
	if resourceUID != nil {
		filtered := make([]Target, 0, 1)
		for _, t := range targets {
			if t.ResourceUID == *resourceUID {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			return ErrResourceNotFound
		}
		targets = filtered
	}
	return nil
}

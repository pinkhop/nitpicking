package core_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestPlanEpicDeletion_NoDescendants_DeletesOnlyEpic(t *testing.T) {
	t.Parallel()

	// Given
	epicID := mustDomainID(t)

	// When
	result := core.PlanEpicDeletion(epicID, nil)

	// Then
	if len(result.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if len(result.ToDelete) != 1 {
		t.Fatalf("expected 1 ID to delete, got %d", len(result.ToDelete))
	}
	if result.ToDelete[0] != epicID {
		t.Errorf("expected epic ID in delete list")
	}
}

func TestPlanEpicDeletion_UnclaimedDescendants_DeletesAll(t *testing.T) {
	t.Parallel()

	// Given
	epicID := mustDomainID(t)
	child1 := mustDomainID(t)
	child2 := mustDomainID(t)
	descendants := []domain.DescendantInfo{
		{ID: child1, IsClaimed: false},
		{ID: child2, IsClaimed: false},
	}

	// When
	result := core.PlanEpicDeletion(epicID, descendants)

	// Then
	if len(result.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(result.Conflicts))
	}
	if len(result.ToDelete) != 3 {
		t.Errorf("expected 3 IDs to delete, got %d", len(result.ToDelete))
	}
}

func TestPlanEpicDeletion_ClaimedDescendant_ReportsConflict(t *testing.T) {
	t.Parallel()

	// Given
	epicID := mustDomainID(t)
	claimedChild := mustDomainID(t)
	descendants := []domain.DescendantInfo{
		{ID: claimedChild, IsClaimed: true, ClaimedBy: "agent-1"},
		{ID: mustDomainID(t), IsClaimed: false},
	}

	// When
	result := core.PlanEpicDeletion(epicID, descendants)

	// Then
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(result.Conflicts))
	}
	if result.Conflicts[0].ID != claimedChild {
		t.Error("expected claimed child in conflicts")
	}
	if len(result.ToDelete) != 0 {
		t.Error("expected empty delete list when conflicts exist")
	}
}

func TestValidateDeletion_AlreadyDeleted_Fails(t *testing.T) {
	t.Parallel()

	// When
	err := core.ValidateDeletion(true)

	// Then
	if !errors.Is(err, domain.ErrDeletedIssue) {
		t.Errorf("expected ErrDeletedIssue, got %v", err)
	}
}

func TestValidateDeletion_NotDeleted_Succeeds(t *testing.T) {
	t.Parallel()

	// When
	err := core.ValidateDeletion(false)
	// Then
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

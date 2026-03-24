package issue_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestPlanEpicDeletion_NoDescendants_DeletesOnlyEpic(t *testing.T) {
	t.Parallel()

	// Given
	epicID := mustID(t)

	// When
	result := issue.PlanEpicDeletion(epicID, nil)

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
	epicID := mustID(t)
	child1 := mustID(t)
	child2 := mustID(t)
	descendants := []issue.DescendantInfo{
		{ID: child1, IsClaimed: false},
		{ID: child2, IsClaimed: false},
	}

	// When
	result := issue.PlanEpicDeletion(epicID, descendants)

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
	epicID := mustID(t)
	claimedChild := mustID(t)
	descendants := []issue.DescendantInfo{
		{ID: claimedChild, IsClaimed: true, ClaimedBy: "agent-1"},
		{ID: mustID(t), IsClaimed: false},
	}

	// When
	result := issue.PlanEpicDeletion(epicID, descendants)

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
	err := issue.ValidateDeletion(true)

	// Then
	if !errors.Is(err, domain.ErrDeletedIssue) {
		t.Errorf("expected ErrDeletedIssue, got %v", err)
	}
}

func TestValidateDeletion_NotDeleted_Succeeds(t *testing.T) {
	t.Parallel()

	// When
	err := issue.ValidateDeletion(false)
	// Then
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

package issue_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestValidateParent_ValidEpicParent_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	childID := mustID(t)
	parentID := mustID(t)

	// When
	err := issue.ValidateParent(childID, parentID, issue.RoleEpic, false)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParent_TaskParent_Fails(t *testing.T) {
	t.Parallel()

	// When
	err := issue.ValidateParent(mustID(t), mustID(t), issue.RoleTask, false)

	// Then
	if err == nil {
		t.Fatal("expected error for task parent")
	}
}

func TestValidateParent_SelfParent_Fails(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	err := issue.ValidateParent(id, id, issue.RoleEpic, false)

	// Then
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestValidateParent_DeletedParent_Fails(t *testing.T) {
	t.Parallel()

	// When
	err := issue.ValidateParent(mustID(t), mustID(t), issue.RoleEpic, true)

	// Then
	if !errors.Is(err, domain.ErrDeletedIssue) {
		t.Errorf("expected ErrDeletedIssue, got %v", err)
	}
}

func TestValidateNoCycle_NoCycle_Succeeds(t *testing.T) {
	t.Parallel()

	// Given: A -> B -> C, assigning D as parent of A
	idA := mustID(t)
	idB := mustID(t)
	idC := mustID(t)
	idD := mustID(t)

	parents := map[issue.ID]issue.ID{
		idA: idB,
		idB: idC,
	}

	lookup := func(id issue.ID) (issue.ID, error) {
		return parents[id], nil
	}

	// When
	err := issue.ValidateNoCycle(idA, idD, lookup)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNoCycle_CycleDetected_Fails(t *testing.T) {
	t.Parallel()

	// Given: A -> B -> C, try to make C's parent = A (creates cycle)
	idA := mustID(t)
	idB := mustID(t)
	idC := mustID(t)

	parents := map[issue.ID]issue.ID{
		idA: idB,
		idB: idC,
	}

	lookup := func(id issue.ID) (issue.ID, error) {
		return parents[id], nil
	}

	// When — try to set A as parent of C (A is an ancestor of B which
	// parents C, so C -> A would create a cycle)
	// Actually: we check if childID appears in ancestor chain of proposedParent.
	// So: ValidateNoCycle(childID=C, proposedParent=A, ...) — does C appear
	// in the ancestor chain of A? A -> B -> C — yes, cycle!
	err := issue.ValidateNoCycle(idC, idA, lookup)

	// Then
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestValidateNoCycle_LookupError_Propagates(t *testing.T) {
	t.Parallel()

	// Given
	lookupErr := errors.New("db error")
	lookup := func(_ issue.ID) (issue.ID, error) {
		return issue.ID{}, lookupErr
	}

	// When
	err := issue.ValidateNoCycle(mustID(t), mustID(t), lookup)

	// Then
	if !errors.Is(err, lookupErr) {
		t.Errorf("expected lookup error, got %v", err)
	}
}

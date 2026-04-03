package core_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestValidateParent_ValidEpicParent_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	childID := mustDomainID(t)
	parentID := mustDomainID(t)

	// When
	err := core.ValidateParent(childID, parentID, false)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParent_TaskParent_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	childID := mustDomainID(t)
	parentID := mustDomainID(t)

	// When
	err := core.ValidateParent(childID, parentID, false)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateParent_SelfParent_Fails(t *testing.T) {
	t.Parallel()

	// Given
	id := mustDomainID(t)

	// When
	err := core.ValidateParent(id, id, false)

	// Then
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestValidateParent_DeletedParent_Fails(t *testing.T) {
	t.Parallel()

	// When
	err := core.ValidateParent(mustDomainID(t), mustDomainID(t), true)

	// Then
	if !errors.Is(err, domain.ErrDeletedIssue) {
		t.Errorf("expected ErrDeletedIssue, got %v", err)
	}
}

func TestValidateNoCycle_NoCycle_Succeeds(t *testing.T) {
	t.Parallel()

	// Given: A -> B -> C, assigning D as parent of A
	idA := mustDomainID(t)
	idB := mustDomainID(t)
	idC := mustDomainID(t)
	idD := mustDomainID(t)

	parents := map[domain.ID]domain.ID{
		idA: idB,
		idB: idC,
	}

	lookup := func(id domain.ID) (domain.ID, error) {
		return parents[id], nil
	}

	// When
	err := core.ValidateNoCycle(idA, idD, lookup)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNoCycle_CycleDetected_Fails(t *testing.T) {
	t.Parallel()

	// Given: A -> B -> C, try to make C's parent = A (creates cycle)
	idA := mustDomainID(t)
	idB := mustDomainID(t)
	idC := mustDomainID(t)

	parents := map[domain.ID]domain.ID{
		idA: idB,
		idB: idC,
	}

	lookup := func(id domain.ID) (domain.ID, error) {
		return parents[id], nil
	}

	// When — try to set A as parent of C (A is an ancestor of B which
	// parents C, so C -> A would create a cycle)
	// Actually: we check if childID appears in ancestor chain of proposedParent.
	// So: ValidateNoCycle(childID=C, proposedParent=A, ...) — does C appear
	// in the ancestor chain of A? A -> B -> C — yes, cycle!
	err := core.ValidateNoCycle(idC, idA, lookup)

	// Then
	if !errors.Is(err, domain.ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestValidateNoCycle_LookupError_Propagates(t *testing.T) {
	t.Parallel()

	// Given
	lookupErr := errors.New("db error")
	lookup := func(_ domain.ID) (domain.ID, error) {
		return domain.ID{}, lookupErr
	}

	// When
	err := core.ValidateNoCycle(mustDomainID(t), mustDomainID(t), lookup)

	// Then
	if !errors.Is(err, lookupErr) {
		t.Errorf("expected lookup error, got %v", err)
	}
}

// --- ValidateDepth ---

func TestValidateDepth_RootParent_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — parent is a root (no parent of its own), so child would be level 2.
	parentID := mustDomainID(t)
	lookup := func(_ domain.ID) (domain.ID, error) {
		return domain.ID{}, nil // root — no parent
	}

	// When
	err := core.ValidateDepth(parentID, lookup)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDepth_Level2Parent_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — parent is at level 2 (has one ancestor), so child would be level 3.
	parentID := mustDomainID(t)
	grandparentID := mustDomainID(t)

	lookup := func(id domain.ID) (domain.ID, error) {
		if id == parentID {
			return grandparentID, nil
		}
		return domain.ID{}, nil // grandparent is root
	}

	// When
	err := core.ValidateDepth(parentID, lookup)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDepth_Level3Parent_Fails(t *testing.T) {
	t.Parallel()

	// Given — parent is at level 3 (has two ancestors), so child would be level 4.
	parentID := mustDomainID(t)
	grandparentID := mustDomainID(t)
	greatGrandparentID := mustDomainID(t)

	lookup := func(id domain.ID) (domain.ID, error) {
		switch id {
		case parentID:
			return grandparentID, nil
		case grandparentID:
			return greatGrandparentID, nil
		default:
			return domain.ID{}, nil
		}
	}

	// When
	err := core.ValidateDepth(parentID, lookup)

	// Then
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Errorf("expected ErrDepthExceeded, got %v", err)
	}
}

func TestValidateDepth_LookupError_Propagates(t *testing.T) {
	t.Parallel()

	// Given
	lookupErr := errors.New("db error")
	lookup := func(_ domain.ID) (domain.ID, error) {
		return domain.ID{}, lookupErr
	}

	// When
	err := core.ValidateDepth(mustDomainID(t), lookup)

	// Then
	if !errors.Is(err, lookupErr) {
		t.Errorf("expected lookup error, got %v", err)
	}
}

// --- ValidateEpicDepth ---

func TestValidateEpicDepth_TaskAtLevel3_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — parent is at level 2, so the child would be level 3.
	// A task at level 3 is fine — tasks are leaf nodes.
	parentID := mustDomainID(t)
	grandparentID := mustDomainID(t)

	lookup := func(id domain.ID) (domain.ID, error) {
		if id == parentID {
			return grandparentID, nil
		}
		return domain.ID{}, nil // grandparent is root
	}

	// When
	err := core.ValidateEpicDepth(domain.RoleTask, parentID, lookup)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEpicDepth_EpicAtLevel2_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — parent is root (level 1), so the epic would be level 2.
	// An epic at level 2 can still have children at level 3.
	parentID := mustDomainID(t)
	lookup := func(_ domain.ID) (domain.ID, error) {
		return domain.ID{}, nil // parent is root
	}

	// When
	err := core.ValidateEpicDepth(domain.RoleEpic, parentID, lookup)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEpicDepth_EpicAtLevel3_Fails(t *testing.T) {
	t.Parallel()

	// Given — parent is at level 2 (has one ancestor), so the epic would
	// be at level 3. Level-3 issues cannot have children, so an epic here
	// is structurally useless.
	parentID := mustDomainID(t)
	grandparentID := mustDomainID(t)

	lookup := func(id domain.ID) (domain.ID, error) {
		if id == parentID {
			return grandparentID, nil
		}
		return domain.ID{}, nil // grandparent is root
	}

	// When
	err := core.ValidateEpicDepth(domain.RoleEpic, parentID, lookup)

	// Then
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Errorf("expected ErrDepthExceeded, got %v", err)
	}
}

func TestValidateEpicDepth_EpicWithNoParent_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — no parent means the epic is at level 1 (root).
	// Root epics can always have children.
	// When parentID is zero, ValidateEpicDepth should not be called
	// (it requires a non-zero parent), but a root epic is always valid.
	// This test verifies that ValidateEpicDepth works correctly when
	// the parent is root-level.
	parentID := mustDomainID(t)
	lookup := func(_ domain.ID) (domain.ID, error) {
		return domain.ID{}, nil // parent is root
	}

	// When
	err := core.ValidateEpicDepth(domain.RoleEpic, parentID, lookup)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEpicDepth_LookupError_Propagates(t *testing.T) {
	t.Parallel()

	// Given
	lookupErr := errors.New("db error")
	lookup := func(_ domain.ID) (domain.ID, error) {
		return domain.ID{}, lookupErr
	}

	// When
	err := core.ValidateEpicDepth(domain.RoleEpic, mustDomainID(t), lookup)

	// Then
	if !errors.Is(err, lookupErr) {
		t.Errorf("expected lookup error, got %v", err)
	}
}

package core_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- Open + No Children ---

func TestEpicSecondaryState_OpenNoChildrenNotBlocked_Ready(t *testing.T) {
	t.Parallel()

	// When
	result := core.EpicSecondaryState(domain.StateOpen, false, false, nil, nil)

	// Then
	if result.ListState != domain.SecondaryReady {
		t.Errorf("ListState = %v, want SecondaryReady", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryReady {
		t.Errorf("DetailStates = %v, want [SecondaryReady]", result.DetailStates)
	}
}

func TestEpicSecondaryState_OpenNoChildrenBlocked_Blocked(t *testing.T) {
	t.Parallel()

	// Given — an unresolved blocker.
	blockers := []domain.BlockerStatus{{IsClosed: false, IsDeleted: false}}

	// When
	result := core.EpicSecondaryState(domain.StateOpen, false, false, blockers, nil)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want SecondaryBlocked", result.ListState)
	}
	if len(result.DetailStates) != 2 ||
		result.DetailStates[0] != domain.SecondaryBlocked ||
		result.DetailStates[1] != domain.SecondaryUnplanned {
		t.Errorf("DetailStates = %v, want [SecondaryBlocked, SecondaryUnplanned]", result.DetailStates)
	}
}

// --- Open + Has Children + Not All Closed ---

func TestEpicSecondaryState_OpenHasChildrenNotAllClosedNotBlocked_Active(t *testing.T) {
	t.Parallel()

	// When
	result := core.EpicSecondaryState(domain.StateOpen, true, false, nil, nil)

	// Then
	if result.ListState != domain.SecondaryActive {
		t.Errorf("ListState = %v, want SecondaryActive", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryActive {
		t.Errorf("DetailStates = %v, want [SecondaryActive]", result.DetailStates)
	}
}

func TestEpicSecondaryState_OpenHasChildrenNotAllClosedBlocked_Blocked(t *testing.T) {
	t.Parallel()

	// Given — an unresolved blocker.
	blockers := []domain.BlockerStatus{{IsClosed: false, IsDeleted: false}}

	// When
	result := core.EpicSecondaryState(domain.StateOpen, true, false, blockers, nil)

	// Then — list-view: blocked wins over active.
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want SecondaryBlocked", result.ListState)
	}
	if len(result.DetailStates) != 2 ||
		result.DetailStates[0] != domain.SecondaryBlocked ||
		result.DetailStates[1] != domain.SecondaryActive {
		t.Errorf("DetailStates = %v, want [SecondaryBlocked, SecondaryActive]", result.DetailStates)
	}
}

// --- Open + Has Children + All Closed ---

func TestEpicSecondaryState_OpenAllChildrenClosedNotBlocked_Completed(t *testing.T) {
	t.Parallel()

	// When
	result := core.EpicSecondaryState(domain.StateOpen, true, true, nil, nil)

	// Then
	if result.ListState != domain.SecondaryCompleted {
		t.Errorf("ListState = %v, want SecondaryCompleted", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryCompleted {
		t.Errorf("DetailStates = %v, want [SecondaryCompleted]", result.DetailStates)
	}
}

func TestEpicSecondaryState_OpenAllChildrenClosedBlocked_Completed(t *testing.T) {
	t.Parallel()

	// Given — an unresolved blocker.
	blockers := []domain.BlockerStatus{{IsClosed: false, IsDeleted: false}}

	// When — completed wins over blocked in list-view.
	result := core.EpicSecondaryState(domain.StateOpen, true, true, blockers, nil)

	// Then
	if result.ListState != domain.SecondaryCompleted {
		t.Errorf("ListState = %v, want SecondaryCompleted", result.ListState)
	}
	if len(result.DetailStates) != 2 ||
		result.DetailStates[0] != domain.SecondaryBlocked ||
		result.DetailStates[1] != domain.SecondaryCompleted {
		t.Errorf("DetailStates = %v, want [SecondaryBlocked, SecondaryCompleted]", result.DetailStates)
	}
}

// --- Deferred ---

func TestEpicSecondaryState_DeferredBlocked_Blocked(t *testing.T) {
	t.Parallel()

	// Given — deferred state with an unresolved blocker.
	blockers := []domain.BlockerStatus{{IsClosed: false, IsDeleted: false}}

	// When
	result := core.EpicSecondaryState(domain.StateDeferred, false, false, blockers, nil)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want SecondaryBlocked", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryBlocked {
		t.Errorf("DetailStates = %v, want [SecondaryBlocked]", result.DetailStates)
	}
}

func TestEpicSecondaryState_DeferredNotBlocked_None(t *testing.T) {
	t.Parallel()

	// When
	result := core.EpicSecondaryState(domain.StateDeferred, false, false, nil, nil)

	// Then
	if result.ListState != domain.SecondaryNone {
		t.Errorf("ListState = %v, want SecondaryNone", result.ListState)
	}
	if len(result.DetailStates) != 0 {
		t.Errorf("DetailStates length = %d, want 0", len(result.DetailStates))
	}
}

// --- Claimed and Closed (no secondary state) ---

func TestEpicSecondaryState_Claimed_None(t *testing.T) {
	t.Parallel()

	// When
	result := core.EpicSecondaryState(domain.StateClaimed, false, false, nil, nil)

	// Then
	if result.HasSecondary() {
		t.Errorf("expected no secondary state for claimed, got ListState=%v", result.ListState)
	}
}

func TestEpicSecondaryState_Closed_None(t *testing.T) {
	t.Parallel()

	// When
	result := core.EpicSecondaryState(domain.StateClosed, false, false, nil, nil)

	// Then
	if result.HasSecondary() {
		t.Errorf("expected no secondary state for closed, got ListState=%v", result.ListState)
	}
}

// --- Ancestor-based blocking ---

func TestEpicSecondaryState_OpenNoChildrenBlockedAncestor_Blocked(t *testing.T) {
	t.Parallel()

	// Given — a blocked ancestor (no direct blockers).
	ancestors := []domain.AncestorStatus{{State: domain.StateOpen, IsBlocked: true}}

	// When
	result := core.EpicSecondaryState(domain.StateOpen, false, false, nil, ancestors)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want SecondaryBlocked", result.ListState)
	}
	if len(result.DetailStates) != 2 ||
		result.DetailStates[0] != domain.SecondaryBlocked ||
		result.DetailStates[1] != domain.SecondaryUnplanned {
		t.Errorf("DetailStates = %v, want [SecondaryBlocked, SecondaryUnplanned]", result.DetailStates)
	}
}

func TestEpicSecondaryState_OpenNoChildrenDeferredAncestor_Blocked(t *testing.T) {
	t.Parallel()

	// Given — a deferred ancestor (no direct blockers).
	ancestors := []domain.AncestorStatus{{State: domain.StateDeferred}}

	// When
	result := core.EpicSecondaryState(domain.StateOpen, false, false, nil, ancestors)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want SecondaryBlocked", result.ListState)
	}
}

func TestEpicSecondaryState_OpenHasChildrenBlockedAncestor_Blocked(t *testing.T) {
	t.Parallel()

	// Given — an epic with children and a blocked ancestor.
	ancestors := []domain.AncestorStatus{{State: domain.StateOpen, IsBlocked: true}}

	// When
	result := core.EpicSecondaryState(domain.StateOpen, true, false, nil, ancestors)

	// Then — blocked takes priority over active in list-view.
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want SecondaryBlocked", result.ListState)
	}
	if len(result.DetailStates) != 2 ||
		result.DetailStates[0] != domain.SecondaryBlocked ||
		result.DetailStates[1] != domain.SecondaryActive {
		t.Errorf("DetailStates = %v, want [SecondaryBlocked, SecondaryActive]", result.DetailStates)
	}
}

// --- Resolved blockers (should not count as blocked) ---

func TestEpicSecondaryState_OpenNoChildrenResolvedBlockers_Ready(t *testing.T) {
	t.Parallel()

	// Given — only resolved blockers (closed and deleted).
	blockers := []domain.BlockerStatus{
		{IsClosed: true, IsDeleted: false},
		{IsClosed: false, IsDeleted: true},
	}

	// When
	result := core.EpicSecondaryState(domain.StateOpen, false, false, blockers, nil)

	// Then
	if result.ListState != domain.SecondaryReady {
		t.Errorf("ListState = %v, want SecondaryReady", result.ListState)
	}
}

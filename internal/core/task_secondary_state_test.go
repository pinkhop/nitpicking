package core_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestTaskSecondaryState_OpenNotBlocked_Ready(t *testing.T) {
	t.Parallel()

	// Given — open task, no active claim, no blockers, no blocked/deferred ancestors.

	// When
	result := core.TaskSecondaryState(domain.StateOpen, false, nil, nil)

	// Then
	if result.ListState != domain.SecondaryReady {
		t.Errorf("ListState = %v, want ready", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryReady {
		t.Errorf("DetailStates = %v, want [ready]", result.DetailStates)
	}
}

func TestTaskSecondaryState_OpenWithActiveClaim_Claimed(t *testing.T) {
	t.Parallel()

	// Given — open task with an active (non-stale) claim.

	// When — claimed takes priority over ready.
	result := core.TaskSecondaryState(domain.StateOpen, true, nil, nil)

	// Then
	if result.ListState != domain.SecondaryClaimed {
		t.Errorf("ListState = %v, want claimed", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryClaimed {
		t.Errorf("DetailStates = %v, want [claimed]", result.DetailStates)
	}
}

func TestTaskSecondaryState_OpenWithActiveClaimAndBlocker_Claimed(t *testing.T) {
	t.Parallel()

	// Given — open task with an active claim and an unresolved blocker.
	// Claimed takes priority over blocked in display — the issue is being worked on.
	blockers := []domain.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := core.TaskSecondaryState(domain.StateOpen, true, blockers, nil)

	// Then — claimed wins over blocked.
	if result.ListState != domain.SecondaryClaimed {
		t.Errorf("ListState = %v, want claimed", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryClaimed {
		t.Errorf("DetailStates = %v, want [claimed]", result.DetailStates)
	}
}

func TestTaskSecondaryState_OpenWithUnresolvedBlocker_Blocked(t *testing.T) {
	t.Parallel()

	// Given — open task with an unresolved blocker.
	blockers := []domain.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := core.TaskSecondaryState(domain.StateOpen, false, blockers, nil)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want blocked", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryBlocked {
		t.Errorf("DetailStates = %v, want [blocked]", result.DetailStates)
	}
}

func TestTaskSecondaryState_OpenWithBlockedAncestor_Blocked(t *testing.T) {
	t.Parallel()

	// Given — open task with a blocked ancestor.
	ancestors := []domain.AncestorStatus{
		{State: domain.StateOpen, IsBlocked: true},
	}

	// When
	result := core.TaskSecondaryState(domain.StateOpen, false, nil, ancestors)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want blocked", result.ListState)
	}
}

func TestTaskSecondaryState_OpenWithDeferredAncestor_Blocked(t *testing.T) {
	t.Parallel()

	// Given — open task with a deferred ancestor.
	ancestors := []domain.AncestorStatus{
		{State: domain.StateDeferred, IsBlocked: false},
	}

	// When
	result := core.TaskSecondaryState(domain.StateOpen, false, nil, ancestors)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want blocked", result.ListState)
	}
}

func TestTaskSecondaryState_OpenWithResolvedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given — open task with a resolved (closed) blocker.
	blockers := []domain.BlockerStatus{
		{IsClosed: true, IsDeleted: false},
	}

	// When
	result := core.TaskSecondaryState(domain.StateOpen, false, blockers, nil)

	// Then
	if result.ListState != domain.SecondaryReady {
		t.Errorf("ListState = %v, want ready", result.ListState)
	}
}

func TestTaskSecondaryState_DeferredAndBlocked_Blocked(t *testing.T) {
	t.Parallel()

	// Given — deferred task with an unresolved blocker.
	blockers := []domain.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := core.TaskSecondaryState(domain.StateDeferred, false, blockers, nil)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want blocked", result.ListState)
	}
	if len(result.DetailStates) != 1 || result.DetailStates[0] != domain.SecondaryBlocked {
		t.Errorf("DetailStates = %v, want [blocked]", result.DetailStates)
	}
}

func TestTaskSecondaryState_DeferredNotBlocked_None(t *testing.T) {
	t.Parallel()

	// Given — deferred task with no blockers.

	// When
	result := core.TaskSecondaryState(domain.StateDeferred, false, nil, nil)

	// Then
	if result.ListState != domain.SecondaryNone {
		t.Errorf("ListState = %v, want none", result.ListState)
	}
	if len(result.DetailStates) != 0 {
		t.Errorf("DetailStates = %v, want empty", result.DetailStates)
	}
}

func TestTaskSecondaryState_Closed_None(t *testing.T) {
	t.Parallel()

	// When
	result := core.TaskSecondaryState(domain.StateClosed, false, nil, nil)

	// Then
	if result.ListState != domain.SecondaryNone {
		t.Errorf("ListState = %v, want none", result.ListState)
	}
	if len(result.DetailStates) != 0 {
		t.Errorf("DetailStates = %v, want empty", result.DetailStates)
	}
}

func TestTaskSecondaryState_DeferredWithBlockedAncestor_Blocked(t *testing.T) {
	t.Parallel()

	// Given — deferred task with a blocked ancestor.
	ancestors := []domain.AncestorStatus{
		{State: domain.StateOpen, IsBlocked: true},
	}

	// When
	result := core.TaskSecondaryState(domain.StateDeferred, false, nil, ancestors)

	// Then
	if result.ListState != domain.SecondaryBlocked {
		t.Errorf("ListState = %v, want blocked", result.ListState)
	}
}

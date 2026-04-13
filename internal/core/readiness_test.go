package core_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestIsTaskReady_OpenNoBlockersNoAncestors_Ready(t *testing.T) {
	t.Parallel()

	// When
	result := core.IsTaskReady(domain.StateOpen, nil, nil)

	// Then
	if !result {
		t.Error("expected ready")
	}
}

func TestIsTaskReady_NotOpen_NotReady(t *testing.T) {
	t.Parallel()

	cases := []domain.State{
		domain.StateClosed,
		domain.StateDeferred,
	}

	for _, state := range cases {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			// When
			result := core.IsTaskReady(state, nil, nil)

			// Then
			if result {
				t.Errorf("expected not ready for state %s", state)
			}
		})
	}
}

func TestIsTaskReady_UnresolvedBlocker_NotReady(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []domain.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := core.IsTaskReady(domain.StateOpen, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready with unresolved blocker")
	}
}

func TestIsTaskReady_ClosedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []domain.BlockerStatus{
		{IsClosed: true, IsDeleted: false},
	}

	// When
	result := core.IsTaskReady(domain.StateOpen, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready with closed blocker")
	}
}

func TestIsTaskReady_DeletedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []domain.BlockerStatus{
		{IsClosed: false, IsDeleted: true},
	}

	// When
	result := core.IsTaskReady(domain.StateOpen, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready with deleted blocker")
	}
}

func TestIsTaskReady_DeferredAncestor_NotReady(t *testing.T) {
	t.Parallel()

	// Given
	ancestors := []domain.AncestorStatus{
		{State: domain.StateDeferred},
	}

	// When
	result := core.IsTaskReady(domain.StateOpen, nil, ancestors)

	// Then
	if result {
		t.Error("expected not ready with deferred ancestor")
	}
}

func TestIsEpicReady_ActiveNoChildrenNoBlockers_Ready(t *testing.T) {
	t.Parallel()

	// When
	result := core.IsEpicReady(domain.StateOpen, false, nil, nil)

	// Then
	if !result {
		t.Error("expected ready")
	}
}

func TestIsEpicReady_HasChildren_NotReady(t *testing.T) {
	t.Parallel()

	// When
	result := core.IsEpicReady(domain.StateOpen, true, nil, nil)

	// Then
	if result {
		t.Error("expected not ready with children")
	}
}

func TestIsEpicReady_NotActive_NotReady(t *testing.T) {
	t.Parallel()

	cases := []domain.State{
		domain.StateDeferred,
	}

	for _, state := range cases {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			// When
			result := core.IsEpicReady(state, false, nil, nil)

			// Then
			if result {
				t.Errorf("expected not ready for state %s", state)
			}
		})
	}
}

func TestIsEpicReady_UnresolvedBlocker_NotReady(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []domain.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := core.IsEpicReady(domain.StateOpen, false, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready with unresolved blocker")
	}
}

func TestIsTaskReady_BlockedAncestor_NotReady(t *testing.T) {
	t.Parallel()

	// Given — an ancestor that is blocked.
	ancestors := []domain.AncestorStatus{
		{State: domain.StateOpen, IsBlocked: true},
	}

	// When
	result := core.IsTaskReady(domain.StateOpen, nil, ancestors)

	// Then
	if result {
		t.Error("expected not ready with blocked ancestor")
	}
}

func TestIsEpicReady_BlockedAncestor_NotReady(t *testing.T) {
	t.Parallel()

	// Given — an ancestor that is blocked.
	ancestors := []domain.AncestorStatus{
		{State: domain.StateOpen, IsBlocked: true},
	}

	// When
	result := core.IsEpicReady(domain.StateOpen, false, nil, ancestors)

	// Then
	if result {
		t.Error("expected not ready with blocked ancestor")
	}
}

func TestIsTaskReady_AncestorBlockedByResolvedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given — an ancestor that is not blocked (blocker was resolved).
	ancestors := []domain.AncestorStatus{
		{State: domain.StateOpen, IsBlocked: false},
	}

	// When
	result := core.IsTaskReady(domain.StateOpen, nil, ancestors)

	// Then
	if !result {
		t.Error("expected ready when ancestor's blocker is resolved")
	}
}

func TestIsTaskReady_OpenBlocker_NotReady(t *testing.T) {
	t.Parallel()

	// Given — a blocker that is neither closed nor deleted.
	blockers := []domain.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := core.IsTaskReady(domain.StateOpen, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready when blocker is open")
	}
}

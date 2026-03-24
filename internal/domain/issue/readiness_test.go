package issue_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestIsTaskReady_OpenNoBlockersNoAncestors_Ready(t *testing.T) {
	t.Parallel()

	// When
	result := issue.IsTaskReady(issue.StateOpen, nil, nil)

	// Then
	if !result {
		t.Error("expected ready")
	}
}

func TestIsTaskReady_NotOpen_NotReady(t *testing.T) {
	t.Parallel()

	cases := []issue.State{
		issue.StateClaimed,
		issue.StateClosed,
		issue.StateDeferred,
		issue.StateWaiting,
	}

	for _, state := range cases {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			// When
			result := issue.IsTaskReady(state, nil, nil)

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
	blockers := []issue.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := issue.IsTaskReady(issue.StateOpen, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready with unresolved blocker")
	}
}

func TestIsTaskReady_ClosedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []issue.BlockerStatus{
		{IsClosed: true, IsDeleted: false},
	}

	// When
	result := issue.IsTaskReady(issue.StateOpen, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready with closed blocker")
	}
}

func TestIsTaskReady_DeletedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []issue.BlockerStatus{
		{IsClosed: false, IsDeleted: true},
	}

	// When
	result := issue.IsTaskReady(issue.StateOpen, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready with deleted blocker")
	}
}

func TestIsTaskReady_DeferredAncestor_NotReady(t *testing.T) {
	t.Parallel()

	// Given
	ancestors := []issue.AncestorStatus{
		{State: issue.StateDeferred},
	}

	// When
	result := issue.IsTaskReady(issue.StateOpen, nil, ancestors)

	// Then
	if result {
		t.Error("expected not ready with deferred ancestor")
	}
}

func TestIsTaskReady_WaitingAncestor_NotReady(t *testing.T) {
	t.Parallel()

	// Given
	ancestors := []issue.AncestorStatus{
		{State: issue.StateWaiting},
	}

	// When
	result := issue.IsTaskReady(issue.StateOpen, nil, ancestors)

	// Then
	if result {
		t.Error("expected not ready with waiting ancestor")
	}
}

func TestIsEpicReady_ActiveNoChildrenNoBlockers_Ready(t *testing.T) {
	t.Parallel()

	// When
	result := issue.IsEpicReady(issue.StateActive, false, nil, nil)

	// Then
	if !result {
		t.Error("expected ready")
	}
}

func TestIsEpicReady_HasChildren_NotReady(t *testing.T) {
	t.Parallel()

	// When
	result := issue.IsEpicReady(issue.StateActive, true, nil, nil)

	// Then
	if result {
		t.Error("expected not ready with children")
	}
}

func TestIsEpicReady_NotActive_NotReady(t *testing.T) {
	t.Parallel()

	cases := []issue.State{
		issue.StateClaimed,
		issue.StateDeferred,
		issue.StateWaiting,
	}

	for _, state := range cases {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			// When
			result := issue.IsEpicReady(state, false, nil, nil)

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
	blockers := []issue.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := issue.IsEpicReady(issue.StateActive, false, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready with unresolved blocker")
	}
}

func TestIsTaskReady_CompleteEpicBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given — a blocker that is not closed and not deleted, but is a complete epic.
	blockers := []issue.BlockerStatus{
		{IsClosed: false, IsDeleted: false, IsComplete: true},
	}

	// When
	result := issue.IsTaskReady(issue.StateOpen, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready when epic blocker is complete")
	}
}

func TestIsEpicReady_CompleteEpicBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given — a blocker that is a complete epic (not closed, not deleted).
	blockers := []issue.BlockerStatus{
		{IsClosed: false, IsDeleted: false, IsComplete: true},
	}

	// When
	result := issue.IsEpicReady(issue.StateActive, false, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready when epic blocker is complete")
	}
}

func TestIsTaskReady_IncompleteEpicBlocker_NotReady(t *testing.T) {
	t.Parallel()

	// Given — a blocker that is an incomplete epic.
	blockers := []issue.BlockerStatus{
		{IsClosed: false, IsDeleted: false, IsComplete: false},
	}

	// When
	result := issue.IsTaskReady(issue.StateOpen, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready when epic blocker is incomplete")
	}
}

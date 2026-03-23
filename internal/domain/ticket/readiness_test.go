package ticket_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestIsTaskReady_OpenNoBlockersNoAncestors_Ready(t *testing.T) {
	t.Parallel()

	// When
	result := ticket.IsTaskReady(ticket.StateOpen, nil, nil)

	// Then
	if !result {
		t.Error("expected ready")
	}
}

func TestIsTaskReady_NotOpen_NotReady(t *testing.T) {
	t.Parallel()

	cases := []ticket.State{
		ticket.StateClaimed,
		ticket.StateClosed,
		ticket.StateDeferred,
		ticket.StateWaiting,
	}

	for _, state := range cases {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			// When
			result := ticket.IsTaskReady(state, nil, nil)

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
	blockers := []ticket.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := ticket.IsTaskReady(ticket.StateOpen, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready with unresolved blocker")
	}
}

func TestIsTaskReady_ClosedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []ticket.BlockerStatus{
		{IsClosed: true, IsDeleted: false},
	}

	// When
	result := ticket.IsTaskReady(ticket.StateOpen, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready with closed blocker")
	}
}

func TestIsTaskReady_DeletedBlocker_Ready(t *testing.T) {
	t.Parallel()

	// Given
	blockers := []ticket.BlockerStatus{
		{IsClosed: false, IsDeleted: true},
	}

	// When
	result := ticket.IsTaskReady(ticket.StateOpen, blockers, nil)

	// Then
	if !result {
		t.Error("expected ready with deleted blocker")
	}
}

func TestIsTaskReady_DeferredAncestor_NotReady(t *testing.T) {
	t.Parallel()

	// Given
	ancestors := []ticket.AncestorStatus{
		{State: ticket.StateDeferred},
	}

	// When
	result := ticket.IsTaskReady(ticket.StateOpen, nil, ancestors)

	// Then
	if result {
		t.Error("expected not ready with deferred ancestor")
	}
}

func TestIsTaskReady_WaitingAncestor_NotReady(t *testing.T) {
	t.Parallel()

	// Given
	ancestors := []ticket.AncestorStatus{
		{State: ticket.StateWaiting},
	}

	// When
	result := ticket.IsTaskReady(ticket.StateOpen, nil, ancestors)

	// Then
	if result {
		t.Error("expected not ready with waiting ancestor")
	}
}

func TestIsEpicReady_ActiveNoChildrenNoBlockers_Ready(t *testing.T) {
	t.Parallel()

	// When
	result := ticket.IsEpicReady(ticket.StateActive, false, nil, nil)

	// Then
	if !result {
		t.Error("expected ready")
	}
}

func TestIsEpicReady_HasChildren_NotReady(t *testing.T) {
	t.Parallel()

	// When
	result := ticket.IsEpicReady(ticket.StateActive, true, nil, nil)

	// Then
	if result {
		t.Error("expected not ready with children")
	}
}

func TestIsEpicReady_NotActive_NotReady(t *testing.T) {
	t.Parallel()

	cases := []ticket.State{
		ticket.StateClaimed,
		ticket.StateDeferred,
		ticket.StateWaiting,
	}

	for _, state := range cases {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			// When
			result := ticket.IsEpicReady(state, false, nil, nil)

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
	blockers := []ticket.BlockerStatus{
		{IsClosed: false, IsDeleted: false},
	}

	// When
	result := ticket.IsEpicReady(ticket.StateActive, false, blockers, nil)

	// Then
	if result {
		t.Error("expected not ready with unresolved blocker")
	}
}

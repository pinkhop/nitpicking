package ticket_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestIsEpicComplete_NoChildren_Incomplete(t *testing.T) {
	t.Parallel()

	// When
	result := ticket.IsEpicComplete(nil)

	// Then
	if result {
		t.Error("expected epic with no children to be incomplete")
	}
}

func TestIsEpicComplete_AllTasksClosed_Complete(t *testing.T) {
	t.Parallel()

	// Given
	children := []ticket.ChildStatus{
		{Role: ticket.RoleTask, State: ticket.StateClosed},
		{Role: ticket.RoleTask, State: ticket.StateClosed},
	}

	// When
	result := ticket.IsEpicComplete(children)

	// Then
	if !result {
		t.Error("expected complete when all tasks closed")
	}
}

func TestIsEpicComplete_OpenTask_Incomplete(t *testing.T) {
	t.Parallel()

	// Given
	children := []ticket.ChildStatus{
		{Role: ticket.RoleTask, State: ticket.StateClosed},
		{Role: ticket.RoleTask, State: ticket.StateOpen},
	}

	// When
	result := ticket.IsEpicComplete(children)

	// Then
	if result {
		t.Error("expected incomplete when any task not closed")
	}
}

func TestIsEpicComplete_CompleteSubEpics_Complete(t *testing.T) {
	t.Parallel()

	// Given
	children := []ticket.ChildStatus{
		{Role: ticket.RoleEpic, IsComplete: true},
		{Role: ticket.RoleTask, State: ticket.StateClosed},
	}

	// When
	result := ticket.IsEpicComplete(children)

	// Then
	if !result {
		t.Error("expected complete with all children done")
	}
}

func TestIsEpicComplete_IncompleteSubEpic_Incomplete(t *testing.T) {
	t.Parallel()

	// Given
	children := []ticket.ChildStatus{
		{Role: ticket.RoleEpic, IsComplete: false},
		{Role: ticket.RoleTask, State: ticket.StateClosed},
	}

	// When
	result := ticket.IsEpicComplete(children)

	// Then
	if result {
		t.Error("expected incomplete when sub-epic not complete")
	}
}

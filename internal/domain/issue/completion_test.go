package issue_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestIsEpicComplete_NoChildren_Incomplete(t *testing.T) {
	t.Parallel()

	// When
	result := issue.IsEpicComplete(nil)

	// Then
	if result {
		t.Error("expected epic with no children to be incomplete")
	}
}

func TestIsEpicComplete_AllTasksClosed_Complete(t *testing.T) {
	t.Parallel()

	// Given
	children := []issue.ChildStatus{
		{Role: issue.RoleTask, State: issue.StateClosed},
		{Role: issue.RoleTask, State: issue.StateClosed},
	}

	// When
	result := issue.IsEpicComplete(children)

	// Then
	if !result {
		t.Error("expected complete when all tasks closed")
	}
}

func TestIsEpicComplete_OpenTask_Incomplete(t *testing.T) {
	t.Parallel()

	// Given
	children := []issue.ChildStatus{
		{Role: issue.RoleTask, State: issue.StateClosed},
		{Role: issue.RoleTask, State: issue.StateOpen},
	}

	// When
	result := issue.IsEpicComplete(children)

	// Then
	if result {
		t.Error("expected incomplete when any task not closed")
	}
}

func TestIsEpicComplete_CompleteSubEpics_Complete(t *testing.T) {
	t.Parallel()

	// Given
	children := []issue.ChildStatus{
		{Role: issue.RoleEpic, IsComplete: true},
		{Role: issue.RoleTask, State: issue.StateClosed},
	}

	// When
	result := issue.IsEpicComplete(children)

	// Then
	if !result {
		t.Error("expected complete with all children done")
	}
}

func TestIsEpicComplete_IncompleteSubEpic_Incomplete(t *testing.T) {
	t.Parallel()

	// Given
	children := []issue.ChildStatus{
		{Role: issue.RoleEpic, IsComplete: false},
		{Role: issue.RoleTask, State: issue.StateClosed},
	}

	// When
	result := issue.IsEpicComplete(children)

	// Then
	if result {
		t.Error("expected incomplete when sub-epic not complete")
	}
}

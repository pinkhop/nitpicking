package issue_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestTransitionTask_LegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current issue.State
		next    issue.State
	}{
		{"open to claimed", issue.StateOpen, issue.StateClaimed},
		{"claimed to open", issue.StateClaimed, issue.StateOpen},
		{"claimed to closed", issue.StateClaimed, issue.StateClosed},
		{"claimed to deferred", issue.StateClaimed, issue.StateDeferred},
		{"claimed to waiting", issue.StateClaimed, issue.StateWaiting},
		{"deferred to claimed", issue.StateDeferred, issue.StateClaimed},
		{"waiting to claimed", issue.StateWaiting, issue.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := issue.TransitionTask(tc.current, tc.next)
			// Then
			if err != nil {
				t.Errorf("expected legal transition, got error: %v", err)
			}
		})
	}
}

func TestTransitionTask_IllegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current issue.State
		next    issue.State
	}{
		{"open to closed", issue.StateOpen, issue.StateClosed},
		{"open to deferred", issue.StateOpen, issue.StateDeferred},
		{"deferred to open", issue.StateDeferred, issue.StateOpen},
		{"waiting to open", issue.StateWaiting, issue.StateOpen},
		{"claimed to claimed", issue.StateClaimed, issue.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := issue.TransitionTask(tc.current, tc.next)

			// Then
			if !errors.Is(err, domain.ErrIllegalTransition) {
				t.Errorf("expected ErrIllegalTransition, got %v", err)
			}
		})
	}
}

func TestTransitionTask_FromClosed_ReturnsTerminalState(t *testing.T) {
	t.Parallel()

	// When
	err := issue.TransitionTask(issue.StateClosed, issue.StateClaimed)

	// Then
	if !errors.Is(err, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState, got %v", err)
	}
}

func TestTransitionEpic_LegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current issue.State
		next    issue.State
	}{
		{"active to claimed", issue.StateActive, issue.StateClaimed},
		{"claimed to active", issue.StateClaimed, issue.StateActive},
		{"claimed to deferred", issue.StateClaimed, issue.StateDeferred},
		{"claimed to waiting", issue.StateClaimed, issue.StateWaiting},
		{"deferred to claimed", issue.StateDeferred, issue.StateClaimed},
		{"waiting to claimed", issue.StateWaiting, issue.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := issue.TransitionEpic(tc.current, tc.next)
			// Then
			if err != nil {
				t.Errorf("expected legal transition, got error: %v", err)
			}
		})
	}
}

func TestTransitionEpic_IllegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current issue.State
		next    issue.State
	}{
		{"active to deferred", issue.StateActive, issue.StateDeferred},
		{"active to waiting", issue.StateActive, issue.StateWaiting},
		{"claimed to closed", issue.StateClaimed, issue.StateClosed},
		{"deferred to active", issue.StateDeferred, issue.StateActive},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := issue.TransitionEpic(tc.current, tc.next)

			// Then
			if !errors.Is(err, domain.ErrIllegalTransition) {
				t.Errorf("expected ErrIllegalTransition, got %v", err)
			}
		})
	}
}

func TestDefaultStateForRole_Task_ReturnsOpen(t *testing.T) {
	t.Parallel()

	// When
	s := issue.DefaultStateForRole(issue.RoleTask)

	// Then
	if s != issue.StateOpen {
		t.Errorf("expected open, got %s", s)
	}
}

func TestDefaultStateForRole_Epic_ReturnsActive(t *testing.T) {
	t.Parallel()

	// When
	s := issue.DefaultStateForRole(issue.RoleEpic)

	// Then
	if s != issue.StateActive {
		t.Errorf("expected active, got %s", s)
	}
}

func TestParseState_ValidStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected issue.State
	}{
		{"open", issue.StateOpen},
		{"active", issue.StateActive},
		{"claimed", issue.StateClaimed},
		{"closed", issue.StateClosed},
		{"deferred", issue.StateDeferred},
		{"waiting", issue.StateWaiting},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			s, err := issue.ParseState(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, s)
			}
		})
	}
}

func TestParseState_InvalidState_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := issue.ParseState("invalid")

	// Then
	if err == nil {
		t.Error("expected error for invalid state")
	}
}

func TestState_IsTerminal(t *testing.T) {
	t.Parallel()

	// Then
	if !issue.StateClosed.IsTerminal() {
		t.Error("expected closed to be terminal")
	}
	if issue.StateOpen.IsTerminal() {
		t.Error("expected open to not be terminal")
	}
	if issue.StateClaimed.IsTerminal() {
		t.Error("expected claimed to not be terminal")
	}
}

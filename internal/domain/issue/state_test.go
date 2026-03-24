package issue_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestTransition_LegalTransitions(t *testing.T) {
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
		{"deferred to claimed", issue.StateDeferred, issue.StateClaimed},
		{"closed to claimed", issue.StateClosed, issue.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := issue.Transition(tc.current, tc.next)
			// Then
			if err != nil {
				t.Errorf("expected legal transition, got error: %v", err)
			}
		})
	}
}

func TestTransition_IllegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current issue.State
		next    issue.State
	}{
		{"open to closed", issue.StateOpen, issue.StateClosed},
		{"open to deferred", issue.StateOpen, issue.StateDeferred},
		{"deferred to open", issue.StateDeferred, issue.StateOpen},
		{"claimed to claimed", issue.StateClaimed, issue.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := issue.Transition(tc.current, tc.next)

			// Then
			if !errors.Is(err, domain.ErrIllegalTransition) {
				t.Errorf("expected ErrIllegalTransition, got %v", err)
			}
		})
	}
}

func TestTransition_ClosedToOpen_IllegalDirectly(t *testing.T) {
	t.Parallel()

	// Closed issues must be claimed before transitioning — direct
	// closed→open is not allowed.

	// When
	err := issue.Transition(issue.StateClosed, issue.StateOpen)

	// Then
	if !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition, got %v", err)
	}
}

func TestDefaultState_ReturnsOpen(t *testing.T) {
	t.Parallel()

	// When
	s := issue.DefaultState()

	// Then
	if s != issue.StateOpen {
		t.Errorf("expected open, got %s", s)
	}
}

func TestParseState_ValidStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected issue.State
	}{
		{"open", issue.StateOpen},
		{"claimed", issue.StateClaimed},
		{"closed", issue.StateClosed},
		{"deferred", issue.StateDeferred},
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

func TestState_IsTerminal_NoStatesAreTerminal(t *testing.T) {
	t.Parallel()

	// No states are terminal — all states can be transitioned out of.
	states := []issue.State{
		issue.StateOpen,
		issue.StateClaimed,
		issue.StateClosed,
		issue.StateDeferred,
	}
	for _, s := range states {
		if s.IsTerminal() {
			t.Errorf("expected %s to not be terminal", s)
		}
	}
}

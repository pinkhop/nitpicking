package domain_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestTransition_LegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current domain.State
		next    domain.State
	}{
		{"open to claimed", domain.StateOpen, domain.StateClaimed},
		{"claimed to open", domain.StateClaimed, domain.StateOpen},
		{"claimed to closed", domain.StateClaimed, domain.StateClosed},
		{"claimed to deferred", domain.StateClaimed, domain.StateDeferred},
		{"deferred to claimed", domain.StateDeferred, domain.StateClaimed},
		{"closed to claimed", domain.StateClosed, domain.StateClaimed},
		{"open to deferred", domain.StateOpen, domain.StateDeferred},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := domain.Transition(tc.current, tc.next)
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
		current domain.State
		next    domain.State
	}{
		{"open to closed", domain.StateOpen, domain.StateClosed},
		{"deferred to open", domain.StateDeferred, domain.StateOpen},
		{"claimed to claimed", domain.StateClaimed, domain.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := domain.Transition(tc.current, tc.next)

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
	err := domain.Transition(domain.StateClosed, domain.StateOpen)

	// Then
	if !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition, got %v", err)
	}
}

func TestDefaultState_ReturnsOpen(t *testing.T) {
	t.Parallel()

	// When
	s := domain.DefaultState()

	// Then
	if s != domain.StateOpen {
		t.Errorf("expected open, got %s", s)
	}
}

func TestParseState_ValidStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected domain.State
	}{
		{"open", domain.StateOpen},
		{"claimed", domain.StateClaimed},
		{"closed", domain.StateClosed},
		{"deferred", domain.StateDeferred},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			s, err := domain.ParseState(tc.input)
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
	_, err := domain.ParseState("invalid")

	// Then
	if err == nil {
		t.Error("expected error for invalid state")
	}
}

func TestReleaseState_ReturnsOpen(t *testing.T) {
	t.Parallel()

	// When
	s := domain.ReleaseState()

	// Then
	if s != domain.StateOpen {
		t.Errorf("expected open, got %s", s)
	}
}

func TestReleaseState_ConsistentWithTransitionTable(t *testing.T) {
	t.Parallel()

	// Given — ReleaseState is used when transitioning from claimed
	releaseTarget := domain.ReleaseState()

	// When — verify the claimed→releaseTarget transition is legal
	err := domain.Transition(domain.StateClaimed, releaseTarget)
	// Then
	if err != nil {
		t.Errorf("expected claimed→%s to be legal, got error: %v", releaseTarget, err)
	}
}

func TestState_IsTerminal_NoStatesAreTerminal(t *testing.T) {
	t.Parallel()

	// No states are terminal — all states can be transitioned out of.
	states := []domain.State{
		domain.StateOpen,
		domain.StateClaimed,
		domain.StateClosed,
		domain.StateDeferred,
	}
	for _, s := range states {
		if s.IsTerminal() {
			t.Errorf("expected %s to not be terminal", s)
		}
	}
}

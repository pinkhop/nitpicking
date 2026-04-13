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
		{"open to closed", domain.StateOpen, domain.StateClosed},
		{"open to deferred", domain.StateOpen, domain.StateDeferred},
		{"closed to open", domain.StateClosed, domain.StateOpen},
		{"deferred to open", domain.StateDeferred, domain.StateOpen},
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
		{"open to open", domain.StateOpen, domain.StateOpen},
		{"closed to closed", domain.StateClosed, domain.StateClosed},
		{"closed to deferred", domain.StateClosed, domain.StateDeferred},
		{"deferred to closed", domain.StateDeferred, domain.StateClosed},
		{"deferred to deferred", domain.StateDeferred, domain.StateDeferred},
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

func TestTransition_ClosedToOpen_LegalDirectly(t *testing.T) {
	t.Parallel()

	// Closed issues transition directly to open (reopen) without
	// requiring an intermediate claimed state.

	// When
	err := domain.Transition(domain.StateClosed, domain.StateOpen)
	// Then
	if err != nil {
		t.Errorf("expected legal transition, got error: %v", err)
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

func TestParseState_Claimed_Fails(t *testing.T) {
	t.Parallel()

	// "claimed" is no longer a valid lifecycle state; parsing it must fail.

	// When
	_, err := domain.ParseState("claimed")

	// Then
	if err == nil {
		t.Error("expected error when parsing \"claimed\" as a state")
	}
}

func TestState_IsTerminal_NoStatesAreTerminal(t *testing.T) {
	t.Parallel()

	// No states are terminal — all states can be transitioned out of.
	states := []domain.State{
		domain.StateOpen,
		domain.StateClosed,
		domain.StateDeferred,
	}
	for _, s := range states {
		if s.IsTerminal() {
			t.Errorf("expected %s to not be terminal", s)
		}
	}
}

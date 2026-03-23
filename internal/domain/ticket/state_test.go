package ticket_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestTransitionTask_LegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current ticket.State
		next    ticket.State
	}{
		{"open to claimed", ticket.StateOpen, ticket.StateClaimed},
		{"claimed to open", ticket.StateClaimed, ticket.StateOpen},
		{"claimed to closed", ticket.StateClaimed, ticket.StateClosed},
		{"claimed to deferred", ticket.StateClaimed, ticket.StateDeferred},
		{"claimed to waiting", ticket.StateClaimed, ticket.StateWaiting},
		{"deferred to claimed", ticket.StateDeferred, ticket.StateClaimed},
		{"waiting to claimed", ticket.StateWaiting, ticket.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := ticket.TransitionTask(tc.current, tc.next)
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
		current ticket.State
		next    ticket.State
	}{
		{"open to closed", ticket.StateOpen, ticket.StateClosed},
		{"open to deferred", ticket.StateOpen, ticket.StateDeferred},
		{"deferred to open", ticket.StateDeferred, ticket.StateOpen},
		{"waiting to open", ticket.StateWaiting, ticket.StateOpen},
		{"claimed to claimed", ticket.StateClaimed, ticket.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := ticket.TransitionTask(tc.current, tc.next)

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
	err := ticket.TransitionTask(ticket.StateClosed, ticket.StateClaimed)

	// Then
	if !errors.Is(err, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState, got %v", err)
	}
}

func TestTransitionEpic_LegalTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current ticket.State
		next    ticket.State
	}{
		{"active to claimed", ticket.StateActive, ticket.StateClaimed},
		{"claimed to active", ticket.StateClaimed, ticket.StateActive},
		{"claimed to deferred", ticket.StateClaimed, ticket.StateDeferred},
		{"claimed to waiting", ticket.StateClaimed, ticket.StateWaiting},
		{"deferred to claimed", ticket.StateDeferred, ticket.StateClaimed},
		{"waiting to claimed", ticket.StateWaiting, ticket.StateClaimed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := ticket.TransitionEpic(tc.current, tc.next)
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
		current ticket.State
		next    ticket.State
	}{
		{"active to deferred", ticket.StateActive, ticket.StateDeferred},
		{"active to waiting", ticket.StateActive, ticket.StateWaiting},
		{"claimed to closed", ticket.StateClaimed, ticket.StateClosed},
		{"deferred to active", ticket.StateDeferred, ticket.StateActive},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := ticket.TransitionEpic(tc.current, tc.next)

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
	s := ticket.DefaultStateForRole(ticket.RoleTask)

	// Then
	if s != ticket.StateOpen {
		t.Errorf("expected open, got %s", s)
	}
}

func TestDefaultStateForRole_Epic_ReturnsActive(t *testing.T) {
	t.Parallel()

	// When
	s := ticket.DefaultStateForRole(ticket.RoleEpic)

	// Then
	if s != ticket.StateActive {
		t.Errorf("expected active, got %s", s)
	}
}

func TestParseState_ValidStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected ticket.State
	}{
		{"open", ticket.StateOpen},
		{"active", ticket.StateActive},
		{"claimed", ticket.StateClaimed},
		{"closed", ticket.StateClosed},
		{"deferred", ticket.StateDeferred},
		{"waiting", ticket.StateWaiting},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			s, err := ticket.ParseState(tc.input)
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
	_, err := ticket.ParseState("invalid")

	// Then
	if err == nil {
		t.Error("expected error for invalid state")
	}
}

func TestState_IsTerminal(t *testing.T) {
	t.Parallel()

	// Then
	if !ticket.StateClosed.IsTerminal() {
		t.Error("expected closed to be terminal")
	}
	if ticket.StateOpen.IsTerminal() {
		t.Error("expected open to not be terminal")
	}
	if ticket.StateClaimed.IsTerminal() {
		t.Error("expected claimed to not be terminal")
	}
}

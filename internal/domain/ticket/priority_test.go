package ticket_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestParsePriority_ValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected ticket.Priority
	}{
		{"P0", ticket.P0},
		{"P1", ticket.P1},
		{"P2", ticket.P2},
		{"P3", ticket.P3},
		{"P4", ticket.P4},
		// Case-insensitive.
		{"p0", ticket.P0},
		{"p3", ticket.P3},
		// Bare numeric (P prefix optional).
		{"0", ticket.P0},
		{"2", ticket.P2},
		{"4", ticket.P4},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			p, err := ticket.ParsePriority(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, p)
			}
		})
	}
}

func TestParsePriority_InvalidValues(t *testing.T) {
	t.Parallel()

	cases := []string{"P5", "high", "", "5", "-1", "p5"}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := ticket.ParsePriority(input)

			// Then
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestPriority_String_RoundTrips(t *testing.T) {
	t.Parallel()

	for p := ticket.P0; p <= ticket.P4; p++ {
		t.Run(p.String(), func(t *testing.T) {
			t.Parallel()

			// When
			parsed, err := ticket.ParsePriority(p.String())
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed != p {
				t.Errorf("round-trip failed: %v != %v", parsed, p)
			}
		})
	}
}

func TestPriority_IsHigherThan(t *testing.T) {
	t.Parallel()

	// Then
	if !ticket.P0.IsHigherThan(ticket.P1) {
		t.Error("expected P0 higher than P1")
	}
	if ticket.P2.IsHigherThan(ticket.P1) {
		t.Error("expected P2 not higher than P1")
	}
	if ticket.P2.IsHigherThan(ticket.P2) {
		t.Error("expected P2 not higher than itself")
	}
}

func TestPriority_ZeroValue_IsNotAValidPriority(t *testing.T) {
	t.Parallel()

	// The zero value of Priority must not coincide with any named priority
	// constant so that constructors can distinguish "not set" from "P0".
	var zero ticket.Priority
	for p := ticket.P0; p <= ticket.P4; p++ {
		if zero == p {
			t.Errorf("zero value of Priority (%d) collides with %s", int(zero), p)
		}
	}
}

func TestDefaultPriority_IsP2(t *testing.T) {
	t.Parallel()

	if ticket.DefaultPriority != ticket.P2 {
		t.Errorf("expected default P2, got %v", ticket.DefaultPriority)
	}
}

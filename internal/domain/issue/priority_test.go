package issue_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestParsePriority_ValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected issue.Priority
	}{
		{"P0", issue.P0},
		{"P1", issue.P1},
		{"P2", issue.P2},
		{"P3", issue.P3},
		{"P4", issue.P4},
		// Case-insensitive.
		{"p0", issue.P0},
		{"p3", issue.P3},
		// Bare numeric (P prefix optional).
		{"0", issue.P0},
		{"2", issue.P2},
		{"4", issue.P4},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			p, err := issue.ParsePriority(tc.input)
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
			_, err := issue.ParsePriority(input)

			// Then
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestPriority_String_RoundTrips(t *testing.T) {
	t.Parallel()

	for p := issue.P0; p <= issue.P4; p++ {
		t.Run(p.String(), func(t *testing.T) {
			t.Parallel()

			// When
			parsed, err := issue.ParsePriority(p.String())
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
	if !issue.P0.IsHigherThan(issue.P1) {
		t.Error("expected P0 higher than P1")
	}
	if issue.P2.IsHigherThan(issue.P1) {
		t.Error("expected P2 not higher than P1")
	}
	if issue.P2.IsHigherThan(issue.P2) {
		t.Error("expected P2 not higher than itself")
	}
}

func TestPriority_ZeroValue_IsNotAValidPriority(t *testing.T) {
	t.Parallel()

	// The zero value of Priority must not coincide with any named priority
	// constant so that constructors can distinguish "not set" from "P0".
	var zero issue.Priority
	for p := issue.P0; p <= issue.P4; p++ {
		if zero == p {
			t.Errorf("zero value of Priority (%d) collides with %s", int(zero), p)
		}
	}
}

func TestDefaultPriority_IsP2(t *testing.T) {
	t.Parallel()

	if issue.DefaultPriority != issue.P2 {
		t.Errorf("expected default P2, got %v", issue.DefaultPriority)
	}
}

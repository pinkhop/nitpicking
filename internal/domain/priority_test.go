package domain_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestParsePriority_ValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected domain.Priority
	}{
		{"P0", domain.P0},
		{"P1", domain.P1},
		{"P2", domain.P2},
		{"P3", domain.P3},
		{"P4", domain.P4},
		// Case-insensitive.
		{"p0", domain.P0},
		{"p3", domain.P3},
		// Bare numeric (P prefix optional).
		{"0", domain.P0},
		{"2", domain.P2},
		{"4", domain.P4},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			p, err := domain.ParsePriority(tc.input)
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
			_, err := domain.ParsePriority(input)

			// Then
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestPriority_String_RoundTrips(t *testing.T) {
	t.Parallel()

	for p := domain.P0; p <= domain.P4; p++ {
		t.Run(p.String(), func(t *testing.T) {
			t.Parallel()

			// When
			parsed, err := domain.ParsePriority(p.String())
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
	if !domain.P0.IsHigherThan(domain.P1) {
		t.Error("expected P0 higher than P1")
	}
	if domain.P2.IsHigherThan(domain.P1) {
		t.Error("expected P2 not higher than P1")
	}
	if domain.P2.IsHigherThan(domain.P2) {
		t.Error("expected P2 not higher than itself")
	}
}

func TestPriority_ZeroValue_IsNotAValidPriority(t *testing.T) {
	t.Parallel()

	// The zero value of Priority must not coincide with any named priority
	// constant so that constructors can distinguish "not set" from "P0".
	var zero domain.Priority
	for p := domain.P0; p <= domain.P4; p++ {
		if zero == p {
			t.Errorf("zero value of Priority (%d) collides with %s", int(zero), p)
		}
	}
}

func TestDefaultPriority_IsP2(t *testing.T) {
	t.Parallel()

	if domain.DefaultPriority != domain.P2 {
		t.Errorf("expected default P2, got %v", domain.DefaultPriority)
	}
}

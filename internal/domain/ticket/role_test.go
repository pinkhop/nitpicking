package ticket_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestParseRole_ValidRoles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected ticket.Role
	}{
		{"task", ticket.RoleTask},
		{"epic", ticket.RoleEpic},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			r, err := ticket.ParseRole(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, r)
			}
		})
	}
}

func TestParseRole_InvalidRoles(t *testing.T) {
	t.Parallel()

	cases := []string{"Task", "EPIC", "story", "bug", ""}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := ticket.ParseRole(input)

			// Then
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestRole_String_RoundTrips(t *testing.T) {
	t.Parallel()

	cases := []ticket.Role{ticket.RoleTask, ticket.RoleEpic}

	for _, r := range cases {
		t.Run(r.String(), func(t *testing.T) {
			t.Parallel()

			// When
			parsed, err := ticket.ParseRole(r.String())
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed != r {
				t.Errorf("round-trip failed: %v != %v", parsed, r)
			}
		})
	}
}

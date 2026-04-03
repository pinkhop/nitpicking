package driven_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// --- DisplayStatus ---

func TestIssueListItem_DisplayStatus_WithSecondaryState_ReturnsPrimaryParenSecondary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		state          domain.State
		secondaryState domain.SecondaryState
		want           string
	}{
		{"open ready", domain.StateOpen, domain.SecondaryReady, "open (ready)"},
		{"open blocked", domain.StateOpen, domain.SecondaryBlocked, "open (blocked)"},
		{"open active", domain.StateOpen, domain.SecondaryActive, "open (active)"},
		{"open completed", domain.StateOpen, domain.SecondaryCompleted, "open (completed)"},
		{"deferred blocked", domain.StateDeferred, domain.SecondaryBlocked, "deferred (blocked)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given
			item := driven.IssueListItem{
				State:          tc.state,
				SecondaryState: tc.secondaryState,
			}

			// When
			got := item.DisplayStatus()

			// Then
			if got != tc.want {
				t.Errorf("DisplayStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIssueListItem_DisplayStatus_NoSecondaryState_ReturnsPrimaryOnly(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state domain.State
		want  string
	}{
		{"claimed", domain.StateClaimed, "claimed"},
		{"closed", domain.StateClosed, "closed"},
		{"open none", domain.StateOpen, "open"},
		{"deferred none", domain.StateDeferred, "deferred"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given
			item := driven.IssueListItem{
				State:          tc.state,
				SecondaryState: domain.SecondaryNone,
			}

			// When
			got := item.DisplayStatus()

			// Then
			if got != tc.want {
				t.Errorf("DisplayStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

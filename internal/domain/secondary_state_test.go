package domain_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestSecondaryState_String_AllValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state domain.SecondaryState
		want  string
	}{
		{domain.SecondaryNone, ""},
		{domain.SecondaryClaimed, "claimed"},
		{domain.SecondaryReady, "ready"},
		{domain.SecondaryBlocked, "blocked"},
		{domain.SecondaryUnplanned, "unplanned"},
		{domain.SecondaryActive, "active"},
		{domain.SecondaryCompleted, "completed"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			// When
			got := tc.state.String()

			// Then
			if got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSecondaryState_ZeroValue_IsNone(t *testing.T) {
	t.Parallel()

	// Given
	var s domain.SecondaryState

	// When
	str := s.String()

	// Then
	if s != domain.SecondaryNone {
		t.Errorf("zero value = %v, want SecondaryNone", s)
	}
	if str != "" {
		t.Errorf("String() = %q, want empty string", str)
	}
}

func TestSecondaryState_ParseRoundTrips(t *testing.T) {
	t.Parallel()

	cases := []domain.SecondaryState{
		domain.SecondaryClaimed,
		domain.SecondaryReady,
		domain.SecondaryBlocked,
		domain.SecondaryUnplanned,
		domain.SecondaryActive,
		domain.SecondaryCompleted,
	}

	for _, ss := range cases {
		t.Run(ss.String(), func(t *testing.T) {
			t.Parallel()

			// When
			parsed, err := domain.ParseSecondaryState(ss.String())
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed != ss {
				t.Errorf("round-trip failed: got %v, want %v", parsed, ss)
			}
		})
	}
}

func TestParseSecondaryState_InvalidInput_ReturnsError(t *testing.T) {
	t.Parallel()

	cases := []string{
		"unknown",
		"READY",
		"42",
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := domain.ParseSecondaryState(input)

			// Then
			if err == nil {
				t.Errorf("expected error for input %q, got nil", input)
			}
		})
	}
}

func TestSecondaryStateResult_NoSecondaryState(t *testing.T) {
	t.Parallel()

	// Given — a result with no secondary state (e.g., claimed or closed issue).
	result := domain.SecondaryStateResult{}

	// Then
	if result.ListState != domain.SecondaryNone {
		t.Errorf("ListState = %v, want SecondaryNone", result.ListState)
	}
	if len(result.DetailStates) != 0 {
		t.Errorf("DetailStates length = %d, want 0", len(result.DetailStates))
	}
}

func TestSecondaryStateResult_HasSecondary_ReportsCorrectly(t *testing.T) {
	t.Parallel()

	// Given — a result with secondary states.
	result := domain.SecondaryStateResult{
		ListState:    domain.SecondaryBlocked,
		DetailStates: []domain.SecondaryState{domain.SecondaryBlocked, domain.SecondaryActive},
	}

	// Then
	if !result.HasSecondary() {
		t.Error("HasSecondary() = false, want true")
	}
}

func TestSecondaryStateResult_NoSecondary_ReportsCorrectly(t *testing.T) {
	t.Parallel()

	// Given — an empty result.
	result := domain.SecondaryStateResult{}

	// Then
	if result.HasSecondary() {
		t.Error("HasSecondary() = true, want false")
	}
}

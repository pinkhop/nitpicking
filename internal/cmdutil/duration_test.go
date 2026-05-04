package cmdutil_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// TestParseExtendedDuration_StandardGoUnits verifies that standard Go duration
// tokens (h, m, s, ms, us, ns) parse identically to time.ParseDuration.
func TestParseExtendedDuration_StandardGoUnits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"1h", time.Hour},
		{"12h30m", 12*time.Hour + 30*time.Minute},
		{"1s", time.Second},
		{"500ms", 500 * time.Millisecond},
		{"1us", time.Microsecond},
		{"1ns", time.Nanosecond},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := cmdutil.ParseExtendedDuration(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseExtendedDuration(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseExtendedDuration_FractionalStandardUnits verifies that fractional
// magnitudes on standard Go units (which time.ParseDuration accepts natively)
// are passed through correctly. The d/w extended units do not support
// fractional magnitudes; that is covered by TestParseExtendedDuration_InvalidInput.
func TestParseExtendedDuration_FractionalStandardUnits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  time.Duration
	}{
		{"1.5h", time.Hour + 30*time.Minute},
		{"0.5m", 30 * time.Second},
		{"2.5s", 2*time.Second + 500*time.Millisecond},
		// Fractional magnitude composes with non-fractional tokens that follow.
		{"1.5h30m", time.Hour + 30*time.Minute + 30*time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := cmdutil.ParseExtendedDuration(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseExtendedDuration(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseExtendedDuration_DayUnit verifies that "d" (day = 24h) is accepted
// as a unit in both pure and compound forms.
func TestParseExtendedDuration_DayUnit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  time.Duration
	}{
		{"1d", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"1d12h", 24*time.Hour + 12*time.Hour},
		{"3d30m", 3*24*time.Hour + 30*time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := cmdutil.ParseExtendedDuration(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseExtendedDuration(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseExtendedDuration_WeekUnit verifies that "w" (week = 7 × 24h) is
// accepted in both pure and compound forms.
func TestParseExtendedDuration_WeekUnit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  time.Duration
	}{
		{"1w", 7 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"1w3d", 7*24*time.Hour + 3*24*time.Hour},
		{"1w3d2h", 7*24*time.Hour + 3*24*time.Hour + 2*time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := cmdutil.ParseExtendedDuration(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseExtendedDuration(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseExtendedDuration_AcceptanceCriteriaExamples tests the four compound
// forms called out in the acceptance criteria: 1w, 1d12h, 30m, 1w3d2h.
func TestParseExtendedDuration_AcceptanceCriteriaExamples(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  time.Duration
	}{
		{"1w", 7 * 24 * time.Hour},
		{"1d12h", 24*time.Hour + 12*time.Hour},
		{"30m", 30 * time.Minute},
		{"1w3d2h", 7*24*time.Hour + 3*24*time.Hour + 2*time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := cmdutil.ParseExtendedDuration(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseExtendedDuration(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseExtendedDuration_Overflow verifies that arithmetic overflow on
// large multipliers (which would silently wrap to a negative duration) is
// detected and reported as an error.
func TestParseExtendedDuration_Overflow(t *testing.T) {
	t.Parallel()

	cases := []string{
		// 9999999999999w would silently wrap to a negative time.Duration.
		"9999999999999w",
		"9999999999999d",
		// Sum overflow: 100w (positive) + something that pushes beyond int64.
		"9999999999w9999999999w",
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			d, err := cmdutil.ParseExtendedDuration(input)
			if err == nil {
				t.Errorf("ParseExtendedDuration(%q): expected overflow error, got %v", input, d)
			}
			if d < 0 {
				t.Errorf("ParseExtendedDuration(%q): returned negative duration %v on overflow", input, d)
			}
		})
	}
}

// TestParseExtendedDuration_InvalidInput verifies that invalid inputs return
// a non-nil error.
func TestParseExtendedDuration_InvalidInput(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"abc",
		"1x",
		"d",
		"w",
		"1.5d",
		"-1d",
		"1 d",
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, err := cmdutil.ParseExtendedDuration(input)
			if err == nil {
				t.Errorf("ParseExtendedDuration(%q): expected error, got nil", input)
			}
		})
	}
}

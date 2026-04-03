package iostreams_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// ---------------------------------------------------------------------------
// ColorScheme — Color256
// ---------------------------------------------------------------------------

// TestColorScheme_Color256_ColorEnabled_Uses256ColorEscape verifies that
// Color256 wraps text in the 256-color escape sequence (\033[38;5;<n>m) when
// color is enabled.
func TestColorScheme_Color256_ColorEnabled_Uses256ColorEscape(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(true)

	// When
	got := cs.Color256(130, "claimed")

	// Then
	if !strings.Contains(got, "\033[38;5;130m") {
		t.Errorf("expected 256-color escape (38;5;130), got %q", got)
	}
	if !strings.Contains(got, "claimed") {
		t.Errorf("expected text to be preserved, got %q", got)
	}
	if !strings.HasSuffix(got, "\033[0m") {
		t.Errorf("expected reset suffix, got %q", got)
	}
}

// TestColorScheme_Color256_ColorDisabled_ReturnsPlain verifies that Color256
// returns the input string unmodified when color is disabled.
func TestColorScheme_Color256_ColorDisabled_ReturnsPlain(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(false)

	// When
	got := cs.Color256(130, "claimed")

	// Then
	if got != "claimed" {
		t.Errorf("expected plain %q, got %q", "claimed", got)
	}
}

// ---------------------------------------------------------------------------
// ColorScheme — Gray
// ---------------------------------------------------------------------------

// TestColorScheme_Gray_ColorEnabled_UsesMiddleGray verifies that Gray wraps
// text in the 256-color middle-gray escape sequence (\033[38;5;244m) when
// color is enabled, giving a shade close to #808080 that reads well on both
// light and dark terminal backgrounds.
func TestColorScheme_Gray_ColorEnabled_UsesMiddleGray(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(true)

	// When
	got := cs.Gray("label")

	// Then
	if !strings.Contains(got, "\033[38;5;244m") {
		t.Errorf("expected middle-gray escape (38;5;244), got %q", got)
	}
}

// TestColorScheme_Gray_ColorDisabled_ReturnsPlain verifies that Gray returns
// the input string unmodified when color is disabled.
func TestColorScheme_Gray_ColorDisabled_ReturnsPlain(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(false)

	// When
	got := cs.Gray("label")

	// Then
	if got != "label" {
		t.Errorf("expected plain %q, got %q", "label", got)
	}
}

// ---------------------------------------------------------------------------
// ColorScheme — Dim
// ---------------------------------------------------------------------------

// TestColorScheme_Dim_ColorEnabled_UsesDimAttribute verifies that Dim wraps
// text in the ANSI dim/faint attribute (\033[2m), which reduces brightness
// relative to the terminal's foreground color and is legible on both light
// and dark backgrounds.
func TestColorScheme_Dim_ColorEnabled_UsesDimAttribute(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(true)

	// When
	got := cs.Dim("(dirty)")

	// Then
	if !strings.Contains(got, "\033[2m") {
		t.Errorf("expected dim escape (\\033[2m), got %q", got)
	}
	if !strings.Contains(got, "(dirty)") {
		t.Errorf("expected text to be preserved, got %q", got)
	}
}

// TestColorScheme_Dim_ColorDisabled_ReturnsPlain verifies that Dim returns
// the input string unmodified when color is disabled.
func TestColorScheme_Dim_ColorDisabled_ReturnsPlain(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(false)

	// When
	got := cs.Dim("(dirty)")

	// Then
	if got != "(dirty)" {
		t.Errorf("expected plain %q, got %q", "(dirty)", got)
	}
}

// ---------------------------------------------------------------------------
// ColorScheme — icons
// ---------------------------------------------------------------------------

// TestColorScheme_SuccessIcon_ColorEnabled_ReturnsGreenCheckmark verifies that
// SuccessIcon returns a green-colored checkmark when color is enabled.
func TestColorScheme_SuccessIcon_ColorEnabled_ReturnsGreenCheckmark(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(true)

	// When
	got := cs.SuccessIcon()

	// Then
	if !strings.Contains(got, "\033[32m") {
		t.Errorf("expected green escape (32), got %q", got)
	}
	if !strings.Contains(got, "✓") {
		t.Errorf("expected checkmark in output, got %q", got)
	}
}

// TestColorScheme_SuccessIcon_ColorDisabled_ReturnsPlainText verifies that
// SuccessIcon returns the plain "[ok]" fallback when color is disabled.
func TestColorScheme_SuccessIcon_ColorDisabled_ReturnsPlainText(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(false)

	// When
	got := cs.SuccessIcon()

	// Then
	if got != "[ok]" {
		t.Errorf("expected %q, got %q", "[ok]", got)
	}
}

// TestColorScheme_WarningIcon_ColorEnabled_ReturnsYellowBang verifies that
// WarningIcon returns a yellow-colored exclamation mark when color is enabled.
func TestColorScheme_WarningIcon_ColorEnabled_ReturnsYellowBang(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(true)

	// When
	got := cs.WarningIcon()

	// Then
	if !strings.Contains(got, "\033[33m") {
		t.Errorf("expected yellow escape (33), got %q", got)
	}
	if !strings.Contains(got, "!") {
		t.Errorf("expected exclamation mark in output, got %q", got)
	}
}

// TestColorScheme_WarningIcon_ColorDisabled_ReturnsPlainText verifies that
// WarningIcon returns the plain "[warning]" fallback when color is disabled.
func TestColorScheme_WarningIcon_ColorDisabled_ReturnsPlainText(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(false)

	// When
	got := cs.WarningIcon()

	// Then
	if got != "[warning]" {
		t.Errorf("expected %q, got %q", "[warning]", got)
	}
}

// TestColorScheme_ErrorIcon_ColorEnabled_ReturnsRedCross verifies that
// ErrorIcon returns a red-colored cross mark when color is enabled.
func TestColorScheme_ErrorIcon_ColorEnabled_ReturnsRedCross(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(true)

	// When
	got := cs.ErrorIcon()

	// Then
	if !strings.Contains(got, "\033[31m") {
		t.Errorf("expected red escape (31), got %q", got)
	}
	if !strings.Contains(got, "✗") {
		t.Errorf("expected cross mark in output, got %q", got)
	}
}

// TestColorScheme_ErrorIcon_ColorDisabled_ReturnsPlainText verifies that
// ErrorIcon returns the plain "[error]" fallback when color is disabled.
func TestColorScheme_ErrorIcon_ColorDisabled_ReturnsPlainText(t *testing.T) {
	t.Parallel()

	// Given
	cs := iostreams.NewColorScheme(false)

	// When
	got := cs.ErrorIcon()

	// Then
	if got != "[error]" {
		t.Errorf("expected %q, got %q", "[error]", got)
	}
}

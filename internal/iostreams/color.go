package iostreams

import "fmt"

// ColorScheme provides ANSI color formatting that respects terminal capabilities.
// When color is disabled (non-TTY or user preference), all methods return
// the input string unmodified, ensuring clean output in piped and scripted contexts.
type ColorScheme struct {
	enabled bool
}

// NewColorScheme creates a ColorScheme. When enabled is false, all formatting
// methods become identity functions that return their input unmodified.
func NewColorScheme(enabled bool) *ColorScheme {
	return &ColorScheme{enabled: enabled}
}

// ansi wraps text with the given ANSI code if color is enabled.
// The reset code (\033[0m) is always appended to prevent color bleed.
func (cs *ColorScheme) ansi(code, text string) string {
	if !cs.enabled {
		return text
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", code, text)
}

// Bold applies bold formatting.
func (cs *ColorScheme) Bold(text string) string {
	return cs.ansi("1", text)
}

// Red applies red foreground color — typically used for errors and failures.
func (cs *ColorScheme) Red(text string) string {
	return cs.ansi("31", text)
}

// Green applies green foreground color — typically used for success states.
func (cs *ColorScheme) Green(text string) string {
	return cs.ansi("32", text)
}

// Yellow applies yellow foreground color — typically used for warnings.
func (cs *ColorScheme) Yellow(text string) string {
	return cs.ansi("33", text)
}

// Blue applies blue foreground color — typically used for informational messages.
func (cs *ColorScheme) Blue(text string) string {
	return cs.ansi("34", text)
}

// Magenta applies magenta foreground color.
func (cs *ColorScheme) Magenta(text string) string {
	return cs.ansi("35", text)
}

// Cyan applies cyan foreground color — typically used for highlights and links.
func (cs *ColorScheme) Cyan(text string) string {
	return cs.ansi("36", text)
}

// Color256 applies a 256-color foreground color using the given palette index
// (0–255). This enables fine-grained color choices beyond the basic 16 ANSI
// colors — e.g., muted theme-specific shades for state indicators.
func (cs *ColorScheme) Color256(code int, text string) string {
	return cs.ansi(fmt.Sprintf("38;5;%03d", code), text)
}

// Gray applies a medium-gray foreground color — typically used for secondary
// information. It uses the 256-color code 244, which maps to rgb(128,128,128)
// (#808080) and is readable on both light and dark terminal backgrounds.
func (cs *ColorScheme) Gray(text string) string {
	return cs.ansi("38;5;244", text)
}

// Dim applies the ANSI dim/faint attribute, which reduces the brightness of
// the text relative to the terminal's default foreground color. Unlike Gray,
// Dim is theme-adaptive: it produces muted dark text on light backgrounds and
// muted light text on dark backgrounds.
func (cs *ColorScheme) Dim(text string) string {
	return cs.ansi("2", text)
}

// SuccessIcon returns a colored check mark when color is enabled, or a plain
// text indicator otherwise — suitable for indicating successful operations.
func (cs *ColorScheme) SuccessIcon() string {
	if cs.enabled {
		return cs.Green("✓")
	}
	return "[ok]"
}

// WarningIcon returns a colored warning symbol when color is enabled, or a
// plain text indicator otherwise.
func (cs *ColorScheme) WarningIcon() string {
	if cs.enabled {
		return cs.Yellow("!")
	}
	return "[warning]"
}

// ErrorIcon returns a colored cross mark when color is enabled, or a plain
// text indicator otherwise — suitable for indicating failures.
func (cs *ColorScheme) ErrorIcon() string {
	if cs.enabled {
		return cs.Red("✗")
	}
	return "[error]"
}

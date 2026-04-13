package cmdutil

import (
	"strings"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// State color palette — 256-color ANSI codes for consistent state rendering
// across all commands:
//
//	closed   = 246 (grey)
//	claimed  = 172 (orange)
//	open     = 71  (green)
//	blocked  = 134 (purple)
//	deferred = 73  (teal)
//	empty    = 238 (dark gray, for zero-count display)
const (
	colorClosed   = 246
	colorClaimed  = 172
	colorOpen     = 71
	colorBlocked  = 134
	colorDeferred = 73
	colorEmpty    = 238
)

// ColorState applies the canonical ANSI color to a primary state string.
// Use this when rendering a bare state label outside of FormatState or
// FormatDetailState — e.g., blocker state annotations in the show command.
func ColorState(cs *iostreams.ColorScheme, state domain.State) string {
	return colorPrimary(cs, state)
}

// ColorStateText applies the canonical color for the given primary state to
// arbitrary text. Use this when the text is not the state name itself — e.g.,
// coloring a numeric count with the state's color.
func ColorStateText(cs *iostreams.ColorScheme, state domain.State, text string) string {
	switch state {
	case domain.StateClosed:
		return cs.Color256(colorClosed, text)
	case domain.StateOpen:
		return cs.Color256(colorOpen, text)
	case domain.StateDeferred:
		return cs.Color256(colorDeferred, text)
	default:
		return text
	}
}

// ColorBlockedText applies the canonical "blocked" color (134, purple) to
// arbitrary text.
func ColorBlockedText(cs *iostreams.ColorScheme, text string) string {
	return cs.Color256(colorBlocked, text)
}

// ColorEmpty applies the "empty" color (dark gray, 238) to text. Use this
// for zero-count indicators in dashboard-style output.
func ColorEmpty(cs *iostreams.ColorScheme, text string) string {
	return cs.Color256(colorEmpty, text)
}

// FormatState formats a primary state with a single secondary state for list
// views. Returns "primary (secondary)" when a secondary state is present, or
// just "primary" when secondary is SecondaryNone. Both the primary and
// secondary state texts are colored according to the unified state palette.
func FormatState(cs *iostreams.ColorScheme, primary domain.State, secondary domain.SecondaryState) string {
	p := colorPrimary(cs, primary)
	if secondary == domain.SecondaryNone {
		// Append an invisible zero-width Color256 application so that
		// primary-only states have the same number of ANSI escape
		// sequences (and therefore the same byte overhead) as
		// primary+secondary states. Without this, text/tabwriter
		// computes different column widths for cells that differ only
		// in invisible ANSI byte count, causing visible misalignment.
		return p + cs.Color256(0, "")
	}
	return p + " (" + colorSecondary(cs, secondary) + ")"
}

// FormatDetailState formats a primary state with multiple secondary conditions
// for detail views. Returns "primary (sec1, sec2)" when secondary conditions
// are present, or just "primary" when the slice is empty or nil. Both the
// primary state and each secondary condition are individually colored.
func FormatDetailState(cs *iostreams.ColorScheme, primary domain.State, details []domain.SecondaryState) string {
	p := colorPrimary(cs, primary)
	if len(details) == 0 {
		return p
	}
	parts := make([]string, 0, len(details))
	for _, s := range details {
		parts = append(parts, colorSecondary(cs, s))
	}
	return p + " (" + strings.Join(parts, ", ") + ")"
}

// colorPrimary applies the canonical 256-color to a primary state string.
func colorPrimary(cs *iostreams.ColorScheme, state domain.State) string {
	str := state.String()
	switch state {
	case domain.StateClosed:
		return cs.Color256(colorClosed, str)
	case domain.StateOpen:
		return cs.Color256(colorOpen, str)
	case domain.StateDeferred:
		return cs.Color256(colorDeferred, str)
	default:
		return str
	}
}

// ColorSecondaryText applies the canonical color for a secondary state to
// arbitrary text. Use this when the text is not the secondary state name
// itself — e.g., wrapping a label like "[blocked]" in the blocked color.
func ColorSecondaryText(cs *iostreams.ColorScheme, s domain.SecondaryState, text string) string {
	switch s {
	case domain.SecondaryClaimed:
		return cs.Color256(colorClaimed, text)
	case domain.SecondaryReady, domain.SecondaryUnplanned:
		return cs.Color256(colorOpen, text)
	case domain.SecondaryBlocked:
		return cs.Color256(colorBlocked, text)
	case domain.SecondaryActive:
		return cs.Color256(colorClaimed, text)
	case domain.SecondaryCompleted:
		return cs.Color256(colorClosed, text)
	default:
		return text
	}
}

// colorSecondary applies the appropriate 256-color to a secondary state value.
// Secondary states inherit colors from their closest primary-state analogue:
// claimed → claimed (172), ready/unplanned → open (71), blocked → blocked (134),
// active → claimed (172), completed → closed (246).
func colorSecondary(cs *iostreams.ColorScheme, s domain.SecondaryState) string {
	str := s.String()
	switch s {
	case domain.SecondaryClaimed:
		return cs.Color256(colorClaimed, str)
	case domain.SecondaryReady, domain.SecondaryUnplanned:
		return cs.Color256(colorOpen, str)
	case domain.SecondaryBlocked:
		return cs.Color256(colorBlocked, str)
	case domain.SecondaryActive:
		return cs.Color256(colorClaimed, str)
	case domain.SecondaryCompleted:
		return cs.Color256(colorClosed, str)
	default:
		return str
	}
}

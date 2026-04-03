package iostreams_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// --- TerminalWidth ---

func TestTerminalWidth_TestStreams_ReturnsZero(t *testing.T) {
	t.Parallel()

	// Given — test IOStreams (not a TTY)
	streams, _, _, _ := iostreams.Test()

	// When
	width := streams.TerminalWidth()

	// Then — non-TTY output should return 0, signaling no width constraint
	if width != 0 {
		t.Errorf("TerminalWidth: got %d, want 0 for non-TTY", width)
	}
}

func TestTerminalWidth_OverrideSet_ReturnsOverride(t *testing.T) {
	t.Parallel()

	// Given — test IOStreams with a width override
	streams, _, _, _ := iostreams.Test()
	streams.SetTerminalWidth(120)

	// When
	width := streams.TerminalWidth()

	// Then
	if width != 120 {
		t.Errorf("TerminalWidth: got %d, want 120", width)
	}
}

func TestTerminalWidth_OverrideZero_ReturnsZero(t *testing.T) {
	t.Parallel()

	// Given — test IOStreams with override explicitly set to 0
	streams, _, _, _ := iostreams.Test()
	streams.SetTerminalWidth(0)

	// When
	width := streams.TerminalWidth()

	// Then — 0 means no width constraint
	if width != 0 {
		t.Errorf("TerminalWidth: got %d, want 0", width)
	}
}

package cmdutil_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// --- TruncateTitle ---

func TestTruncateTitle_FitsWithinMax_ReturnsUnchanged(t *testing.T) {
	t.Parallel()

	// Given
	title := "Short title"

	// When
	got := cmdutil.TruncateTitle(title, 80)

	// Then
	if got != title {
		t.Errorf("TruncateTitle: got %q, want %q", got, title)
	}
}

func TestTruncateTitle_ExactlyMax_ReturnsUnchanged(t *testing.T) {
	t.Parallel()

	// Given
	title := "12345"

	// When
	got := cmdutil.TruncateTitle(title, 5)

	// Then
	if got != title {
		t.Errorf("TruncateTitle: got %q, want %q", got, title)
	}
}

func TestTruncateTitle_ExceedsMax_TruncatesWithEllipsis(t *testing.T) {
	t.Parallel()

	// Given
	title := "A very long title that exceeds the maximum"

	// When
	got := cmdutil.TruncateTitle(title, 20)

	// Then — should be 19 visible characters + "…" = 20 columns
	want := "A very long title t…"
	if got != want {
		t.Errorf("TruncateTitle: got %q, want %q", got, want)
	}
}

func TestTruncateTitle_ZeroMax_ReturnsUnchanged(t *testing.T) {
	t.Parallel()

	// Given — 0 means no constraint (non-TTY)
	title := "This should not be truncated"

	// When
	got := cmdutil.TruncateTitle(title, 0)

	// Then
	if got != title {
		t.Errorf("TruncateTitle: got %q, want %q", got, title)
	}
}

func TestTruncateTitle_NegativeMax_ReturnsUnchanged(t *testing.T) {
	t.Parallel()

	// Given
	title := "This should not be truncated"

	// When
	got := cmdutil.TruncateTitle(title, -1)

	// Then
	if got != title {
		t.Errorf("TruncateTitle: got %q, want %q", got, title)
	}
}

func TestTruncateTitle_MaxTooSmallForEllipsis_ReturnsEllipsis(t *testing.T) {
	t.Parallel()

	// Given — max is 1, only room for the ellipsis itself
	title := "Long title"

	// When
	got := cmdutil.TruncateTitle(title, 1)

	// Then
	want := "…"
	if got != want {
		t.Errorf("TruncateTitle: got %q, want %q", got, want)
	}
}

// --- AvailableTitleWidth ---

func TestAvailableTitleWidth_SubtractsOverhead(t *testing.T) {
	t.Parallel()

	// Given — terminal width 80, non-title columns use 30 characters
	termWidth := 80
	overhead := 30

	// When
	got := cmdutil.AvailableTitleWidth(termWidth, overhead)

	// Then
	if got != 50 {
		t.Errorf("AvailableTitleWidth: got %d, want 50", got)
	}
}

func TestAvailableTitleWidth_ZeroTermWidth_ReturnsZero(t *testing.T) {
	t.Parallel()

	// Given — non-TTY
	termWidth := 0
	overhead := 30

	// When
	got := cmdutil.AvailableTitleWidth(termWidth, overhead)

	// Then — 0 signals no truncation
	if got != 0 {
		t.Errorf("AvailableTitleWidth: got %d, want 0", got)
	}
}

func TestAvailableTitleWidth_OverheadExceedsWidth_ReturnsMinimum(t *testing.T) {
	t.Parallel()

	// Given — terminal is narrower than non-title columns
	termWidth := 20
	overhead := 30

	// When
	got := cmdutil.AvailableTitleWidth(termWidth, overhead)

	// Then — should return a small positive minimum, not zero or negative
	if got < 1 {
		t.Errorf("AvailableTitleWidth: got %d, want >= 1", got)
	}
}

// --- WriteListHeader ---

func TestWriteListHeader_WithoutTimestamp_WritesExpectedColumns(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer

	// When
	cmdutil.WriteListHeader(&buf, false)

	// Then
	got := buf.String()
	want := "ID\tROLE\tSTATE\tPRIORITY\tTITLE\n"
	if got != want {
		t.Errorf("WriteListHeader(false): got %q, want %q", got, want)
	}
}

func TestWriteListHeader_WithTimestamp_IncludesCreatedColumn(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer

	// When
	cmdutil.WriteListHeader(&buf, true)

	// Then
	got := buf.String()
	want := "ID\tROLE\tSTATE\tPRIORITY\tCREATED\tTITLE\n"
	if got != want {
		t.Errorf("WriteListHeader(true): got %q, want %q", got, want)
	}
}

func TestWriteListHeader_AllColumnsAreUpperCase(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer

	// When
	cmdutil.WriteListHeader(&buf, true)

	// Then — every column name must be all-caps.
	header := strings.TrimRight(buf.String(), "\n")
	for _, col := range strings.Split(header, "\t") {
		if col != strings.ToUpper(col) {
			t.Errorf("column %q is not all-caps", col)
		}
	}
}

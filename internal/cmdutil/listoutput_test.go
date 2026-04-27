package cmdutil_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
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

// --- ConvertListItems: ParentCreatedAt ---

func TestConvertListItems_WithParentCreatedAt_PopulatesField(t *testing.T) {
	t.Parallel()

	// Given — an item whose parent was created at a known time.
	parentTime := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	items := []driving.IssueListItemDTO{
		{
			ID:              "FOO-abc12",
			Role:            domain.RoleTask,
			State:           domain.StateOpen,
			Priority:        domain.P2,
			Title:           "Child task",
			ParentID:        "FOO-par01",
			ParentCreatedAt: parentTime,
			CreatedAt:       time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC),
			DisplayStatus:   "open (ready)",
		},
	}

	// When
	out := cmdutil.ConvertListItems(items)

	// Then
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	want := cmdutil.FormatJSONTimestamp(parentTime)
	if out[0].ParentCreatedAt != want {
		t.Errorf("ParentCreatedAt = %q, want %q", out[0].ParentCreatedAt, want)
	}
	if out[0].ParentID != "FOO-par01" {
		t.Errorf("ParentID = %q, want %q", out[0].ParentID, "FOO-par01")
	}
}

func TestConvertListItems_WithoutParent_ParentCreatedAtEmpty(t *testing.T) {
	t.Parallel()

	// Given — an orphan issue with zero ParentCreatedAt.
	items := []driving.IssueListItemDTO{
		{
			ID:            "FOO-abc12",
			Role:          domain.RoleTask,
			State:         domain.StateOpen,
			Priority:      domain.P2,
			Title:         "Orphan task",
			CreatedAt:     time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC),
			DisplayStatus: "open (ready)",
		},
	}

	// When
	out := cmdutil.ConvertListItems(items)

	// Then — ParentCreatedAt should be empty for issues without a parent.
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	if out[0].ParentCreatedAt != "" {
		t.Errorf("ParentCreatedAt = %q, want empty string", out[0].ParentCreatedAt)
	}
}

package cmdutil

import (
	"fmt"
	"io"
	"strings"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// ListItemOutput is the JSON representation of a single issue in a list.
// It is shared by the list, ready, and blocked commands so that their
// --json output is structurally identical.
type ListItemOutput struct {
	ID             string   `json:"id"`
	Role           string   `json:"role"`
	State          string   `json:"state"`
	SecondaryState string   `json:"secondary_state,omitempty"`
	DisplayStatus  string   `json:"display_status"`
	Priority       string   `json:"priority"`
	Title          string   `json:"title"`
	BlockerIDs     []string `json:"blocker_ids,omitempty"`
	CreatedAt      string   `json:"created_at"`
}

// ListOutput is the JSON representation of a list command result.
type ListOutput struct {
	Items   []ListItemOutput `json:"items"`
	HasMore bool             `json:"has_more"`
}

// ConvertListItems transforms a slice of service-layer IssueListItemDTO values
// into the shared JSON output representation.
func ConvertListItems(items []driving.IssueListItemDTO) []ListItemOutput {
	out := make([]ListItemOutput, 0, len(items))
	for _, item := range items {
		o := ListItemOutput{
			ID:             item.ID,
			Role:           item.Role.String(),
			State:          item.State.String(),
			SecondaryState: item.SecondaryState.String(),
			DisplayStatus:  item.DisplayStatus,
			Priority:       item.Priority.String(),
			Title:          item.Title,
			BlockerIDs:     item.BlockerIDs,
			CreatedAt:      FormatJSONTimestamp(item.CreatedAt),
		}
		out = append(out, o)
	}
	return out
}

// minTitleWidth is the smallest title column width before truncation stops
// being useful. Below this threshold, even a single word is too short to be
// meaningful with an ellipsis.
const minTitleWidth = 10

// TruncateTitle truncates a title string to fit within maxWidth columns.
// If the title fits, it is returned unchanged. If it exceeds maxWidth, the
// last visible character is replaced with "…" (U+2026). When maxWidth is
// zero or negative, the title is returned unchanged — this signals a
// non-TTY environment where no truncation should occur.
func TruncateTitle(title string, maxWidth int) string {
	if maxWidth <= 0 {
		return title
	}
	runes := []rune(title)
	if len(runes) <= maxWidth {
		return title
	}
	if maxWidth <= 1 {
		return "…"
	}
	return string(runes[:maxWidth-1]) + "…"
}

// AvailableTitleWidth calculates how many columns the title may occupy given
// the terminal width and the overhead consumed by non-title columns and tab
// padding. Returns 0 when termWidth is 0 (non-TTY), signaling that callers
// should not truncate. Returns at least minTitleWidth when the terminal is
// very narrow, so the title remains somewhat readable.
func AvailableTitleWidth(termWidth, overhead int) int {
	if termWidth <= 0 {
		return 0
	}
	available := termWidth - overhead
	if available < minTitleWidth {
		return minTitleWidth
	}
	return available
}

// maxBlockerDisplay is the maximum number of blocker IDs shown in text output
// before truncating with "…".
const maxBlockerDisplay = 3

// FormatBlockerSuffix formats blocker IDs for display after the issue title in
// text output. Shows at most maxBlockerDisplay IDs; appends "…" when truncated.
// Accepts pre-formatted string IDs from the service-layer DTO.
func FormatBlockerSuffix(blockerIDs []string) string {
	n := len(blockerIDs)
	if n == 0 {
		return ""
	}

	limit := n
	if limit > maxBlockerDisplay {
		limit = maxBlockerDisplay
	}

	suffix := "[← " + strings.Join(blockerIDs[:limit], ", ")
	if n > maxBlockerDisplay {
		suffix += ", …"
	}
	suffix += "]"
	return suffix
}

// WriteListHeader writes an all-caps column header row to a tabwriter. The
// includeTimestamp flag controls whether a CREATED column appears between
// PRIORITY and TITLE, matching the optional timestamp column in list and
// search output. The header uses the same tab-separated format as the data
// rows so that tabwriter aligns them together.
func WriteListHeader(w io.Writer, includeTimestamp bool) {
	if includeTimestamp {
		_, _ = fmt.Fprintf(w, "ID\tROLE\tSTATE\tPRIORITY\tCREATED\tTITLE\n")
	} else {
		_, _ = fmt.Fprintf(w, "ID\tROLE\tSTATE\tPRIORITY\tTITLE\n")
	}
}

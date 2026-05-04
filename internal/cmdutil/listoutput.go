package cmdutil

import (
	"strings"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// ListItemOutput is the JSON representation of a single issue in a list.
// It is shared by the list, ready, and blocked commands so that their
// --json output is structurally identical.
type ListItemOutput struct {
	ID              string   `json:"id"`
	Role            string   `json:"role"`
	State           string   `json:"state"`
	SecondaryState  string   `json:"secondary_state,omitempty"`
	DisplayStatus   string   `json:"display_status"`
	Priority        string   `json:"priority"`
	Title           string   `json:"title"`
	BlockerIDs      []string `json:"blocker_ids,omitempty"`
	ParentID        string   `json:"parent_id,omitempty"`
	ParentCreatedAt string   `json:"parent_created_at,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

// ListOutput is the JSON representation of a list command result.
type ListOutput struct {
	Issues  []ListItemOutput `json:"issues"`
	HasMore bool             `json:"has_more"`
}

// ConvertListItems transforms a slice of service-layer IssueListItemDTO values
// into the shared JSON output representation.
func ConvertListItems(items []driving.IssueListItemDTO) []ListItemOutput {
	out := make([]ListItemOutput, 0, len(items))
	for _, item := range items {
		o := ListItemOutput{
			ID:              item.ID,
			Role:            item.Role.String(),
			State:           item.State.String(),
			SecondaryState:  item.SecondaryState.String(),
			DisplayStatus:   item.DisplayStatus,
			Priority:        item.Priority.String(),
			Title:           item.Title,
			BlockerIDs:      item.BlockerIDs,
			ParentID:        item.ParentID,
			ParentCreatedAt: FormatJSONTimestamp(item.ParentCreatedAt),
			CreatedAt:       FormatJSONTimestamp(item.CreatedAt),
		}
		out = append(out, o)
	}
	return out
}

// minTitleWidth is the smallest title column width before truncation stops
// being useful. Below this threshold, even a single word is too short to be
// meaningful with an ellipsis.
const minTitleWidth = 10

// treeBaseOverhead is the non-title character overhead for the rel list tree
// table at zero indentation depth. The TREE column holds a 10-char ID (12 with
// 2-char tab padding), P is 2 chars (4 with padding), ROLE is 5 chars (7 with
// padding), and STATE is 14 chars worst-case (16 with padding), summing to 39.
//
// When the table contains rows at depth > 0, the TREE column expands by 2
// chars per depth level. Because TableWriter pads every column to its maximum
// width across all rows, ALL rows in a mixed-depth table pay the same expanded
// TREE column cost. Callers must therefore account for the global maximum cell
// width — not the per-row depth — when computing the uniform overhead.
const treeBaseOverhead = 39

// issueIDWidth is the assumed display width of an issue ID within the TREE
// column, embedded in treeBaseOverhead. Subtracting it isolates the overhead
// that comes from non-TREE columns (P, ROLE, STATE plus tab padding = 29).
const issueIDWidth = 10

// UniformTreeOverhead computes the total non-title column overhead for rel list
// tree tables given the maximum TREE cell width across all rendered rows.
//
// The formula is maxCellWidth + (treeBaseOverhead - issueIDWidth): the first
// term is the actual TREE column width (which TableWriter pads to uniformly),
// and the second term (29) is the fixed overhead from the P, ROLE, and STATE
// columns with their tab padding.
//
// Both the parent-child and blocking sections use this formula. For a
// pure-issue tree without back-references, prefer UniformIssueTreeOverhead.
// For blocking trees with back-reference rows, maxCellWidth must be computed
// from the actual node list because back-ref rows are wider than issue rows at
// the same depth.
func UniformTreeOverhead(maxCellWidth int) int {
	return maxCellWidth + (treeBaseOverhead - issueIDWidth)
}

// UniformIssueTreeOverhead is a convenience wrapper around UniformTreeOverhead
// for pure-issue trees (no back-references). It derives the maximum TREE cell
// width from maxDepth: at the deepest level each issue row is issueIDWidth wide
// plus 2 chars of indent per level.
func UniformIssueTreeOverhead(maxDepth int) int {
	return UniformTreeOverhead(issueIDWidth + maxDepth*2)
}

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

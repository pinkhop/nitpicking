package show

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// showOutput is the JSON representation of the show command result.
type showOutput struct {
	ID                 string                   `json:"id"`
	Role               string                   `json:"role"`
	Title              string                   `json:"title"`
	Description        string                   `json:"description,omitzero"`
	AcceptanceCriteria string                   `json:"acceptance_criteria,omitzero"`
	Priority           string                   `json:"priority"`
	State              string                   `json:"state"`
	SecondaryState     string                   `json:"secondary_state,omitzero"`
	SecondaryStates    []string                 `json:"secondary_states,omitzero"`
	ParentID           string                   `json:"parent_id,omitzero"`
	Revision           int                      `json:"revision"`
	Author             string                   `json:"author,omitzero"`
	IsReady            bool                     `json:"is_ready"`
	InheritedBlocking  *inheritedBlockingOutput `json:"inherited_blocking,omitzero"`
	CommentCount       int                      `json:"comment_count,omitzero"`
	ChildCount         int                      `json:"child_count"`
	Children           []childOutput            `json:"children,omitzero"`
	ClaimAuthor        string                   `json:"claim_author,omitzero"`
	ClaimedAt          string                   `json:"claimed_at,omitzero"`
	ClaimStaleAt       string                   `json:"claim_stale_at,omitzero"`
	Relationships      []relationshipOutput     `json:"relationships,omitzero"`
	Labels             map[string]string        `json:"labels,omitzero"`
	Comments           []commentOutput          `json:"comments,omitzero"`
	CreatedAt          string                   `json:"created_at"`
}

// childOutput is the JSON representation of a child issue in the show output.
type childOutput struct {
	ID             string `json:"id"`
	Role           string `json:"role"`
	State          string `json:"state"`
	SecondaryState string `json:"secondary_state,omitzero"`
	DisplayStatus  string `json:"display_status,omitzero"`
	Priority       string `json:"priority"`
	Title          string `json:"title"`
}

// commentOutput is the JSON representation of a comment in the show output.
type commentOutput struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// inheritedBlockingOutput is the JSON representation of blocking inherited
// from an ancestor epic.
type inheritedBlockingOutput struct {
	AncestorID string   `json:"ancestor_id"`
	BlockerIDs []string `json:"blocker_ids"`
}

// relationshipOutput is the JSON representation of a single relationship.
type relationshipOutput struct {
	Type     string `json:"type"`
	TargetID string `json:"target_id"`
}

// RunInput holds the parameters for the show command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	IssueID       string
	TerminalWidth int // Terminal width for word wrapping; 0 means no wrapping.
	JSON          bool
	WriteTo       io.Writer
	ColorScheme   *iostreams.ColorScheme
}

// Run executes the show workflow: fetches the issue and writes the full detail
// view to the output writer.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Service.ShowIssue(ctx, input.IssueID)
	if err != nil {
		return fmt.Errorf("showing issue: %w", err)
	}

	if input.JSON {
		out := showOutput{
			ID:                 result.ID,
			Role:               result.Role.String(),
			Title:              result.Title,
			Description:        result.Description,
			AcceptanceCriteria: result.AcceptanceCriteria,
			Priority:           result.Priority.String(),
			State:              result.State.String(),
			Revision:           result.Revision,
			Author:             result.Author,
			IsReady:            result.IsReady,
			CommentCount:       result.CommentCount,
			ChildCount:         result.ChildCount,
			ClaimAuthor:        result.ClaimAuthor,
			CreatedAt:          cmdutil.FormatJSONTimestamp(result.CreatedAt),
		}

		if result.SecondaryState != domain.SecondaryNone {
			out.SecondaryState = result.SecondaryState.String()
			out.SecondaryStates = secondaryStatesToStrings(result.DetailStates)
		}

		if result.ParentID != "" {
			out.ParentID = result.ParentID
		}

		if !result.ClaimedAt.IsZero() {
			out.ClaimedAt = cmdutil.FormatJSONTimestamp(result.ClaimedAt)
		}

		if !result.ClaimStaleAt.IsZero() {
			out.ClaimStaleAt = cmdutil.FormatJSONTimestamp(result.ClaimStaleAt)
		}

		for _, rel := range result.Relationships {
			out.Relationships = append(out.Relationships, relationshipOutput{
				Type:     rel.Type,
				TargetID: rel.TargetID,
			})
		}

		if len(result.Labels) > 0 {
			out.Labels = result.Labels
		}

		if result.InheritedBlocking != nil {
			ib := &inheritedBlockingOutput{
				AncestorID: result.InheritedBlocking.AncestorID,
				BlockerIDs: result.InheritedBlocking.BlockerIDs,
			}
			out.InheritedBlocking = ib
		}

		for _, child := range result.Children {
			out.Children = append(out.Children, childOutput{
				ID:             child.ID,
				Role:           child.Role.String(),
				State:          child.State.String(),
				SecondaryState: child.SecondaryState.String(),
				DisplayStatus:  child.DisplayStatus,
				Priority:       child.Priority.String(),
				Title:          child.Title,
			})
		}

		// JSON path: include only the 3 most recent comments.
		recentComments := result.Comments
		if len(recentComments) > maxRecentComments {
			recentComments = recentComments[len(recentComments)-maxRecentComments:]
		}
		for _, c := range recentComments {
			out.Comments = append(out.Comments, commentOutput{
				ID:        c.DisplayID,
				Author:    c.Author,
				Body:      c.Body,
				CreatedAt: cmdutil.FormatJSONTimestamp(c.CreatedAt),
			})
		}

		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	// Human-readable output.
	w := input.WriteTo
	cs := input.ColorScheme
	if cs == nil {
		cs = iostreams.NewColorScheme(false)
	}

	// --- Header ---
	_, _ = fmt.Fprintf(w, "%s  %s  %s\n",
		cs.Bold(result.ID),
		cs.Dim(result.Role.String()),
		result.Title)
	_, _ = fmt.Fprintln(w, "────────────────────────────────────────────────────")

	// --- Labels (alphabetized, no header, double-spaced) ---
	if len(result.Labels) > 0 {
		labelPairs := make([]string, 0, len(result.Labels))
		for k, v := range result.Labels {
			labelPairs = append(labelPairs, cs.Dim(k+":"+v))
		}
		_, _ = fmt.Fprintln(w, strings.Join(labelPairs, "  "))
		_, _ = fmt.Fprintln(w)
	}

	// --- State (with secondary state from domain computation) ---
	stateDisplay := cmdutil.FormatDetailState(cs, result.State, result.DetailStates)

	_, _ = fmt.Fprintf(w, "%s  %s\n", cs.Dim("Priority:"), result.Priority)
	_, _ = fmt.Fprintf(w, "%s     %s\n", cs.Dim("State:"), stateDisplay)
	_, _ = fmt.Fprintln(w)

	// --- Claim info ---
	if result.ClaimID != "" {
		_, _ = fmt.Fprintf(w, "%s  %s\n", cs.Dim("Claimed by:"), result.ClaimAuthor)
		if !result.ClaimStaleAt.IsZero() {
			staleStr := result.ClaimStaleAt.UTC().Format("2006-01-02 15:04 UTC")
			dur := time.Until(result.ClaimStaleAt)
			if dur > 0 {
				staleStr += fmt.Sprintf(" (in %s)", formatDuration(dur))
			} else {
				staleStr += " (stale)"
			}
			_, _ = fmt.Fprintf(w, "%s    %s\n", cs.Dim("Stale at:"), staleStr)
		}
	} else {
		_, _ = fmt.Fprintf(w, "%s  %s\n", cs.Dim("Claimed by:"), "(none)")
	}
	_, _ = fmt.Fprintln(w)

	// --- Timestamps, author, and revision ---
	_, _ = fmt.Fprintf(w, "%s    %s\n", cs.Dim("Created:"), result.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"))
	_, _ = fmt.Fprintf(w, "%s     %s\n", cs.Dim("Author:"), result.Author)
	_, _ = fmt.Fprintf(w, "%s   %s\n", cs.Dim("Revision:"), fmt.Sprintf("%d", result.Revision))

	// --- Description ---
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, cs.Dim("Description:"))
	if result.Description != "" {
		_, _ = fmt.Fprintln(w, cmdutil.WordWrap(result.Description, input.TerminalWidth))
	} else {
		_, _ = fmt.Fprintln(w, "(none)")
	}

	// --- Acceptance Criteria ---
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, cs.Dim("Acceptance Criteria:"))
	if result.AcceptanceCriteria != "" {
		_, _ = fmt.Fprintln(w, cmdutil.WordWrap(result.AcceptanceCriteria, input.TerminalWidth))
	} else {
		_, _ = fmt.Fprintln(w, "(none)")
	}

	// --- Parent ---
	_, _ = fmt.Fprintln(w)
	if result.ParentID != "" {
		parentDisplay := result.ParentID
		if result.ParentTitle != "" {
			parentDisplay += "  " + result.ParentTitle
		}
		_, _ = fmt.Fprintf(w, "%s  %s\n", cs.Dim("Parent:"), parentDisplay)
	} else {
		_, _ = fmt.Fprintf(w, "%s  %s\n", cs.Dim("Parent:"), "(none)")
	}

	// --- Children ---
	writeChildrenSection(w, cs, result.Children, result.ChildCount)

	// --- Inherited blocking ---
	if result.InheritedBlocking != nil {
		_, _ = fmt.Fprintf(w, "\n%s %s (blocked by %s)\n",
			cs.Dim("Blocked via:"),
			result.InheritedBlocking.AncestorID,
			strings.Join(result.InheritedBlocking.BlockerIDs, ", "))
	}

	// --- Relationships by type ---
	writeBlockedBySection(w, cs, result.BlockerDetails)
	writeRelationshipSection(w, cs, "Blocks", domain.RelBlocks.String(), result.Relationships)
	writeRelationshipSection(w, cs, "References", domain.RelRefs.String(), result.Relationships)

	// --- Comments section ---
	if result.CommentCount > 0 {
		renderComments(w, cs, result.Comments, result.CommentCount, input.TerminalWidth)
	}

	return nil
}

// maxRecentComments is the maximum number of comments to display in the
// show text output. Earlier comments are indicated by a "N earlier" marker.
const maxRecentComments = 3

// renderComments writes the comments section to the output writer. It shows
// up to maxRecentComments most recent comments in ascending (chronological)
// order, with an "N earlier" indicator when older comments exist.
func renderComments(w io.Writer, cs *iostreams.ColorScheme, comments []driving.CommentDTO, totalCount, terminalWidth int) {
	// Build header.
	header := fmt.Sprintf("Comments (%d)", totalCount)

	// Determine which comments to show (most recent N).
	startIdx := 0
	if len(comments) > maxRecentComments {
		startIdx = len(comments) - maxRecentComments
	}
	shown := comments[startIdx:]

	// Add "N earlier" indicator if we're truncating.
	if startIdx > 0 {
		header += fmt.Sprintf("  %d earlier", startIdx)
	}

	_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold(header))

	for _, c := range shown {
		_, _ = fmt.Fprintln(w, "──")
		_, _ = fmt.Fprintf(w, "%s · %s\n",
			cs.Bold(c.Author),
			cs.Dim(c.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")))
		_, _ = fmt.Fprintln(w, cmdutil.WordWrap(c.Body, terminalWidth))
	}
}

// maxChildrenDisplay is the maximum number of children shown in the text
// output before truncating with an overflow indicator.
const maxChildrenDisplay = 10

// writeChildrenSection renders the children section of the show output.
// Shows up to maxChildrenDisplay children with their ID, state, and title.
// When there are more children than the limit, an overflow indicator is shown.
func writeChildrenSection(w io.Writer, cs *iostreams.ColorScheme, children []driving.IssueListItemDTO, count int) {
	if count == 0 {
		_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold("Children (0):"))
		return
	}

	_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold(fmt.Sprintf("Children (%d):", count)))

	limit := count
	if limit > maxChildrenDisplay {
		limit = maxChildrenDisplay
	}
	for i := range limit {
		if i >= len(children) {
			break
		}
		child := children[i]
		_, _ = fmt.Fprintf(w, "  %s  %s  %s\n",
			child.ID,
			cmdutil.FormatState(cs, child.State, child.SecondaryState),
			child.Title)
	}
	if count > maxChildrenDisplay {
		_, _ = fmt.Fprintf(w, "  %s\n", cs.Dim(fmt.Sprintf("… and %d more", count-maxChildrenDisplay)))
	}
}

// writeBlockedBySection renders the "Blocked by" section with enriched
// blocker details. Blockers are ordered: claimed first, then open, then
// blocked (open with unresolved blockers), then deferred. Claimed issues
// show the claim author; deferred issues are marked with a warning symbol.
func writeBlockedBySection(w io.Writer, cs *iostreams.ColorScheme, blockers []driving.BlockerDetail) {
	if len(blockers) == 0 {
		return
	}

	// Sort blockers by state priority: claimed, open, deferred, closed.
	slices.SortFunc(blockers, func(a, b driving.BlockerDetail) int {
		return cmp.Compare(blockerSortOrder(a), blockerSortOrder(b))
	})

	_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold(fmt.Sprintf("Blocked by (%d):", len(blockers))))
	for _, b := range blockers {
		line := fmt.Sprintf("  %s  %s  %s", b.ID, cmdutil.ColorState(cs, b.State), b.Title)
		// For open issues with an active claim, append the claim author to indicate work is in progress.
		if b.State == domain.StateOpen && b.ClaimAuthor != "" {
			line += fmt.Sprintf(" (%s)", b.ClaimAuthor)
		}
		if b.State == domain.StateDeferred {
			line += " ⚠"
		}
		_, _ = fmt.Fprintln(w, line)
	}
}

// blockerSortOrder returns a numeric sort key for a blocker's state.
// Lower values sort first: open (including claimed) (0), deferred (1), closed/other (2).
func blockerSortOrder(b driving.BlockerDetail) int {
	switch b.State {
	case domain.StateOpen:
		return 0
	case domain.StateDeferred:
		return 1
	default:
		return 2
	}
}

// writeRelationshipSection renders a relationship section (e.g., "Blocks",
// "References") by filtering the relationship list for the given type string.
func writeRelationshipSection(w io.Writer, _ *iostreams.ColorScheme, header string, relType string, rels []driving.RelationshipDTO) {
	var matching []driving.RelationshipDTO
	for _, rel := range rels {
		if rel.Type == relType {
			matching = append(matching, rel)
		}
	}
	if len(matching) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "\n%s (%d):\n", header, len(matching))
	for _, rel := range matching {
		_, _ = fmt.Fprintf(w, "  %s\n", rel.TargetID)
	}
}

// formatDuration formats a time.Duration into a human-readable short form
// like "2h", "45m", "3d", or "1h30m". Used for relative timestamps in text
// output (e.g., "in 2h" for stale-at, "2h ago" for claimed-at).
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	if hours < 24 {
		mins := int(d.Minutes()) % 60
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	days := hours / 24
	remainingHours := hours % 24
	if remainingHours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, remainingHours)
}

// NewCmd constructs the "show" command, which displays the full detail view
// of a single domain.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "show",
		Usage:     "Show issue details, relationships, and labels",
		ArgsUsage: "<ISSUE-ID>",
		Description: `Displays the complete detail view of a single issue: title, description,
acceptance criteria, priority, state, labels, claim status, parent/child
hierarchy, relationships (blocked_by, blocks, refs), and recent comments.

Use this to understand what an issue requires before starting work, to
check whether an issue is blocked and by whom, or to review comments left
by other agents. For epics, the children list shows each child's state so
you can assess progress at a glance.

In JSON mode (--json), the output includes all fields and is suitable for
programmatic consumption by agents and scripts. Text mode formats the
output for human readability with color and word-wrapping.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("issue ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			return Run(ctx, RunInput{
				Service:       svc,
				IssueID:       issueID.String(),
				TerminalWidth: f.IOStreams.TerminalWidth(),
				JSON:          jsonOutput,
				WriteTo:       f.IOStreams.Out,
				ColorScheme:   f.IOStreams.ColorScheme(),
			})
		},
	}
}

// secondaryStatesToStrings converts a slice of SecondaryState values to their
// string representations for JSON output. Returns nil when the input is empty
// to preserve omitzero behavior.
func secondaryStatesToStrings(states []domain.SecondaryState) []string {
	if len(states) == 0 {
		return nil
	}
	out := make([]string, len(states))
	for i, s := range states {
		out[i] = s.String()
	}
	return out
}

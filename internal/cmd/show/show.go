package show

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
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
	ParentID           string                   `json:"parent_id,omitzero"`
	Revision           int                      `json:"revision"`
	Author             string                   `json:"author,omitzero"`
	IsReady            bool                     `json:"is_ready"`
	InheritedBlocking  *inheritedBlockingOutput `json:"inherited_blocking,omitzero"`
	CommentCount       int                      `json:"comment_count,omitzero"`
	ChildCount         int                      `json:"child_count,omitzero"`
	ClaimID            string                   `json:"claim_id,omitzero"`
	ClaimAuthor        string                   `json:"claim_author,omitzero"`
	ClaimStaleAt       string                   `json:"claim_stale_at,omitzero"`
	Relationships      []relationshipOutput     `json:"relationships,omitzero"`
	CreatedAt          string                   `json:"created_at"`
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

// NewCmd constructs the "show" command, which displays the full detail view
// of a single issue.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "show",
		Usage:     "Show issue details, relationships, and metadata",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
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
			result, err := svc.ShowIssue(ctx, issueID)
			if err != nil {
				return fmt.Errorf("showing issue: %w", err)
			}

			t := result.Issue

			if jsonOutput {
				out := showOutput{
					ID:                 t.ID().String(),
					Role:               t.Role().String(),
					Title:              t.Title(),
					Description:        t.Description(),
					AcceptanceCriteria: t.AcceptanceCriteria(),
					Priority:           t.Priority().String(),
					State:              t.State().String(),
					Revision:           result.Revision,
					Author:             result.Author.String(),
					IsReady:            result.IsReady,
					CommentCount:       result.CommentCount,
					ChildCount:         result.ChildCount,
					ClaimID:            result.ClaimID,
					ClaimAuthor:        result.ClaimAuthor,
					CreatedAt:          cmdutil.FormatJSONTimestamp(t.CreatedAt()),
				}

				if !t.ParentID().IsZero() {
					out.ParentID = t.ParentID().String()
				}

				if !result.ClaimStaleAt.IsZero() {
					out.ClaimStaleAt = cmdutil.FormatJSONTimestamp(result.ClaimStaleAt)
				}

				for _, rel := range result.Relationships {
					out.Relationships = append(out.Relationships, relationshipOutput{
						Type:     rel.Type().String(),
						TargetID: rel.TargetID().String(),
					})
				}

				if result.InheritedBlocking != nil {
					ib := &inheritedBlockingOutput{
						AncestorID: result.InheritedBlocking.AncestorID.String(),
						BlockerIDs: make([]string, 0, len(result.InheritedBlocking.BlockerIDs)),
					}
					for _, bid := range result.InheritedBlocking.BlockerIDs {
						ib.BlockerIDs = append(ib.BlockerIDs, bid.String())
					}
					out.InheritedBlocking = ib
				}

				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			// Human-readable output.
			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			_, _ = fmt.Fprintf(w, "%s  %s\n", cs.Bold(t.ID().String()), t.Title())
			_, _ = fmt.Fprintf(w, "Role: %s  |  State: %s  |  Priority: %s\n",
				t.Role().String(), t.State().String(), t.Priority().String())

			if !t.ParentID().IsZero() {
				_, _ = fmt.Fprintf(w, "Parent: %s\n", t.ParentID().String())
			}

			_, _ = fmt.Fprintf(w, "Revision: %d  |  Author: %s\n", result.Revision, result.Author.String())

			if result.IsReady {
				_, _ = fmt.Fprintf(w, "Ready: %s\n", cs.Green("yes"))
			}

			if result.InheritedBlocking != nil {
				blockerStrs := make([]string, 0, len(result.InheritedBlocking.BlockerIDs))
				for _, bid := range result.InheritedBlocking.BlockerIDs {
					blockerStrs = append(blockerStrs, bid.String())
				}
				_, _ = fmt.Fprintf(w, "Blocked via ancestor: %s (blocked by %s)\n",
					cs.Bold(result.InheritedBlocking.AncestorID.String()),
					cs.Red(strings.Join(blockerStrs, ", ")))
			}

			if result.ChildCount > 0 {
				_, _ = fmt.Fprintf(w, "Children: %d — %s\n",
					result.ChildCount,
					cs.Dim(fmt.Sprintf("np epic children %s", t.ID())))
			}

			if result.CommentCount > 0 {
				_, _ = fmt.Fprintf(w, "Comments: %d — %s\n",
					result.CommentCount,
					cs.Dim(fmt.Sprintf("np comment list --issue %s", t.ID())))
			}

			if result.ClaimID != "" {
				_, _ = fmt.Fprintf(w, "Claim: %s by %s", cs.Cyan(result.ClaimID), result.ClaimAuthor)
				if !result.ClaimStaleAt.IsZero() {
					_, _ = fmt.Fprintf(w, " (stale at %s)", result.ClaimStaleAt.Format(time.RFC3339))
				}
				_, _ = fmt.Fprintln(w)
			}

			if t.Description() != "" {
				_, _ = fmt.Fprintf(w, "\n%s\n%s\n", cs.Bold("Description:"), t.Description())
			}
			if t.AcceptanceCriteria() != "" {
				_, _ = fmt.Fprintf(w, "\n%s\n%s\n", cs.Bold("Acceptance Criteria:"), t.AcceptanceCriteria())
			}

			if len(result.Relationships) > 0 {
				_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold("Relationships:"))
				for _, rel := range result.Relationships {
					_, _ = fmt.Fprintf(w, "  %s → %s\n", rel.Type().String(), rel.TargetID().String())
				}
			}

			// Display labels.
			if t.Labels().Len() > 0 {
				_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold("Labels:"))
				for k, v := range t.Labels().All() {
					_, _ = fmt.Fprintf(w, "  %s: %s\n", k, v)
				}
			}

			return nil
		},
	}
}

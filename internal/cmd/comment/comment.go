package comment

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- JSON output types ---

// commentOutput is the JSON representation of a single comment.
type commentOutput struct {
	CommentID string `json:"comment_id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// commentListOutput is the JSON representation of a comment listing.
type commentListOutput struct {
	Comments []commentOutput `json:"comments"`
	HasMore  bool            `json:"has_more"`
}

// NewCmd constructs the "comment" command with list and search subcommands for
// managing issue comments. Comment creation is handled by "json comment".
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "comment",
		Usage: "Manage issue comments",
		Description: `Provides commands for listing and searching comments on issues. Comments
are the primary mechanism for recording context that does not belong in code
or commit messages — reasoning behind decisions, trade-offs considered, dead
ends explored, and summaries of completed work.

Comment creation is handled through "json comment" (or "form comment" for
interactive use). The subcommands here are read-only: "comment list" shows
comments on a specific issue, and "comment search" performs full-text search
across all comment bodies with optional scoping by issue, parent, tree,
author, or label.`,
		Commands: []*cli.Command{
			newListCmd(f),
			newSearchCmd(f),
		},
	}
}

// RunListInput holds the parameters for the comment list operation, decoupled
// from CLI flag parsing so it can be tested directly.
type RunListInput struct {
	Service driving.Service
	IssueID string
	Limit   int
	JSON    bool
	WriteTo io.Writer
}

// RunList executes the comment list workflow: lists comments for the specified
// issue and writes the result to WriteTo.
func RunList(ctx context.Context, input RunListInput) error {
	result, err := input.Service.ListComments(ctx, driving.ListCommentsInput{
		IssueID: input.IssueID,
		Limit:   input.Limit,
	})
	if err != nil {
		return fmt.Errorf("listing comments: %w", err)
	}

	if input.JSON {
		out := commentListOutput{
			HasMore:  result.HasMore,
			Comments: make([]commentOutput, 0, len(result.Comments)),
		}
		for _, n := range result.Comments {
			out.Comments = append(out.Comments, commentOutput{
				CommentID: n.DisplayID,
				IssueID:   n.IssueID,
				Author:    n.Author,
				Body:      n.Body,
				CreatedAt: cmdutil.FormatJSONTimestamp(n.CreatedAt),
			})
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	w := input.WriteTo

	if len(result.Comments) == 0 {
		_, _ = fmt.Fprintln(w, "No comments found.")
		return nil
	}

	for _, n := range result.Comments {
		_, _ = fmt.Fprintf(w, "%s  %s  %s  %s\n",
			n.DisplayID,
			n.Author,
			n.CreatedAt.Format(time.RFC3339),
			truncate(n.Body, 80))
	}

	shown := len(result.Comments)
	if result.HasMore {
		_, _ = fmt.Fprintf(w, "\n%d comments (more available)\n", shown)
	} else {
		_, _ = fmt.Fprintf(w, "\n%d comments\n", shown)
	}

	return nil
}

// newListCmd constructs the "comment list" subcommand, which lists comments for a
// specific issue.
func newListCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		limit      int
	)

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List comments for an issue",
		Description: `Shows comments attached to the specified issue, ordered by creation time.
Each comment displays its ID, author, timestamp, and a truncated body
preview. Use --limit to control how many comments are returned; the default
is capped to keep output manageable, and the response indicates whether more
comments are available.

This command does not require a claim. Use it to review discussion history
before claiming an issue, to verify that a comment was posted successfully,
or to catch up on context left by previous agents. For searching across
comments on multiple issues, use "comment search" instead.`,
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results (0 = default, negative = unlimited)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &limit,
			},
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

			return RunList(ctx, RunListInput{
				Service: svc,
				IssueID: issueID.String(),
				Limit:   limit,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

// truncate shortens a string to maxLen runes, appending "..." if truncated.
// Newlines are replaced with spaces for single-line display.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

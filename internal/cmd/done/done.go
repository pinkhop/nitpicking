// Package done provides the "done" workflow shortcut — a combined close-with-
// reason command that adds a comment and then closes the issue in one step.
// Aliased as "close" at the top level.
package done

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// doneOutput is the JSON representation of a done command result.
type doneOutput struct {
	IssueID string `json:"issue_id"`
	Action  string `json:"action"`
}

// RunInput holds the parameters for the done command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service service.Service
	IssueID issue.ID
	ClaimID string
	Author  identity.Author
	Reason  string
	JSON    bool
	WriteTo io.Writer
}

// Run executes the done workflow: validates the reason, adds a comment with
// the reason text, and then closes the issue. The comment is added first
// because closing invalidates the claim — and comment creation updates the
// claim's last-activity timestamp while it is still active.
func Run(ctx context.Context, input RunInput) error {
	if input.Reason == "" {
		return fmt.Errorf("reason is required: explain why the issue is being closed")
	}

	// Step 1: Add the reason as a comment while the claim is still active.
	_, err := input.Service.AddComment(ctx, service.AddCommentInput{
		IssueID: input.IssueID,
		Author:  input.Author,
		Body:    input.Reason,
	})
	if err != nil {
		return fmt.Errorf("adding closing reason: %w", err)
	}

	// Step 2: Close the issue (invalidates the claim).
	err = input.Service.TransitionState(ctx, service.TransitionInput{
		IssueID: input.IssueID,
		ClaimID: input.ClaimID,
		Action:  service.ActionClose,
	})
	if err != nil {
		return fmt.Errorf("closing issue: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, doneOutput{
			IssueID: input.IssueID.String(),
			Action:  "done",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Closed %s\n", input.IssueID)
	return err
}

// NewCmd constructs the "done" command, which adds a closing reason as a
// comment and then closes the issue. This is a workflow shortcut that combines
// "comment add" and "state close" into a single step. Aliased as "close".
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
		author     string
		reason     string
	)

	return &cli.Command{
		Name:      "done",
		Aliases:   []string{"close"},
		Usage:     "Close a claimed issue with a required reason",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Destination: &claimID,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name for the closing comment (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "reason",
				Aliases:     []string{"r"},
				Usage:       "Reason for closing (added as a comment) (required)",
				Required:    true,
				Destination: &reason,
			},
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

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
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
				Service: svc,
				IssueID: issueID,
				ClaimID: claimID,
				Author:  parsedAuthor,
				Reason:  reason,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

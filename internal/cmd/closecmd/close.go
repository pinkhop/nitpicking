// Package closecmd provides the "close" workflow shortcut — a combined close-with-
// reason command that adds a comment and then closes the issue in one step.
// The "cmd" suffix avoids collision with Go's built-in close function.
package closecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// closeOutput is the JSON representation of a close command result.
type closeOutput struct {
	IssueID string `json:"issue_id"`
	Action  string `json:"action"`
}

// RunInput holds the parameters for the close command's core logic, decoupled
// from CLI flag parsing so it can be tested directly. The command delegates to
// CloseWithReason, which derives the author from the claim record and performs
// the comment + close atomically.
type RunInput struct {
	Service driving.Service
	IssueID string
	ClaimID string
	Reason  string
	JSON    bool
	WriteTo io.Writer
}

// Run executes the close workflow: delegates to the service's CloseWithReason
// method, which atomically adds a comment with the reason text and closes the
// issue within a single transaction. The author for the closing comment is
// derived from the claim record by the service.
func Run(ctx context.Context, input RunInput) error {
	err := input.Service.CloseWithReason(ctx, driving.CloseWithReasonInput{
		IssueID: input.IssueID,
		ClaimID: input.ClaimID,
		Reason:  input.Reason,
	})
	if err != nil {
		return fmt.Errorf("closing issue with reason: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, closeOutput{
			IssueID: input.IssueID,
			Action:  "close",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Closed %s\n", input.IssueID)
	return err
}

// NewCmd constructs the "close" command, which adds a closing reason as a
// comment and then closes the issue. This is a workflow shortcut that combines
// "comment add" and "state close" into a single step.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
		reason     string
	)

	return &cli.Command{
		Name:  "close",
		Usage: "Close an issue that you have claimed",
		Description: `Closes an issue that you have claimed, atomically adding a closing reason
as a comment in the same transaction. This is a workflow shortcut that
combines "comment add" and a state transition into a single step — you do
not need to add a comment separately before closing.

Use this when your work on the issue is complete and it should no longer
appear in the ready queue. The --claim flag must reference an active claim
you hold; the author for the closing comment is derived from that claim.
The --reason flag is required so that every closure carries an explanation
of what was done and why.

Closed issues are hidden from "list" output by default but can be
re-included with --all. They can also be reopened later with
"issue reopen" if the closure turns out to be premature.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &claimID,
			},
			&cli.StringFlag{
				Name:        "reason",
				Aliases:     []string{"r"},
				Usage:       "Reason for closing (added as a comment) (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &reason,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			issueID, err := svc.LookupClaimIssueID(ctx, claimID)
			if err != nil {
				return fmt.Errorf("looking up claim: %w", err)
			}

			return Run(ctx, RunInput{
				Service: svc,
				IssueID: issueID,
				ClaimID: claimID,
				Reason:  reason,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

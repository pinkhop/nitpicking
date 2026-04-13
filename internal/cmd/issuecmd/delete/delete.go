package delete

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// deleteOutput is the JSON representation of the delete command result.
type deleteOutput struct {
	IssueID string `json:"issue_id"`
	Deleted bool   `json:"deleted"`
}

// RunInput holds the parameters for the delete command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service     driving.Service
	IssueID     string
	ClaimID     string
	Confirm     bool
	JSON        bool
	WriteTo     io.Writer
	ColorScheme *iostreams.ColorScheme
}

// Run executes the delete workflow: validates the confirm flag, deletes the
// issue via the service, and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	if !input.Confirm {
		return cmdutil.FlagErrorf("--confirm is required to delete an issue")
	}

	deleteIn := driving.DeleteInput{
		IssueID: input.IssueID,
		ClaimID: input.ClaimID,
	}
	if err := input.Service.DeleteIssue(ctx, deleteIn); err != nil {
		return fmt.Errorf("deleting issue: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, deleteOutput{
			IssueID: input.IssueID,
			Deleted: true,
		})
	}

	cs := input.ColorScheme
	_, err := fmt.Fprintf(input.WriteTo, "%s Deleted %s\n",
		cs.SuccessIcon(), cs.Bold(input.IssueID))
	return err
}

// NewCmd constructs the "delete" command, which deletes a claimed issue.
// The --confirm flag is required to prevent accidental deletions.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
		confirm    bool
	)

	return &cli.Command{
		Name:  "delete",
		Usage: "Delete a claimed issue",
		Description: `Permanently deletes a claimed issue from the database. This is a
destructive operation — the issue and its history cannot be recovered. The
--confirm flag is required to prevent accidental deletions.

Use this only when an issue was created in error or is otherwise completely
invalid. In most cases, closing an issue is preferable to deleting it, since
closed issues preserve their history and can be reopened. The issue must be
claimed before it can be deleted, which prevents deletion of issues that are
being actively worked on by another agent.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &claimID,
			},
			&cli.BoolFlag{
				Name:        "confirm",
				Usage:       "Confirm the deletion (required)",
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &confirm,
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
				Service:     svc,
				IssueID:     issueID,
				ClaimID:     claimID,
				Confirm:     confirm,
				JSON:        jsonOutput,
				WriteTo:     f.IOStreams.Out,
				ColorScheme: f.IOStreams.ColorScheme(),
			})
		},
	}
}

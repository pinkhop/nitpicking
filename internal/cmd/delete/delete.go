package delete

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// deleteOutput is the JSON representation of the delete command result.
type deleteOutput struct {
	IssueID string `json:"issue_id"`
	Deleted bool   `json:"deleted"`
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
		Name:      "delete",
		Usage:     "Delete a claimed issue",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Destination: &claimID,
			},
			&cli.BoolFlag{
				Name:        "confirm",
				Usage:       "Confirm the deletion (required)",
				Category:    "Options",
				Destination: &confirm,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if !confirm {
				return cmdutil.FlagErrorf("--confirm is required to delete an issue")
			}

			rawID := cmd.Args().Get(0)

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			idResolver := cmdutil.NewIDResolver(svc)
			claimResolver := cmdutil.NewClaimIssueResolver(svc, idResolver)

			issueID, err := claimResolver.Resolve(ctx, rawID, claimID)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			input := service.DeleteInput{
				IssueID: issueID,
				ClaimID: claimID,
			}
			if err := svc.DeleteIssue(ctx, input); err != nil {
				return fmt.Errorf("deleting issue: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, deleteOutput{
					IssueID: issueID.String(),
					Deleted: true,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Deleted %s\n",
				cs.SuccessIcon(), cs.Bold(issueID.String()))
			return err
		},
	}
}

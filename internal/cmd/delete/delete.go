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
	TicketID string `json:"ticket_id"`
	Deleted  bool   `json:"deleted"`
}

// NewCmd constructs the "delete" command, which deletes a claimed ticket.
// The --confirm flag is required to prevent accidental deletions.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
		confirm    bool
	)

	return &cli.Command{
		Name:      "delete",
		Usage:     "Delete a claimed ticket",
		ArgsUsage: "<TICKET-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the ticket",
				Required:    true,
				Destination: &claimID,
			},
			&cli.BoolFlag{
				Name:        "confirm",
				Usage:       "Confirm the deletion (required)",
				Destination: &confirm,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if !confirm {
				return cmdutil.FlagErrorf("--confirm is required to delete a ticket")
			}

			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("ticket ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			ticketID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid ticket ID: %s", err)
			}

			input := service.DeleteInput{
				TicketID: ticketID,
				ClaimID:  claimID,
			}
			if err := svc.DeleteTicket(ctx, input); err != nil {
				return fmt.Errorf("deleting ticket: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, deleteOutput{
					TicketID: ticketID.String(),
					Deleted:  true,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Deleted %s\n",
				cs.SuccessIcon(), cs.Bold(ticketID.String()))
			return err
		},
	}
}

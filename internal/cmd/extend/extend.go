package extend

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// extendOutput is the JSON representation of the extend command result.
type extendOutput struct {
	TicketID  string `json:"ticket_id"`
	ClaimID   string `json:"claim_id"`
	Threshold string `json:"threshold"`
}

// NewCmd constructs the "extend" command, which extends the stale threshold
// on an active claim to prevent it from being considered stale.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
		threshold  string
	)

	return &cli.Command{
		Name:      "extend",
		Usage:     "Extend the stale threshold on an active claim",
		ArgsUsage: "<TICKET-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "claim-id",
				Usage:       "Active claim ID",
				Required:    true,
				Destination: &claimID,
			},
			&cli.StringFlag{
				Name:        "threshold",
				Usage:       "New stale threshold duration (e.g., 1h, 45m)",
				Required:    true,
				Destination: &threshold,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("ticket ID argument is required")
			}

			ticketID, err := ticket.ParseID(rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid ticket ID: %s", err)
			}

			duration, err := time.ParseDuration(threshold)
			if err != nil {
				return cmdutil.FlagErrorf("invalid threshold duration: %s", err)
			}

			svc := f.Tracker()
			if err := svc.ExtendStaleThreshold(ctx, ticketID, claimID, duration); err != nil {
				return fmt.Errorf("extending stale threshold: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, extendOutput{
					TicketID:  ticketID.String(),
					ClaimID:   claimID,
					Threshold: duration.String(),
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Extended stale threshold on %s to %s\n",
				cs.SuccessIcon(),
				cs.Bold(ticketID.String()),
				duration.String())
			return err
		},
	}
}

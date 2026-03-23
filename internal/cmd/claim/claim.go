package claim

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// claimOutput is the JSON representation of the claim command result.
type claimOutput struct {
	TicketID string `json:"ticket_id"`
	ClaimID  string `json:"claim_id"`
	Stolen   bool   `json:"stolen"`
}

// NewCmd constructs the "claim" command, which claims a specific ticket by ID.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput     bool
		author         string
		steal          bool
		staleThreshold string
	)

	return &cli.Command{
		Name:      "claim",
		Usage:     "Claim a ticket by ID",
		ArgsUsage: "<TICKET-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Usage:       "Author name for the claim",
				Required:    true,
				Destination: &author,
			},
			&cli.BoolFlag{
				Name:        "steal",
				Usage:       "Steal the claim from another agent if already claimed",
				Destination: &steal,
			},
			&cli.StringFlag{
				Name:        "stale-threshold",
				Usage:       "Duration after which the claim becomes stale (e.g., 30m, 1h)",
				Destination: &staleThreshold,
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

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			var threshold time.Duration
			if staleThreshold != "" {
				threshold, err = time.ParseDuration(staleThreshold)
				if err != nil {
					return cmdutil.FlagErrorf("invalid stale threshold: %s", err)
				}
			}

			input := service.ClaimInput{
				TicketID:       ticketID,
				Author:         parsedAuthor,
				AllowSteal:     steal,
				StaleThreshold: threshold,
			}

			svc := f.Tracker()
			result, err := svc.ClaimByID(ctx, input)
			if err != nil {
				return fmt.Errorf("claiming ticket: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, claimOutput{
					TicketID: result.TicketID.String(),
					ClaimID:  result.ClaimID,
					Stolen:   result.Stolen,
				})
			}

			cs := f.IOStreams.ColorScheme()
			out := f.IOStreams.Out

			verb := "Claimed"
			if result.Stolen {
				verb = "Stole claim on"
			}

			_, err = fmt.Fprintf(out, "%s %s %s\n  Claim ID: %s\n",
				cs.SuccessIcon(),
				verb,
				cs.Bold(result.TicketID.String()),
				cs.Cyan(result.ClaimID))
			return err
		},
	}
}

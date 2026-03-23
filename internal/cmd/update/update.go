package update

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// updateOutput is the JSON representation of the update command result.
type updateOutput struct {
	TicketID string `json:"ticket_id"`
	Updated  bool   `json:"updated"`
}

// NewCmd constructs the "update" command, which updates fields on a claimed
// ticket. The caller must hold an active claim and provide its claim ID.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput         bool
		claimID            string
		title              string
		description        string
		acceptanceCriteria string
		priority           string
		parent             string
		noteBody           string
	)

	return &cli.Command{
		Name:      "update",
		Usage:     "Update a claimed ticket's fields",
		ArgsUsage: "<TICKET-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "claim-id",
				Usage:       "Active claim ID for the ticket",
				Required:    true,
				Destination: &claimID,
			},
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "New title",
				Destination: &title,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "New description",
				Destination: &description,
			},
			&cli.StringFlag{
				Name:        "acceptance-criteria",
				Usage:       "New acceptance criteria",
				Destination: &acceptanceCriteria,
			},
			&cli.StringFlag{
				Name:        "priority",
				Aliases:     []string{"p"},
				Usage:       "New priority: P0–P4",
				Destination: &priority,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "New parent epic ID (empty string to remove parent)",
				Destination: &parent,
			},
			&cli.StringSliceFlag{
				Name:  "facet-set",
				Usage: "Set a facet in key:value format (repeatable)",
			},
			&cli.StringSliceFlag{
				Name:  "facet-remove",
				Usage: "Remove a facet by key (repeatable)",
			},
			&cli.StringFlag{
				Name:        "note",
				Usage:       "Add a note to the ticket",
				Destination: &noteBody,
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

			input := service.UpdateTicketInput{
				TicketID:    ticketID,
				ClaimID:     claimID,
				FacetRemove: cmd.StringSlice("facet-remove"),
				NoteBody:    noteBody,
			}

			// Set optional pointer fields only when flags are explicitly provided.
			if cmd.IsSet("title") {
				input.Title = &title
			}
			if cmd.IsSet("description") {
				input.Description = &description
			}
			if cmd.IsSet("acceptance-criteria") {
				input.AcceptanceCriteria = &acceptanceCriteria
			}
			if cmd.IsSet("priority") {
				p, err := ticket.ParsePriority(priority)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
				input.Priority = &p
			}
			if cmd.IsSet("parent") {
				if parent == "" {
					zeroID := ticket.ID{}
					input.ParentID = &zeroID
				} else {
					pid, err := ticket.ParseID(parent)
					if err != nil {
						return cmdutil.FlagErrorf("invalid parent ID: %s", err)
					}
					input.ParentID = &pid
				}
			}

			// Parse facet-set values.
			rawFacetSet := cmd.StringSlice("facet-set")
			for _, s := range rawFacetSet {
				key, value, ok := strings.Cut(s, ":")
				if !ok {
					return cmdutil.FlagErrorf("invalid facet %q: must be in key:value format", s)
				}
				facet, err := ticket.NewFacet(key, value)
				if err != nil {
					return cmdutil.FlagErrorf("invalid facet %q: %s", s, err)
				}
				input.FacetSet = append(input.FacetSet, facet)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			if err := svc.UpdateTicket(ctx, input); err != nil {
				return fmt.Errorf("updating ticket: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, updateOutput{
					TicketID: ticketID.String(),
					Updated:  true,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Updated %s\n",
				cs.SuccessIcon(), cs.Bold(ticketID.String()))
			return err
		},
	}
}

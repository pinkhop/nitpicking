package edit

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// editOutput is the JSON representation of the edit command result.
type editOutput struct {
	TicketID string `json:"ticket_id"`
	Updated  bool   `json:"updated"`
}

// NewCmd constructs the "edit" command, which performs an atomic
// claim-update-release on a ticket without requiring a separate claim step.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput         bool
		author             string
		title              string
		description        string
		acceptanceCriteria string
		priority           string
		parent             string
	)

	return &cli.Command{
		Name:      "edit",
		Usage:     "Atomically claim, update, and release a ticket",
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
				Usage:       "Author name for the one-shot claim",
				Required:    true,
				Destination: &author,
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

			input := service.OneShotUpdateInput{
				TicketID:    ticketID,
				Author:      parsedAuthor,
				FacetRemove: cmd.StringSlice("facet-remove"),
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

			svc := f.Tracker()
			if err := svc.OneShotUpdate(ctx, input); err != nil {
				return fmt.Errorf("editing ticket: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, editOutput{
					TicketID: ticketID.String(),
					Updated:  true,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Edited %s\n",
				cs.SuccessIcon(), cs.Bold(ticketID.String()))
			return err
		},
	}
}

package claim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// claimOutput is the JSON representation shared by both the "id" and "ready"
// subcommands, since both return a ticket ID, claim ID, and stolen flag.
type claimOutput struct {
	TicketID string `json:"ticket_id"`
	ClaimID  string `json:"claim_id"`
	Stolen   bool   `json:"stolen"`
}

// NewCmd constructs the "claim" parent command with "id" and "ready"
// subcommands for all claiming workflows.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "claim",
		Usage: "Claim tickets (by ID or next ready)",
		Commands: []*cli.Command{
			newIDCmd(f),
			newReadyCmd(f),
		},
	}
}

// newIDCmd constructs the "id" subcommand, which claims a specific ticket
// by its ID.
func newIDCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput     bool
		author         string
		steal          bool
		staleThreshold string
	)

	return &cli.Command{
		Name:      "id",
		Usage:     "Claim a specific ticket by ID",
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
				Sources:     cli.EnvVars("NP_AUTHOR"),
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

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			result, err := svc.ClaimByID(ctx, input)
			if err != nil {
				return fmt.Errorf("claiming ticket: %w", err)
			}

			return writeClaimResult(f, jsonOutput, result)
		},
	}
}

// newReadyCmd constructs the "ready" subcommand, which claims the
// highest-priority ready ticket matching optional filters.
func newReadyCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput     bool
		author         string
		role           string
		stealIfNeeded  bool
		staleThreshold string
	)

	return &cli.Command{
		Name:  "ready",
		Usage: "Claim the next ready ticket",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name for the claim",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "role",
				Aliases:     []string{"r"},
				Usage:       "Filter by role: task or epic",
				Destination: &role,
			},
			&cli.StringSliceFlag{
				Name:  "facet",
				Usage: "Facet filter in key:value format (repeatable)",
			},
			&cli.BoolFlag{
				Name:        "steal-if-needed",
				Aliases:     []string{"steal"},
				Usage:       "Fall back to stealing a stale claim if no unclaimed tickets are ready",
				Destination: &stealIfNeeded,
			},
			&cli.StringFlag{
				Name:        "stale-threshold",
				Usage:       "Duration after which the claim becomes stale (e.g., 30m, 1h)",
				Destination: &staleThreshold,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			var parsedRole ticket.Role
			if role != "" {
				parsedRole, err = ticket.ParseRole(role)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
			}

			// Parse facet filters.
			rawFacets := cmd.StringSlice("facet")
			facetFilters, err := parseFacetFilters(rawFacets)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			var threshold time.Duration
			if staleThreshold != "" {
				threshold, err = time.ParseDuration(staleThreshold)
				if err != nil {
					return cmdutil.FlagErrorf("invalid stale threshold: %s", err)
				}
			}

			input := service.ClaimNextReadyInput{
				Author:         parsedAuthor,
				Role:           parsedRole,
				FacetFilters:   facetFilters,
				StealIfNeeded:  stealIfNeeded,
				StaleThreshold: threshold,
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			result, err := svc.ClaimNextReady(ctx, input)
			if err != nil {
				return fmt.Errorf("claiming next ready ticket: %w", err)
			}

			return writeClaimResult(f, jsonOutput, result)
		},
	}
}

// writeClaimResult renders either JSON or human-readable output for a claim
// operation. Both subcommands share the same output structure.
func writeClaimResult(f *cmdutil.Factory, jsonOutput bool, result service.ClaimOutput) error {
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

	_, err := fmt.Fprintf(out, "%s %s %s\n  Claim ID: %s\n",
		cs.SuccessIcon(),
		verb,
		cs.Bold(result.TicketID.String()),
		cs.Cyan(result.ClaimID))
	return err
}

// parseFacetFilters converts a slice of "key:value" strings into port.FacetFilter
// values. A value of "*" is treated as a wildcard (match any value for that key).
func parseFacetFilters(raw []string) ([]port.FacetFilter, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	filters := make([]port.FacetFilter, 0, len(raw))
	for _, s := range raw {
		key, value, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fmt.Errorf("invalid facet filter %q: must be in key:value format", s)
		}
		ff := port.FacetFilter{Key: key}
		if value != "*" {
			ff.Value = value
		}
		filters = append(filters, ff)
	}

	return filters, nil
}

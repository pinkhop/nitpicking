package next

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

// nextOutput is the JSON representation of the next command result.
type nextOutput struct {
	TicketID string `json:"ticket_id"`
	ClaimID  string `json:"claim_id"`
	Stolen   bool   `json:"stolen"`
}

// NewCmd constructs the "next" command, which claims the highest-priority
// ready ticket matching optional filters.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput     bool
		author         string
		role           string
		stealFallback  bool
		staleThreshold string
	)

	return &cli.Command{
		Name:  "next",
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
				Name:        "steal-fallback",
				Usage:       "Fall back to stealing a stale claim if no unclaimed tickets are ready",
				Destination: &stealFallback,
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
				StealFallback:  stealFallback,
				StaleThreshold: threshold,
			}

			svc := f.Tracker()
			result, err := svc.ClaimNextReady(ctx, input)
			if err != nil {
				return fmt.Errorf("claiming next ready ticket: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, nextOutput{
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

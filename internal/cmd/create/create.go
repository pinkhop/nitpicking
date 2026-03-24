package create

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// createOutput is the JSON representation of the create command result.
type createOutput struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Title     string `json:"title"`
	Priority  string `json:"priority"`
	State     string `json:"state"`
	ClaimID   string `json:"claim_id,omitzero"`
	CreatedAt string `json:"created_at"`
}

// NewCmd constructs the "create" command, which creates a new ticket (task or
// epic) with the specified attributes.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput         bool
		role               string
		title              string
		description        string
		acceptanceCriteria string
		priority           string
		parent             string
		facets             []string
		claim              bool
		author             string
		idempotencyKey     string
	)

	return &cli.Command{
		Name:  "create",
		Usage: "Create a new ticket",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "role",
				Aliases:     []string{"r"},
				Usage:       "Ticket role: task or epic",
				Required:    true,
				Destination: &role,
			},
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "Ticket title",
				Required:    true,
				Destination: &title,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "Ticket description",
				Destination: &description,
			},
			&cli.StringFlag{
				Name:        "acceptance-criteria",
				Usage:       "Acceptance criteria for the ticket",
				Destination: &acceptanceCriteria,
			},
			&cli.StringFlag{
				Name:        "priority",
				Aliases:     []string{"p"},
				Usage:       "Priority level: P0–P4 (default P2)",
				Destination: &priority,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "Parent epic ticket ID",
				Destination: &parent,
			},
			&cli.StringSliceFlag{
				Name:  "facet",
				Usage: "Facet in key:value format (repeatable)",
			},
			&cli.BoolFlag{
				Name:        "claim",
				Usage:       "Immediately claim the ticket after creation",
				Destination: &claim,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (required when --claim is set)",
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "idempotency-key",
				Usage:       "Idempotency key for deduplication",
				Destination: &idempotencyKey,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Parse role.
			parsedRole, err := ticket.ParseRole(role)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Parse optional priority.
			var parsedPriority ticket.Priority
			if priority != "" {
				parsedPriority, err = ticket.ParsePriority(priority)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
			}

			// Parse optional parent ID.
			var parentID ticket.ID
			if parent != "" {
				parentID, err = ticket.ParseID(parent)
				if err != nil {
					return cmdutil.FlagErrorf("invalid parent ID: %s", err)
				}
			}

			// Parse facets: merge NP_FACETS env var (space-separated) with
			// explicit --facet flags. Explicit flags take precedence.
			facets = cmd.StringSlice("facet")
			facets = mergeFacetSources(os.Getenv("NP_FACETS"), facets)
			parsedFacets, err := parseFacets(facets)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Parse author if claiming.
			var parsedAuthor identity.Author
			if claim {
				if author == "" {
					return cmdutil.FlagErrorf("--author is required when --claim is set")
				}
				parsedAuthor, err = identity.NewAuthor(author)
				if err != nil {
					return cmdutil.FlagErrorf("invalid author: %s", err)
				}
			}

			input := service.CreateTicketInput{
				Role:               parsedRole,
				Title:              title,
				Description:        description,
				AcceptanceCriteria: acceptanceCriteria,
				Priority:           parsedPriority,
				ParentID:           parentID,
				Facets:             parsedFacets,
				Author:             parsedAuthor,
				Claim:              claim,
				IdempotencyKey:     idempotencyKey,
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			result, err := svc.CreateTicket(ctx, input)
			if err != nil {
				return fmt.Errorf("creating ticket: %w", err)
			}

			t := result.Ticket
			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, createOutput{
					ID:        t.ID().String(),
					Role:      t.Role().String(),
					Title:     t.Title(),
					Priority:  t.Priority().String(),
					State:     t.State().String(),
					ClaimID:   result.ClaimID,
					CreatedAt: t.CreatedAt().Format(time.RFC3339),
				})
			}

			cs := f.IOStreams.ColorScheme()
			out := f.IOStreams.Out
			_, err = fmt.Fprintf(out, "%s Created %s %s — %s\n",
				cs.SuccessIcon(),
				t.Role().String(),
				cs.Bold(t.ID().String()),
				t.Title())
			if err != nil {
				return err
			}

			if result.ClaimID != "" {
				_, err = fmt.Fprintf(out, "  Claim ID: %s\n", cs.Cyan(result.ClaimID))
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
}

// parseFacets converts a slice of "key:value" strings into a slice of
// validated ticket.Facet values.
// mergeFacetSources combines facets from the NP_FACETS env var (space-separated
// key:value pairs) with explicitly provided --facet flag values. Explicit flags
// take precedence: if the same key appears in both sources, the flag value wins.
func mergeFacetSources(envValue string, flagFacets []string) []string {
	if envValue == "" {
		return flagFacets
	}

	// Build a set of keys provided by explicit flags.
	explicitKeys := make(map[string]bool)
	for _, f := range flagFacets {
		key, _, ok := strings.Cut(f, ":")
		if ok {
			explicitKeys[key] = true
		}
	}

	// Prepend env facets that don't conflict with explicit flags.
	envParts := strings.Fields(envValue)
	merged := make([]string, 0, len(envParts)+len(flagFacets))
	for _, part := range envParts {
		key, _, ok := strings.Cut(part, ":")
		if ok && !explicitKeys[key] {
			merged = append(merged, part)
		}
	}
	merged = append(merged, flagFacets...)

	return merged
}

func parseFacets(raw []string) ([]ticket.Facet, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	facets := make([]ticket.Facet, 0, len(raw))
	for _, s := range raw {
		key, value, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fmt.Errorf("invalid facet %q: must be in key:value format", s)
		}
		f, err := ticket.NewFacet(key, value)
		if err != nil {
			return nil, fmt.Errorf("invalid facet %q: %w", s, err)
		}
		facets = append(facets, f)
	}

	return facets, nil
}

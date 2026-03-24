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
		fromJSON           string
		role               string
		title              string
		description        string
		acceptanceCriteria string
		priority           string
		parent             string
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
				Name:        "from-json",
				Usage:       `JSON string with ticket fields (use "-" to read from stdin)`,
				Destination: &fromJSON,
			},
			&cli.StringFlag{
				Name:        "role",
				Aliases:     []string{"r"},
				Usage:       "Ticket role: task or epic",
				Destination: &role,
			},
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "Ticket title",
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
				Usage:       "Author name (required)",
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "idempotency-key",
				Usage:       "Idempotency key for deduplication",
				Destination: &idempotencyKey,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// If --from-json is provided, parse it and apply JSON values as
			// defaults for any fields not explicitly set via flags. Precedence
			// (highest to lowest): flags > JSON > env vars.
			var tj ticketJSON
			if fromJSON != "" {
				data, err := readJSONSource(fromJSON, f.IOStreams.In)
				if err != nil {
					return err
				}
				parsed, err := parseTicketJSON(data)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
				tj = parsed

				// Apply JSON defaults where flags were not explicitly set.
				if !cmd.IsSet("role") && tj.Role != "" {
					role = tj.Role
				}
				if !cmd.IsSet("title") && tj.Title != "" {
					title = tj.Title
				}
				if !cmd.IsSet("description") && tj.Description != "" {
					description = tj.Description
				}
				if !cmd.IsSet("acceptance-criteria") && tj.AcceptanceCriteria != "" {
					acceptanceCriteria = tj.AcceptanceCriteria
				}
				if !cmd.IsSet("priority") && tj.Priority != "" {
					priority = tj.Priority
				}
				if !cmd.IsSet("parent") && tj.ParentID != "" {
					parent = tj.ParentID
				}
			}

			// Validate required fields (may come from flags or JSON).
			if role == "" {
				return cmdutil.FlagErrorf("--role is required (via flag or --from-json)")
			}
			if title == "" {
				return cmdutil.FlagErrorf("--title is required (via flag or --from-json)")
			}

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

			// Parse facets: three-way merge of env, JSON, and flags.
			// Precedence: flags > JSON > env. Different keys are merged;
			// same key uses the highest-precedence source.
			flagFacets := cmd.StringSlice("facet")
			envFacets := envFacetStrings(os.Getenv("NP_FACETS"))
			jsonFacets := jsonFacetsToStrings(tj.Facets)
			mergedFacets := mergeFacetsFromJSON(envFacets, jsonFacets, flagFacets)
			parsedFacets, err := parseFacets(mergedFacets)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Parse author — required for all creates.
			if author == "" {
				return cmdutil.FlagErrorf("--author is required")
			}
			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			// Parse optional parent ID.
			var parentID ticket.ID
			if parent != "" {
				parentID, err = resolver.Resolve(ctx, parent)
				if err != nil {
					return cmdutil.FlagErrorf("invalid parent ID: %s", err)
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

// envFacetStrings splits the NP_FACETS env var (space-separated key:value
// pairs) into individual facet strings.
func envFacetStrings(envValue string) []string {
	if envValue == "" {
		return nil
	}
	return strings.Fields(envValue)
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

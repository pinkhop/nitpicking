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
	"github.com/pinkhop/nitpicking/internal/domain/issue"
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

// NewCmd constructs the "create" command, which creates a new issue (task or
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
		Usage: "Create a new issue",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "from-json",
				Usage:       `JSON string with issue fields (use "-" to read from stdin)`,
				Category:    "Options",
				Destination: &fromJSON,
			},
			&cli.StringFlag{
				Name:        "role",
				Aliases:     []string{"r"},
				Usage:       "Issue role: task or epic",
				Category:    "Options",
				Destination: &role,
			},
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "Issue title",
				Category:    "Options",
				Destination: &title,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "Issue description",
				Category:    "Options",
				Destination: &description,
			},
			&cli.StringFlag{
				Name:        "acceptance-criteria",
				Usage:       "Acceptance criteria for the issue",
				Category:    "Options",
				Destination: &acceptanceCriteria,
			},
			&cli.StringFlag{
				Name:        "priority",
				Aliases:     []string{"p"},
				Usage:       "Priority level: P0–P4 (default P2)",
				Category:    "Options",
				Destination: &priority,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "Parent epic issue ID",
				Category:    "Options",
				Destination: &parent,
			},
			&cli.StringSliceFlag{
				Name:     "dimension",
				Usage:    "Dimension in key:value format (repeatable)",
				Category: "Options",
			},
			&cli.BoolFlag{
				Name:        "claim",
				Usage:       "Immediately claim the issue after creation",
				Category:    "Options",
				Destination: &claim,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (required)",
				Category:    "Options",
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "idempotency-key",
				Usage:       "Idempotency key for deduplication",
				Category:    "Options",
				Destination: &idempotencyKey,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// If --from-json is provided, parse it and apply JSON values as
			// defaults for any fields not explicitly set via flags. Precedence
			// (highest to lowest): flags > JSON > env vars.
			var tj issueJSON
			if fromJSON != "" {
				data, err := readJSONSource(fromJSON, f.IOStreams.In)
				if err != nil {
					return err
				}
				parsed, err := parseIssueJSON(data)
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
			parsedRole, err := issue.ParseRole(role)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Parse optional priority.
			var parsedPriority issue.Priority
			if priority != "" {
				parsedPriority, err = issue.ParsePriority(priority)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
			}

			// Parse dimensions: three-way merge of env, JSON, and flags.
			// Precedence: flags > JSON > env. Different keys are merged;
			// same key uses the highest-precedence source.
			flagDimensions := cmd.StringSlice("dimension")
			envDimensions := envDimensionStrings(os.Getenv("NP_DIMENSIONS"))
			jsonDimensions := jsonDimensionsToStrings(tj.Dimensions)
			mergedDimensions := mergeDimensionsFromJSON(envDimensions, jsonDimensions, flagDimensions)
			parsedDimensions, err := parseDimensions(mergedDimensions)
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
			var parentID issue.ID
			if parent != "" {
				parentID, err = resolver.Resolve(ctx, parent)
				if err != nil {
					return cmdutil.FlagErrorf("invalid parent ID: %s", err)
				}
			}

			input := service.CreateIssueInput{
				Role:               parsedRole,
				Title:              title,
				Description:        description,
				AcceptanceCriteria: acceptanceCriteria,
				Priority:           parsedPriority,
				ParentID:           parentID,
				Dimensions:         parsedDimensions,
				Author:             parsedAuthor,
				Claim:              claim,
				IdempotencyKey:     idempotencyKey,
			}
			result, err := svc.CreateIssue(ctx, input)
			if err != nil {
				return fmt.Errorf("creating issue: %w", err)
			}

			t := result.Issue
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

// envDimensionStrings splits the NP_DIMENSIONS env var (space-separated key:value
// pairs) into individual dimension strings.
func envDimensionStrings(envValue string) []string {
	if envValue == "" {
		return nil
	}
	return strings.Fields(envValue)
}

func parseDimensions(raw []string) ([]issue.Dimension, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	dimensions := make([]issue.Dimension, 0, len(raw))
	for _, s := range raw {
		key, value, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fmt.Errorf("invalid dimension %q: must be in key:value format", s)
		}
		f, err := issue.NewDimension(key, value)
		if err != nil {
			return nil, fmt.Errorf("invalid dimension %q: %w", s, err)
		}
		dimensions = append(dimensions, f)
	}

	return dimensions, nil
}

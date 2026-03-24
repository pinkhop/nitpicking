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
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// claimOutput is the JSON representation shared by both the "id" and "ready"
// subcommands, since both return an issue ID, claim ID, and stolen flag.
type claimOutput struct {
	IssueID string `json:"issue_id"`
	ClaimID string `json:"claim_id"`
	Stolen  bool   `json:"stolen"`
}

// NewCmd constructs the "claim" parent command with "id" and "ready"
// subcommands for all claiming workflows.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "claim",
		Usage: "Claim issues (by ID or next ready)",
		Commands: []*cli.Command{
			newIDCmd(f),
			newReadyCmd(f),
		},
	}
}

// newIDCmd constructs the "id" subcommand, which claims a specific issue
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
		Usage:     "Claim a specific issue by ID",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name for the claim (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.BoolFlag{
				Name:        "steal",
				Usage:       "Steal the claim from another agent if already claimed",
				Category:    "Options",
				Destination: &steal,
			},
			&cli.StringFlag{
				Name:        "stale-threshold",
				Usage:       "Duration after which the claim becomes stale (e.g., 30m, 1h)",
				Category:    "Options",
				Destination: &staleThreshold,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("issue ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
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
				IssueID:        issueID,
				Author:         parsedAuthor,
				AllowSteal:     steal,
				StaleThreshold: threshold,
			}
			result, err := svc.ClaimByID(ctx, input)
			if err != nil {
				return fmt.Errorf("claiming issue: %w", err)
			}

			return writeClaimResult(f, jsonOutput, result)
		},
	}
}

// newReadyCmd constructs the "ready" subcommand, which claims the
// highest-priority ready issue matching optional filters.
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
		Usage: "Claim the next ready issue",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name for the claim (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "role",
				Aliases:     []string{"r"},
				Usage:       "Filter by role: task or epic",
				Category:    "Options",
				Destination: &role,
			},
			&cli.StringSliceFlag{
				Name:     "dimension",
				Usage:    "Dimension filter in key:value format (repeatable)",
				Category: "Options",
			},
			&cli.BoolFlag{
				Name:        "steal-if-needed",
				Aliases:     []string{"steal"},
				Usage:       "Fall back to stealing a stale claim if no unclaimed issues are ready",
				Category:    "Options",
				Destination: &stealIfNeeded,
			},
			&cli.StringFlag{
				Name:        "stale-threshold",
				Usage:       "Duration after which the claim becomes stale (e.g., 30m, 1h)",
				Category:    "Options",
				Destination: &staleThreshold,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			var parsedRole issue.Role
			if role != "" {
				parsedRole, err = issue.ParseRole(role)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
			}

			// Parse dimension filters.
			rawDimensions := cmd.StringSlice("dimension")
			dimensionFilters, err := parseDimensionFilters(rawDimensions)
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
				Author:           parsedAuthor,
				Role:             parsedRole,
				DimensionFilters: dimensionFilters,
				StealIfNeeded:    stealIfNeeded,
				StaleThreshold:   threshold,
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			result, err := svc.ClaimNextReady(ctx, input)
			if err != nil {
				return fmt.Errorf("claiming next ready issue: %w", err)
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
			IssueID: result.IssueID.String(),
			ClaimID: result.ClaimID,
			Stolen:  result.Stolen,
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
		cs.Bold(result.IssueID.String()),
		cs.Cyan(result.ClaimID))
	return err
}

// parseDimensionFilters converts a slice of "key:value" strings into port.DimensionFilter
// values. A value of "*" is treated as a wildcard (match any value for that key).
func parseDimensionFilters(raw []string) ([]port.DimensionFilter, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	filters := make([]port.DimensionFilter, 0, len(raw))
	for _, s := range raw {
		key, value, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fmt.Errorf("invalid dimension filter %q: must be in key:value format", s)
		}
		ff := port.DimensionFilter{Key: key}
		if value != "*" {
			ff.Value = value
		}
		filters = append(filters, ff)
	}

	return filters, nil
}

package claim

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// claimOutput is the JSON representation of a claim result, since both
// claim-by-ID and claim-ready return an issue ID, claim ID, author, and
// timestamps.
type claimOutput struct {
	IssueID   string `json:"issue_id"`
	ClaimID   string `json:"claim_id"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	StaleAt   string `json:"stale_at"`
}

// RunClaimByIDInput holds the parameters for claiming an issue by ID,
// decoupled from CLI flag parsing so it can be tested directly.
type RunClaimByIDInput struct {
	Service      driving.Service
	IssueID      string
	Author       string
	Duration     time.Duration
	StaleAt      time.Time
	LabelFilters []driving.LabelFilterInput
	Role         string
	JSON         bool
	WriteTo      io.Writer
}

// RunClaimByID executes the claim-by-ID workflow: claims the specified issue
// and writes the result to the output writer.
func RunClaimByID(ctx context.Context, input RunClaimByIDInput) error {
	var roleFilter domain.Role
	if input.Role != "" {
		var roleErr error
		roleFilter, roleErr = domain.ParseRole(input.Role)
		if roleErr != nil {
			return fmt.Errorf("invalid role %q: must be task or epic", input.Role)
		}
	}

	result, err := input.Service.ClaimByID(ctx, driving.ClaimInput{
		IssueID:        input.IssueID,
		Author:         input.Author,
		StaleThreshold: input.Duration,
		StaleAt:        input.StaleAt,
		LabelFilters:   input.LabelFilters,
		Role:           roleFilter,
	})
	if err != nil {
		return fmt.Errorf("claiming issue: %w", err)
	}

	return writeClaimOutput(input.WriteTo, input.JSON, result)
}

// RunClaimReadyInput holds the parameters for claiming the next ready issue,
// decoupled from CLI flag parsing so it can be tested directly.
type RunClaimReadyInput struct {
	Service      driving.Service
	Author       string
	Role         string
	LabelFilters []driving.LabelFilterInput
	Duration     time.Duration
	StaleAt      time.Time
	JSON         bool
	WriteTo      io.Writer
}

// RunClaimReady executes the claim-ready workflow: finds and claims the
// highest-priority ready issue matching the given filters.
func RunClaimReady(ctx context.Context, input RunClaimReadyInput) error {
	var roleFilter domain.Role
	if input.Role != "" {
		var roleErr error
		roleFilter, roleErr = domain.ParseRole(input.Role)
		if roleErr != nil {
			return fmt.Errorf("invalid role %q: must be task or epic", input.Role)
		}
	}
	result, err := input.Service.ClaimNextReady(ctx, driving.ClaimNextReadyInput{
		Author:         input.Author,
		Role:           roleFilter,
		LabelFilters:   input.LabelFilters,
		StaleThreshold: input.Duration,
		StaleAt:        input.StaleAt,
	})
	if err != nil {
		return fmt.Errorf("claiming next ready issue: %w", err)
	}

	return writeClaimOutput(input.WriteTo, input.JSON, result)
}

// NewCmd constructs the unified "claim" command that takes a required
// positional argument: either an issue ID or the literal word "ready"
// (case-insensitive).
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
		role       string
		duration   string
		staleAtRaw string
	)

	return &cli.Command{
		Name:      "claim",
		Usage:     "Claim an issue by ID or the next ready issue",
		ArgsUsage: "<ISSUE-ID | ready>",
		Description: `Claiming an issue is the gateway to modifying it. All mutations (state
transitions, field updates, closing) require an active claim. A claim is
bearer-authenticated: whoever holds the claim ID is the authorized author
for subsequent operations on that issue.

There are two modes. Pass an issue ID to claim a specific issue you already
know about. Pass the literal word "ready" to let np pick the highest-priority
ready issue for you — this is the standard starting point for agents that need
work. Use --role and --label to narrow what "ready" considers.

Claims expire after a configurable duration (default 2 hours). Stale claims
are treated as nonexistent: any agent can overwrite them by claiming normally.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name for the claim (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &author,
			},
			// --role and --label intentionally share the same bare flag names as
			// np list and np ready, but their semantics differ in an important way.
			//
			// In np list / np ready, these flags are advisory browse filters: they
			// narrow the display set without any side effect. In np claim, they
			// are atomic pre-claim guards: when "ready" mode is used, the filter
			// is applied inside the claim transaction so only a matching issue is
			// claimed; when claiming by ID, the flags act as guard-rail assertions
			// that reject the claim if the issue does not match (preventing agents
			// from accidentally claiming the wrong kind of issue).
			//
			// They were previously named --with-role / --with-label to make this
			// semantic distinction visible at the call site. The names were unified
			// with list/ready in commit 7d2730e so the CLI vocabulary is
			// consistent, but the underlying behaviour was not changed.
			// Resist the temptation to merge the implementations: the flag names
			// are the same, but the contract is different.
			&cli.StringFlag{
				Name:        "role",
				Usage:       "Filter by role: task or epic",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &role,
			},
			&cli.StringSliceFlag{
				Name:     "label",
				Usage:    "Label filter in key:value or key:* format (repeatable, AND semantics)",
				Category: cmdutil.FlagCategorySupplemental,
			},
			&cli.StringFlag{
				Name:        "duration",
				Usage:       "Duration after which the claim becomes stale (e.g., 30m, 1h)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &duration,
			},
			&cli.StringFlag{
				Name:        "stale-at",
				Usage:       "RFC3339 UTC timestamp when the claim becomes stale (e.g., 2026-04-02T14:00:00Z); mutually exclusive with --duration",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &staleAtRaw,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			arg := cmd.Args().Get(0)
			if arg == "" {
				return cmdutil.FlagErrorf("positional argument is required: <ISSUE-ID | ready>")
			}

			// Parse label filters — must be key:value or key:*, not key-only.
			rawLabels := cmd.StringSlice("label")
			labelFilters, err := parseLabelFiltersStrict(rawLabels)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Mutual exclusivity: --duration and --stale-at cannot both be set.
			if duration != "" && staleAtRaw != "" {
				return cmdutil.FlagErrorf("--duration and --stale-at are mutually exclusive")
			}

			var dur time.Duration
			if duration != "" {
				dur, err = time.ParseDuration(duration)
				if err != nil {
					return cmdutil.FlagErrorf("invalid duration: %s", err)
				}
			}

			var staleAt time.Time
			if staleAtRaw != "" {
				staleAt, err = parseStaleAt(staleAtRaw)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			// Dispatch: "ready" (case-insensitive) → claim next ready;
			// anything else → claim by issue ID.
			if strings.EqualFold(arg, "ready") {
				return RunClaimReady(ctx, RunClaimReadyInput{
					Service:      svc,
					Author:       author,
					Role:         role,
					LabelFilters: labelFilters,
					Duration:     dur,
					StaleAt:      staleAt,
					JSON:         jsonOutput,
					WriteTo:      f.IOStreams.Out,
				})
			}

			resolver := cmdutil.NewIDResolver(svc)
			issueID, err := resolver.Resolve(ctx, arg)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			return RunClaimByID(ctx, RunClaimByIDInput{
				Service:      svc,
				IssueID:      issueID.String(),
				Author:       author,
				Duration:     dur,
				StaleAt:      staleAt,
				LabelFilters: labelFilters,
				Role:         role,
				JSON:         jsonOutput,
				WriteTo:      f.IOStreams.Out,
			})
		},
	}
}

// maxStaleAtDistance is the maximum allowed distance between now and a
// --stale-at timestamp. Matches domain.MaxStaleThreshold (24h) but is
// defined locally to keep the CLI validation self-contained.
const maxStaleAtDistance = 24 * time.Hour

// parseStaleAt validates and parses a --stale-at flag value. The value must
// be a valid RFC3339 timestamp in UTC (ending in "Z"), in the future, and
// within 24 hours from now.
func parseStaleAt(raw string) (time.Time, error) {
	// Require UTC suffix — reject non-UTC offsets before even attempting
	// to parse, so the error message is specific.
	if len(raw) == 0 || raw[len(raw)-1] != 'Z' {
		return time.Time{}, fmt.Errorf("--stale-at must be a UTC timestamp ending in Z, got %q", raw)
	}

	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("--stale-at must be a valid RFC3339 timestamp: %s", err)
	}

	now := time.Now()
	if !t.After(now) {
		return time.Time{}, fmt.Errorf("--stale-at must be in the future, got %s", raw)
	}

	if t.Sub(now) > maxStaleAtDistance {
		return time.Time{}, fmt.Errorf("--stale-at must be within 24h from now, got %s", raw)
	}

	return t, nil
}

// parseLabelFiltersStrict parses label filter strings with strict validation:
// each filter must be in key:value or key:* format. A bare key without a colon
// is rejected — unlike the general ParseLabelFilters which accepts key:value.
func parseLabelFiltersStrict(raw []string) ([]driving.LabelFilterInput, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	filters := make([]driving.LabelFilterInput, 0, len(raw))
	for _, s := range raw {
		key, value, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fmt.Errorf("invalid label filter %q: must be in key:value or key:* format", s)
		}

		ff := driving.LabelFilterInput{Key: key}
		if value != "*" {
			ff.Value = value
		}
		filters = append(filters, ff)
	}

	return filters, nil
}

// writeClaimOutput renders either JSON or human-readable text output for a
// claim operation. Both claim-by-ID and claim-ready share the same output
// structure.
func writeClaimOutput(w io.Writer, jsonOut bool, result driving.ClaimOutput) error {
	if jsonOut {
		return cmdutil.WriteJSON(w, claimOutput{
			IssueID:   result.IssueID,
			ClaimID:   result.ClaimID,
			Author:    result.Author,
			CreatedAt: result.CreatedAt.Format(time.RFC3339),
			StaleAt:   result.StaleAt.Format(time.RFC3339),
		})
	}

	_, err := fmt.Fprintf(w, "Claimed %s\n  Claim ID: %s\n  Author: %s\n  Stale at: %s\n",
		result.IssueID,
		result.ClaimID,
		result.Author,
		result.StaleAt.Format(time.DateTime))
	return err
}

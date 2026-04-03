package list

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RunInput holds the parameters for the list command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	Filter        driving.IssueFilterInput
	OrderBy       driving.OrderBy
	Limit         int
	JSON          bool
	Timestamps    bool
	WriteTo       io.Writer
	TerminalWidth int
	ColorScheme   *iostreams.ColorScheme
}

// Run executes the list workflow: queries the service for issues matching the
// given filter and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Service.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  input.Filter,
		OrderBy: input.OrderBy,
		Limit:   input.Limit,
	})
	if err != nil {
		return fmt.Errorf("listing issues: %w", err)
	}

	if input.JSON {
		out := cmdutil.ListOutput{
			HasMore: result.HasMore,
			Items:   cmdutil.ConvertListItems(result.Items),
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	// Human-readable output.
	w := input.WriteTo
	cs := input.ColorScheme
	if cs == nil {
		cs = iostreams.NewColorScheme(false)
	}

	if len(result.Items) == 0 {
		_, _ = fmt.Fprintln(w, "No issues found.")
		return nil
	}

	// Estimate non-title column overhead for title truncation.
	// Without timestamps: ID(~8) + role(4) + status(7) + priority(2) + padding(8) = ~29.
	// With timestamps: add timestamp(19) + padding(2) = ~50.
	overhead := 29
	if input.Timestamps {
		overhead = 50
	}
	maxTitle := cmdutil.AvailableTitleWidth(input.TerminalWidth, overhead)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, item := range result.Items {
		title := item.Title
		if len(item.BlockerIDs) > 0 {
			title += " " + cmdutil.FormatBlockerSuffix(item.BlockerIDs)
		}
		title = cmdutil.TruncateTitle(title, maxTitle)
		stateCol := cmdutil.FormatState(cs, item.State, item.SecondaryState)
		if input.Timestamps {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.ID,
				item.Role,
				stateCol,
				item.Priority,
				item.CreatedAt.Format(time.DateTime),
				title)
		} else {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.ID,
				item.Role,
				stateCol,
				item.Priority,
				title)
		}
	}
	_ = tw.Flush()

	shown := len(result.Items)
	_, _ = fmt.Fprintf(w, "\n%d issues\n", shown)

	return nil
}

// NewCmd constructs the "list" command, which returns a filtered, ordered,
// paginated list of issues.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput    bool
		ready         bool
		includeClosed bool
		order         string
		limit         int
		all           bool
		timestamps    bool
	)

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List issues",
		Description: `Lists issues matching the given filters, ordered by priority (default),
creation time, or modification time. By default, closed issues are hidden
and the result set is capped at a terminal-friendly limit.

This is the general-purpose query command. Use it to browse the backlog,
check the state of a specific parent's children (--parent), find issues
with a particular label (--label), or view only ready issues (--ready).
For the common case of "what can I work on next?", the dedicated "ready"
command is a convenient shortcut.

Filters combine with AND semantics: --role task --state open --label kind:bug
returns only open tasks labeled kind:bug. Use --all or --limit to control
pagination, --include-closed to see resolved issues, and --json for
machine-readable output.`,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:     "role",
				Aliases:  []string{"r"},
				Usage:    "Filter by role: task or epic (repeatable)",
				Category: cmdutil.FlagCategorySupplemental,
			},
			&cli.StringSliceFlag{
				Name:     "state",
				Aliases:  []string{"s"},
				Usage:    "Filter by state: open, claimed, closed, deferred (repeatable)",
				Category: cmdutil.FlagCategorySupplemental,
			},
			&cli.BoolFlag{
				Name:        "ready",
				Usage:       "Show only issues in the ready secondary state",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &ready,
			},
			&cli.BoolFlag{
				Name:        "include-closed",
				Usage:       "Include closed issues in the output (hidden by default)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &includeClosed,
			},
			&cli.StringSliceFlag{
				Name:     "parent",
				Usage:    "Filter by parent epic ID (repeatable)",
				Category: cmdutil.FlagCategorySupplemental,
			},
			&cli.StringSliceFlag{
				Name:     "label",
				Usage:    "Label filter in key:value format (repeatable)",
				Category: cmdutil.FlagCategorySupplemental,
			},
			&cli.StringFlag{
				Name:        "order",
				Usage:       "Sort order: priority, created, modified (default: priority)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &order,
			},
			&cli.BoolFlag{
				Name:        "timestamps",
				Usage:       "Include created_at timestamp in text output",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &timestamps,
			},
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results (0 = default, negative = unlimited)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &limit,
			},
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "Return all results without limit",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &all,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			var filter driving.IssueFilterInput
			filter.Ready = ready

			for _, r := range cmd.StringSlice("role") {
				role, err := domain.ParseRole(r)
				if err != nil {
					return cmdutil.FlagErrorf("invalid role %q: must be task or epic", r)
				}
				filter.Roles = append(filter.Roles, role)
			}

			rawStates := cmd.StringSlice("state")
			for _, s := range rawStates {
				st, err := domain.ParseState(s)
				if err != nil {
					return cmdutil.FlagErrorf("invalid state %q: %v", s, err)
				}
				filter.States = append(filter.States, st)
			}
			filter.ExcludeClosed = !includeClosed && len(filter.States) == 0

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			rawParents := cmd.StringSlice("parent")
			for _, p := range rawParents {
				parentID, err := resolver.Resolve(ctx, p)
				if err != nil {
					return cmdutil.FlagErrorf("invalid parent ID: %s", err)
				}
				filter.ParentIDs = append(filter.ParentIDs, parentID.String())
			}

			// Parse label filters.
			rawLabels := cmd.StringSlice("label")
			labelFilters, parseErr := cmdutil.ParseLabelFilters(rawLabels)
			if parseErr != nil {
				return cmdutil.FlagErrorf("%s", parseErr)
			}
			filter.LabelFilters = labelFilters

			orderBy, err := cmdutil.ParseOrderBy(order)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			effectiveLimit := cmdutil.ResolveLimit(limit, all, f.IOStreams.IsStdoutTTY())

			return Run(ctx, RunInput{
				Service:       svc,
				Filter:        filter,
				OrderBy:       orderBy,
				Limit:         effectiveLimit,
				JSON:          jsonOutput,
				Timestamps:    timestamps,
				WriteTo:       f.IOStreams.Out,
				TerminalWidth: f.IOStreams.TerminalWidth(),
				ColorScheme:   f.IOStreams.ColorScheme(),
			})
		},
	}
}

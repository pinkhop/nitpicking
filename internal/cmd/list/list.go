package list

import (
	"context"
	"fmt"
	"io"

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
	Direction     driving.SortDirection
	Limit         int
	JSON          bool
	Columns       []cmdutil.Column
	WriteTo       io.Writer
	TerminalWidth int
	ColorScheme   *iostreams.ColorScheme
}

// Run executes the list workflow: queries the service for issues matching the
// given filter and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Service.ListIssues(ctx, driving.ListIssuesInput{
		Filter:    input.Filter,
		OrderBy:   input.OrderBy,
		Direction: input.Direction,
		Limit:     input.Limit,
	})
	if err != nil {
		return fmt.Errorf("listing issues: %w", err)
	}

	if input.JSON {
		out := cmdutil.ListOutput{
			HasMore: result.HasMore,
			Issues:  cmdutil.ConvertListItems(result.Items),
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

	cols := input.Columns
	if len(cols) == 0 {
		cols = cmdutil.DefaultColumns
	}

	overhead := cmdutil.OverheadForColumns(cols)
	maxTitle := cmdutil.AvailableTitleWidth(input.TerminalWidth, overhead)

	tw := cmdutil.NewTableWriter(w, 2)
	tw.AddRow(cmdutil.ColumnarHeaderCells(cols)...)

	rc := cmdutil.RenderContext{
		ColorScheme:   cs,
		MaxTitleWidth: maxTitle,
	}
	for _, item := range result.Items {
		tw.AddRow(cmdutil.ColumnarRowCells(item, cols, rc)...)
	}
	// Flush error is best-effort — output is going to stdout and we cannot
	// meaningfully recover from a write failure at this point.
	_ = tw.Flush()

	shown := len(result.Items)
	_, _ = fmt.Fprintf(w, "\n%d issues\n", shown)

	return nil
}

// NewCmd constructs the "list" command, which returns a filtered, ordered,
// paginated list of issues.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput  bool
		ready       bool
		all         bool
		order       string
		limit       int
		noLimit     bool
		columnsFlag string
	)

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List issues",
		Description: `Lists issues matching the given filters, ordered by issue ID (default),
priority, creation time, or modification time. By default, closed issues are
hidden and the result set is capped at a reasonable default.

This is the general-purpose query command. Use it to browse the backlog,
check the state of a specific parent's children (--parent), find issues
with a particular label (--label), or view only ready issues (--ready).
For the common case of "what can I work on next?", the dedicated "ready"
command is a convenient shortcut.

Filters combine with AND semantics: --role task --state open --label kind:bug
returns only open tasks labeled kind:bug. Pass --all to include resolved
issues in the output. Use --no-limit or --limit to control pagination,
and --json for machine-readable output.`,
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
				Usage:    "Filter by state: open, closed, deferred (repeatable)",
				Category: cmdutil.FlagCategorySupplemental,
			},
			&cli.BoolFlag{
				Name:        "ready",
				Usage:       "Show only issues in the ready secondary state",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &ready,
			},
			&cli.BoolFlag{
				Name:        "all",
				Aliases:     []string{"a"},
				Usage:       "Include all issues regardless of state, including closed",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &all,
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
				Usage:       "Sort order: " + cmdutil.ValidOrderNames() + "; append :asc or :desc for direction (default: ID)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &order,
			},
			&cli.StringFlag{
				Name:        "columns",
				Usage:       "Comma-separated list of columns to display; valid columns: " + cmdutil.ValidColumnNames(),
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &columnsFlag,
			},
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results",
				Value:       cmdutil.DefaultLimit,
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &limit,
			},
			&cli.BoolFlag{
				Name:        "no-limit",
				Usage:       "Return all matching results",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &noLimit,
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
			filter.ExcludeClosed = !all && len(filter.States) == 0

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

			// Default to ID ordering for list (unlike other commands which
			// default to priority). When the user explicitly passes --order,
			// respect their choice.
			effectiveOrder := order
			if effectiveOrder == "" {
				effectiveOrder = "id"
			}
			orderBy, direction, err := cmdutil.ParseOrderBy(effectiveOrder)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			effectiveLimit, err := cmdutil.ResolveLimit(limit, noLimit)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			cols, err := cmdutil.ParseColumns(columnsFlag)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			return Run(ctx, RunInput{
				Service:       svc,
				Filter:        filter,
				OrderBy:       orderBy,
				Direction:     direction,
				Limit:         effectiveLimit,
				JSON:          jsonOutput,
				Columns:       cols,
				WriteTo:       f.IOStreams.Out,
				TerminalWidth: f.IOStreams.TerminalWidth(),
				ColorScheme:   f.IOStreams.ColorScheme(),
			})
		},
	}
}

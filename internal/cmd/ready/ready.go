// Package ready provides the "ready" shortcut command — a quick way to list
// issues that are available for work.
package ready

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

// RunInput holds the parameters for the ready command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service driving.Service
	// Filter narrows the ready set. The Ready field is always forced to true
	// by Run regardless of what the caller sets, so callers may omit it.
	Filter        driving.IssueFilterInput
	OrderBy       driving.OrderBy
	Direction     driving.SortDirection
	JSON          bool
	Limit         int
	Columns       []cmdutil.Column
	WriteTo       io.Writer
	ColorScheme   *iostreams.ColorScheme
	TerminalWidth int
}

// Run executes the ready workflow: queries for ready issues ordered by priority
// and writes the result to the output writer. The Ready field of input.Filter
// is always forced to true — this command's purpose is the ready set.
func Run(ctx context.Context, input RunInput) error {
	filter := input.Filter
	filter.Ready = true

	result, err := input.Service.ListIssues(ctx, driving.ListIssuesInput{
		Filter:    filter,
		OrderBy:   input.OrderBy,
		Direction: input.Direction,
		Limit:     input.Limit,
	})
	if err != nil {
		return fmt.Errorf("listing ready issues: %w", err)
	}

	if input.JSON {
		out := cmdutil.ListOutput{
			HasMore: result.HasMore,
			Issues:  cmdutil.ConvertListItems(result.Items),
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	w := input.WriteTo
	cs := input.ColorScheme
	if cs == nil {
		cs = iostreams.NewColorScheme(false)
	}

	if len(result.Items) == 0 {
		_, _ = fmt.Fprintln(w, "No ready issues.")
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
	// Flush error is best-effort — output is going to stdout.
	_ = tw.Flush()

	shown := len(result.Items)
	if result.HasMore {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d ready (more available)", shown)))
	} else {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d ready", shown)))
	}

	return nil
}

// NewCmd constructs the "ready" command, which lists issues that are ready
// for work. The optional runFn parameter replaces the default Run for testing;
// when injected, the service is not constructed and the runFn receives the
// fully-resolved RunInput so flag parsing can be verified in isolation.
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, RunInput) error) *cli.Command {
	var (
		jsonOutput  bool
		order       string
		limit       int
		noLimit     bool
		columnsFlag string
	)

	return &cli.Command{
		Name:  "ready",
		Usage: "List the open issues with no blockers",
		Description: `Shows all issues that are currently available for work — open issues with
no unresolved blocked_by relationships and no ancestor epics that are
themselves blocked. The results are ordered by priority (P0 first).

This is the starting point for finding work. Agents should typically use
"claim ready" to atomically pick and lock the top issue, but "ready" is
useful when you want to browse the queue without committing to anything.
It is equivalent to "list --ready" but shorter to type and easier to
remember.

Filters combine with AND semantics: --role task --label kind:bug --parent FOO-abc12
returns only ready tasks labeled kind:bug whose parent is FOO-abc12.`,
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
				Usage:       "Sort order: " + cmdutil.ValidOrderNames() + "; append :asc or :desc for direction (default: PRIORITY)",
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
			// Ready is a flat listing; ResolveFlatListOrderBy remaps any
			// priority variant (bare, :asc, :desc, case/whitespace mixes, or
			// the empty default) to driving.OrderByPriorityCreated so parent
			// grouping does not affect ordering.
			orderBy, direction, err := cmdutil.ResolveFlatListOrderBy(order)
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

			// Build the filter from the new narrowing flags. Ready is
			// always forced to true by Run itself, so it is not set here.
			var filter driving.IssueFilterInput

			for _, r := range cmd.StringSlice("role") {
				role, roleErr := domain.ParseRole(r)
				if roleErr != nil {
					return cmdutil.FlagErrorf("invalid role %q: must be task or epic", r)
				}
				filter.Roles = append(filter.Roles, role)
			}

			for _, s := range cmd.StringSlice("state") {
				st, stErr := domain.ParseState(s)
				if stErr != nil {
					return cmdutil.FlagErrorf("invalid state %q: %v", s, stErr)
				}
				filter.States = append(filter.States, st)
			}

			rawLabels := cmd.StringSlice("label")
			labelFilters, labelErr := cmdutil.ParseLabelFilters(rawLabels)
			if labelErr != nil {
				return cmdutil.FlagErrorf("%s", labelErr)
			}
			filter.LabelFilters = labelFilters

			input := RunInput{
				Filter:        filter,
				OrderBy:       orderBy,
				Direction:     direction,
				JSON:          jsonOutput,
				Limit:         effectiveLimit,
				Columns:       cols,
				WriteTo:       f.IOStreams.Out,
				ColorScheme:   f.IOStreams.ColorScheme(),
				TerminalWidth: f.IOStreams.TerminalWidth(),
			}

			// When a test runFn is injected, skip service construction so
			// tests can verify flag parsing without a real database.
			if len(runFn) > 0 && runFn[0] != nil {
				return runFn[0](ctx, input)
			}

			svc, svcErr := cmdutil.NewTracker(f)
			if svcErr != nil {
				return svcErr
			}
			resolver := cmdutil.NewIDResolver(svc)

			rawParents := cmd.StringSlice("parent")
			for _, p := range rawParents {
				parentID, parentErr := resolver.Resolve(ctx, p)
				if parentErr != nil {
					return cmdutil.FlagErrorf("invalid parent ID: %s", parentErr)
				}
				filter.ParentIDs = append(filter.ParentIDs, parentID.String())
			}
			input.Filter = filter
			input.Service = svc

			return Run(ctx, input)
		},
	}
}

// Package blocked provides the "blocked" shortcut command — lists issues
// that have unresolved blocked_by relationships.
package blocked

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RunInput holds the parameters for the blocked command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	OrderBy       driving.OrderBy
	Direction     driving.SortDirection
	JSON          bool
	Limit         int
	Columns       []cmdutil.Column
	WriteTo       io.Writer
	ColorScheme   *iostreams.ColorScheme
	TerminalWidth int
}

// Run executes the blocked workflow: queries for blocked issues (excluding
// closed ones), ordered by priority, and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Service.ListIssues(ctx, driving.ListIssuesInput{
		Filter:    driving.IssueFilterInput{Blocked: true, ExcludeClosed: true},
		OrderBy:   input.OrderBy,
		Direction: input.Direction,
		Limit:     input.Limit,
	})
	if err != nil {
		return fmt.Errorf("listing blocked issues: %w", err)
	}

	if input.JSON {
		out := cmdutil.ListOutput{
			HasMore: result.HasMore,
			Items:   cmdutil.ConvertListItems(result.Items),
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	w := input.WriteTo
	cs := input.ColorScheme
	if cs == nil {
		cs = iostreams.NewColorScheme(false)
	}

	if len(result.Items) == 0 {
		_, _ = fmt.Fprintln(w, "No blocked issues.")
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
			cs.Dim(fmt.Sprintf("%d blocked (more available)", shown)))
	} else {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d blocked", shown)))
	}

	return nil
}

// NewCmd constructs the "blocked" command, which lists issues that have
// unresolved blocked_by relationships.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput  bool
		order       string
		limit       int
		noLimit     bool
		columnsFlag string
	)

	return &cli.Command{
		Name:  "blocked",
		Usage: "List open and deferred issues that are blocked",
		Description: `Shows all open and deferred issues that have at least one unresolved
blocked_by relationship. Each issue's title is annotated with the IDs of
its blockers so you can see at a glance what needs to be resolved first.

Use this to diagnose pipeline stalls: if "ready" returns nothing but there
is plenty of work in the tracker, "blocked" reveals which issues are stuck
and what is holding them up. Resolving or closing the blocking issues will
cause the blocked issues to transition to the ready state automatically.

Closed issues are excluded from the output. Results are ordered by
priority so that the highest-impact blockages appear first.`,
		Flags: []cli.Flag{
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
			// Blocked is a flat listing; ResolveFlatListOrderBy remaps any
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

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return Run(ctx, RunInput{
				Service:       svc,
				OrderBy:       orderBy,
				Direction:     direction,
				JSON:          jsonOutput,
				Limit:         effectiveLimit,
				Columns:       cols,
				WriteTo:       f.IOStreams.Out,
				ColorScheme:   f.IOStreams.ColorScheme(),
				TerminalWidth: f.IOStreams.TerminalWidth(),
			})
		},
	}
}

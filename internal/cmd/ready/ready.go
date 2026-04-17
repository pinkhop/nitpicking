// Package ready provides the "ready" shortcut command — a quick way to list
// issues that are available for work.
package ready

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RunInput holds the parameters for the ready command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
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

// Run executes the ready workflow: queries for ready issues ordered by priority
// and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Service.ListIssues(ctx, driving.ListIssuesInput{
		Filter:    driving.IssueFilterInput{Ready: true},
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
// for work. This is a shortcut for "np list --ready".
func NewCmd(f *cmdutil.Factory) *cli.Command {
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
remember.`,
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

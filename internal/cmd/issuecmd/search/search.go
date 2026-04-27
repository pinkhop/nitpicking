package search

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

// RunInput holds the parameters for the search command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	Query         string
	Filter        driving.IssueFilterInput
	OrderBy       driving.OrderBy
	Direction     driving.SortDirection
	IncludeNotes  bool
	Limit         int
	JSON          bool
	Columns       []cmdutil.Column
	WriteTo       io.Writer
	TerminalWidth int
	ColorScheme   *iostreams.ColorScheme
}

// Run executes the search workflow: validates the query, searches for issues,
// and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	if input.Query == "" {
		return fmt.Errorf("search query is required")
	}

	result, err := input.Service.SearchIssues(ctx, driving.SearchIssuesInput{
		Query:        input.Query,
		Filter:       input.Filter,
		OrderBy:      input.OrderBy,
		Direction:    input.Direction,
		IncludeNotes: input.IncludeNotes,
		Limit:        input.Limit,
	})
	if err != nil {
		return fmt.Errorf("searching issues: %w", err)
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

	cs := input.ColorScheme
	if cs == nil {
		cs = iostreams.NewColorScheme(false)
	}

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
	_, _ = fmt.Fprintf(w, "\n%d issues\n", shown)

	return nil
}

// NewCmd constructs the "search" command, which performs full-text search
// across issues.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput   bool
		state        string
		order        string
		includeNotes bool
		limit        int
		noLimit      bool
		columnsFlag  string
	)

	return &cli.Command{
		Name:      "search",
		Usage:     "Search issues by text query",
		ArgsUsage: "<QUERY>",
		Description: `Performs full-text search across issue titles, descriptions, and
acceptance criteria. Use this when you need to find issues by keyword or phrase
rather than browsing by state or priority — for example, to check whether an
issue already exists before creating a duplicate, or to locate all issues
related to a particular feature area.

The search can be narrowed with filters for role, state, and labels, and results
can be sorted by priority, creation time, or modification time. Use
--search-comments to extend the search to comment bodies. By default, results
are limited to a reasonable page size; use --no-limit or --limit to adjust.`,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:     "role",
				Aliases:  []string{"r"},
				Usage:    "Filter by role: task or epic (repeatable)",
				Category: cmdutil.FlagCategorySupplemental,
			},
			&cli.StringFlag{
				Name:        "state",
				Aliases:     []string{"s"},
				Usage:       "Filter by state",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &state,
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
			&cli.BoolFlag{
				Name:        "search-comments",
				Usage:       "Include comment bodies in the full-text search",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &includeNotes,
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
			query := cmd.Args().Get(0)
			if query == "" {
				return cmdutil.FlagErrorf("search query argument is required")
			}

			var filter driving.IssueFilterInput

			for _, r := range cmd.StringSlice("role") {
				role, err := domain.ParseRole(r)
				if err != nil {
					return cmdutil.FlagErrorf("invalid role %q: must be task or epic", r)
				}
				filter.Roles = append(filter.Roles, role)
			}

			if state != "" {
				st, err := domain.ParseState(state)
				if err != nil {
					return cmdutil.FlagErrorf("invalid state %q: %v", state, err)
				}
				filter.States = []domain.State{st}
			}

			// Parse label filters.
			rawLabels := cmd.StringSlice("label")
			labelFilters, parseErr := cmdutil.ParseLabelFilters(rawLabels)
			if parseErr != nil {
				return cmdutil.FlagErrorf("%s", parseErr)
			}
			filter.LabelFilters = labelFilters

			orderBy, direction, err := cmdutil.ParseOrderBy(order)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
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
				Query:         query,
				Filter:        filter,
				OrderBy:       orderBy,
				Direction:     direction,
				IncludeNotes:  includeNotes,
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

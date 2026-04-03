package search

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

// RunInput holds the parameters for the search command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	Query         string
	Filter        driving.IssueFilterInput
	OrderBy       driving.OrderBy
	IncludeNotes  bool
	Limit         int
	JSON          bool
	Timestamps    bool
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
		IncludeNotes: input.IncludeNotes,
		Limit:        input.Limit,
	})
	if err != nil {
		return fmt.Errorf("searching issues: %w", err)
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

	if len(result.Items) == 0 {
		_, _ = fmt.Fprintln(w, "No issues found.")
		return nil
	}

	// Estimate non-title column overhead for title truncation.
	overhead := 29
	if input.Timestamps {
		overhead = 50
	}
	maxTitle := cmdutil.AvailableTitleWidth(input.TerminalWidth, overhead)

	cs := input.ColorScheme
	if cs == nil {
		cs = iostreams.NewColorScheme(false)
	}

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

// NewCmd constructs the "search" command, which performs full-text search
// across issues.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput   bool
		state        string
		order        string
		includeNotes bool
		limit        int
		all          bool
		timestamps   bool
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
are limited to a reasonable page size; use --all or --limit to adjust.`,
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
				Usage:       "Sort order: priority, created, modified (default: priority)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &order,
			},
			&cli.BoolFlag{
				Name:        "search-comments",
				Usage:       "Include comment bodies in the full-text search",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &includeNotes,
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

			orderBy, err := cmdutil.ParseOrderBy(order)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			effectiveLimit := cmdutil.ResolveLimit(limit, all, f.IOStreams.IsStdoutTTY())

			return Run(ctx, RunInput{
				Service:       svc,
				Query:         query,
				Filter:        filter,
				OrderBy:       orderBy,
				IncludeNotes:  includeNotes,
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

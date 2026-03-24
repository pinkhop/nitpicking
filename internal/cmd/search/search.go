package search

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// searchItemOutput is the JSON representation of a single search result item.
type searchItemOutput struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	State     string `json:"state"`
	Priority  string `json:"priority"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// searchOutput is the JSON representation of the search command result.
type searchOutput struct {
	Items   []searchItemOutput `json:"items"`
	HasMore bool               `json:"has_more"`
}

// NewCmd constructs the "search" command, which performs full-text search
// across issues.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput   bool
		role         string
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
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "role",
				Aliases:     []string{"r"},
				Usage:       "Filter by role: task or epic",
				Category:    "Options",
				Destination: &role,
			},
			&cli.StringFlag{
				Name:        "state",
				Aliases:     []string{"s"},
				Usage:       "Filter by state",
				Category:    "Options",
				Destination: &state,
			},
			&cli.StringSliceFlag{
				Name:     "dimension",
				Usage:    "Dimension filter in key:value format (repeatable)",
				Category: "Options",
			},
			&cli.StringFlag{
				Name:        "order",
				Usage:       "Sort order: priority, created, modified (default: priority)",
				Category:    "Options",
				Destination: &order,
			},
			&cli.BoolFlag{
				Name:        "search-comments",
				Usage:       "Include comment bodies in the full-text search",
				Category:    "Options",
				Destination: &includeNotes,
			},
			&cli.BoolFlag{
				Name:        "timestamps",
				Usage:       "Include created_at timestamp in text output",
				Category:    "Options",
				Destination: &timestamps,
			},
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results (0 = default, negative = unlimited)",
				Category:    "Options",
				Destination: &limit,
			},
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "Return all results without limit",
				Category:    "Options",
				Destination: &all,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			query := cmd.Args().Get(0)
			if query == "" {
				return cmdutil.FlagErrorf("search query argument is required")
			}

			var filter port.IssueFilter

			if role != "" {
				parsedRole, err := issue.ParseRole(role)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
				filter.Role = parsedRole
			}

			if state != "" {
				parsedState, err := issue.ParseState(state)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
				filter.States = []issue.State{parsedState}
			}

			// Parse dimension filters.
			rawDimensions := cmd.StringSlice("dimension")
			for _, s := range rawDimensions {
				key, value, ok := strings.Cut(s, ":")
				if !ok {
					return cmdutil.FlagErrorf("invalid dimension filter %q: must be in key:value format", s)
				}
				ff := port.DimensionFilter{Key: key}
				if value != "*" {
					ff.Value = value
				}
				filter.DimensionFilters = append(filter.DimensionFilters, ff)
			}

			orderBy, err := parseOrderBy(order)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			effectiveLimit := cmdutil.ResolveLimit(limit, all, f.IOStreams.IsStdoutTTY())
			input := service.SearchIssuesInput{
				Query:        query,
				Filter:       filter,
				OrderBy:      orderBy,
				IncludeNotes: includeNotes,
				Limit:        effectiveLimit,
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			result, err := svc.SearchIssues(ctx, input)
			if err != nil {
				return fmt.Errorf("searching issues: %w", err)
			}

			if jsonOutput {
				out := searchOutput{
					HasMore: result.HasMore,
					Items:   make([]searchItemOutput, 0, len(result.Items)),
				}
				for _, item := range result.Items {
					out.Items = append(out.Items, searchItemOutput{
						ID:        item.ID.String(),
						Role:      item.Role.String(),
						State:     item.State.String(),
						Priority:  item.Priority.String(),
						Title:     item.Title,
						CreatedAt: item.CreatedAt.Format(time.RFC3339),
						UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
					})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			// Human-readable output.
			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "No issues found.")
				return nil
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, item := range result.Items {
				if timestamps {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
						cs.Bold(item.ID.String()),
						cs.Dim(item.Role.String()),
						item.State.String(),
						cs.Yellow(item.Priority.String()),
						cs.Dim(item.CreatedAt.Format(time.DateTime)),
						item.Title)
				} else {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						cs.Bold(item.ID.String()),
						cs.Dim(item.Role.String()),
						item.State.String(),
						cs.Yellow(item.Priority.String()),
						item.Title)
				}
			}
			_ = tw.Flush()

			shown := len(result.Items)
			_, _ = fmt.Fprintf(w, "\n%s\n",
				cs.Dim(fmt.Sprintf("%d issues", shown)))
			if result.HasMore {
				_, _ = fmt.Fprintf(f.IOStreams.ErrOut,
					"Showing %d issues (use --all for all results)\n", shown)
			}

			return nil
		},
	}
}

// parseOrderBy converts a user-provided sort order string into a
// port.IssueOrderBy constant.
func parseOrderBy(s string) (port.IssueOrderBy, error) {
	switch strings.ToLower(s) {
	case "", "priority":
		return port.OrderByPriority, nil
	case "created":
		return port.OrderByCreatedAt, nil
	case "modified":
		return port.OrderByUpdatedAt, nil
	default:
		return 0, fmt.Errorf("invalid sort order %q: must be priority, created, or modified", s)
	}
}

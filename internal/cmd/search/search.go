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
	Items      []searchItemOutput `json:"items"`
	TotalCount int                `json:"total_count"`
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
		pageSize     int
		timestamps   bool
	)

	return &cli.Command{
		Name:      "search",
		Usage:     "Search issues by text query",
		ArgsUsage: "<QUERY>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "role",
				Aliases:     []string{"r"},
				Usage:       "Filter by role: task or epic",
				Destination: &role,
			},
			&cli.StringFlag{
				Name:        "state",
				Aliases:     []string{"s"},
				Usage:       "Filter by state",
				Destination: &state,
			},
			&cli.StringSliceFlag{
				Name:  "facet",
				Usage: "Facet filter in key:value format (repeatable)",
			},
			&cli.StringFlag{
				Name:        "order",
				Usage:       "Sort order: priority, created, modified (default: priority)",
				Destination: &order,
			},
			&cli.BoolFlag{
				Name:        "search-notes",
				Usage:       "Include note bodies in the full-text search",
				Destination: &includeNotes,
			},
			&cli.BoolFlag{
				Name:        "timestamps",
				Usage:       "Include created_at timestamp in text output",
				Destination: &timestamps,
			},
			&cli.IntFlag{
				Name:        "page-size",
				Usage:       "Number of results per page",
				Value:       20,
				Destination: &pageSize,
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

			// Parse facet filters.
			rawFacets := cmd.StringSlice("facet")
			for _, s := range rawFacets {
				key, value, ok := strings.Cut(s, ":")
				if !ok {
					return cmdutil.FlagErrorf("invalid facet filter %q: must be in key:value format", s)
				}
				ff := port.FacetFilter{Key: key}
				if value != "*" {
					ff.Value = value
				}
				filter.FacetFilters = append(filter.FacetFilters, ff)
			}

			orderBy, err := parseOrderBy(order)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			input := service.SearchIssuesInput{
				Query:        query,
				Filter:       filter,
				OrderBy:      orderBy,
				IncludeNotes: includeNotes,
				Page:         port.PageRequest{PageSize: pageSize},
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
					TotalCount: result.TotalCount,
					Items:      make([]searchItemOutput, 0, len(result.Items)),
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
			if shown < result.TotalCount {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("Showing %d of %d issues", shown, result.TotalCount)))
			} else {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d issues", result.TotalCount)))
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

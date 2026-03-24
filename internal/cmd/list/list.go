package list

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

// listItemOutput is the JSON representation of a single issue in a list.
type listItemOutput struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	State     string `json:"state"`
	Priority  string `json:"priority"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// listOutput is the JSON representation of the list command result.
type listOutput struct {
	Items      []listItemOutput `json:"items"`
	TotalCount int              `json:"total_count"`
}

// NewCmd constructs the "list" command, which returns a filtered, ordered,
// paginated list of issues.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput    bool
		role          string
		state         string
		ready         bool
		includeClosed bool
		parent        string
		descendantsOf string
		ancestorsOf   string
		order         string
		pageSize      int
		timestamps    bool
	)

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List issues",
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
				Usage:       "Filter by state: open, active, claimed, closed, deferred, waiting",
				Destination: &state,
			},
			&cli.BoolFlag{
				Name:        "ready",
				Usage:       "Show only ready issues",
				Destination: &ready,
			},
			&cli.BoolFlag{
				Name:        "include-closed",
				Usage:       "Include closed issues in the output (hidden by default)",
				Destination: &includeClosed,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "Filter by parent epic ID",
				Destination: &parent,
			},
			&cli.StringFlag{
				Name:        "descendants-of",
				Usage:       "Recursively list all descendants of the given issue ID",
				Destination: &descendantsOf,
			},
			&cli.StringFlag{
				Name:        "ancestors-of",
				Usage:       "List the parent chain of the given issue ID up to the root",
				Destination: &ancestorsOf,
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
			var filter port.IssueFilter
			filter.Ready = ready
			filter.ExcludeClosed = !includeClosed && state == ""

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

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			if parent != "" {
				parentID, err := resolver.Resolve(ctx, parent)
				if err != nil {
					return cmdutil.FlagErrorf("invalid parent ID: %s", err)
				}
				filter.ParentID = parentID
			}

			if descendantsOf != "" {
				descID, err := resolver.Resolve(ctx, descendantsOf)
				if err != nil {
					return cmdutil.FlagErrorf("invalid descendants-of ID: %s", err)
				}
				filter.DescendantsOf = descID
			}

			if ancestorsOf != "" {
				ancID, err := resolver.Resolve(ctx, ancestorsOf)
				if err != nil {
					return cmdutil.FlagErrorf("invalid ancestors-of ID: %s", err)
				}
				filter.AncestorsOf = ancID
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

			input := service.ListIssuesInput{
				Filter:  filter,
				OrderBy: orderBy,
				Page:    port.PageRequest{PageSize: pageSize},
			}
			result, err := svc.ListIssues(ctx, input)
			if err != nil {
				return fmt.Errorf("listing issues: %w", err)
			}

			if jsonOutput {
				out := listOutput{
					TotalCount: result.TotalCount,
					Items:      make([]listItemOutput, 0, len(result.Items)),
				}
				for _, item := range result.Items {
					out.Items = append(out.Items, listItemOutput{
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
// port.IssueOrderBy constant. An empty string defaults to priority ordering.
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

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
	ID            string `json:"id"`
	Role          string `json:"role"`
	State         string `json:"state"`
	DisplayStatus string `json:"display_status"`
	Priority      string `json:"priority"`
	Title         string `json:"title"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// listOutput is the JSON representation of the list command result.
type listOutput struct {
	Items   []listItemOutput `json:"items"`
	HasMore bool             `json:"has_more"`
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
		limit         int
		all           bool
		timestamps    bool
	)

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List issues",
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
				Usage:       "Filter by state: open, claimed, closed, deferred",
				Category:    "Options",
				Destination: &state,
			},
			&cli.BoolFlag{
				Name:        "ready",
				Usage:       "Show only ready issues",
				Category:    "Options",
				Destination: &ready,
			},
			&cli.BoolFlag{
				Name:        "include-closed",
				Usage:       "Include closed issues in the output (hidden by default)",
				Category:    "Options",
				Destination: &includeClosed,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "Filter by parent epic ID",
				Category:    "Options",
				Destination: &parent,
			},
			&cli.StringFlag{
				Name:        "descendants-of",
				Usage:       "Recursively list all descendants of the given issue ID",
				Category:    "Options",
				Destination: &descendantsOf,
			},
			&cli.StringFlag{
				Name:        "ancestors-of",
				Usage:       "List the parent chain of the given issue ID up to the root",
				Category:    "Options",
				Destination: &ancestorsOf,
			},
			&cli.StringSliceFlag{
				Name:     "label",
				Aliases:  []string{"dimension"},
				Usage:    "Label filter in key:value format (repeatable)",
				Category: "Options",
			},
			&cli.StringFlag{
				Name:        "order",
				Usage:       "Sort order: priority, created, modified (default: priority)",
				Category:    "Options",
				Destination: &order,
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

			// Parse label filters.
			rawLabels := cmd.StringSlice("label")
			for _, s := range rawLabels {
				key, value, ok := strings.Cut(s, ":")
				if !ok {
					return cmdutil.FlagErrorf("invalid label filter %q: must be in key:value format", s)
				}
				ff := port.LabelFilter{Key: key}
				if value != "*" {
					ff.Value = value
				}
				filter.LabelFilters = append(filter.LabelFilters, ff)
			}

			orderBy, err := parseOrderBy(order)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			effectiveLimit := cmdutil.ResolveLimit(limit, all, f.IOStreams.IsStdoutTTY())
			input := service.ListIssuesInput{
				Filter:  filter,
				OrderBy: orderBy,
				Limit:   effectiveLimit,
			}
			result, err := svc.ListIssues(ctx, input)
			if err != nil {
				return fmt.Errorf("listing issues: %w", err)
			}

			if jsonOutput {
				out := listOutput{
					HasMore: result.HasMore,
					Items:   make([]listItemOutput, 0, len(result.Items)),
				}
				for _, item := range result.Items {
					out.Items = append(out.Items, listItemOutput{
						ID:            item.ID.String(),
						Role:          item.Role.String(),
						State:         item.State.String(),
						DisplayStatus: item.DisplayStatus(),
						Priority:      item.Priority.String(),
						Title:         item.Title,
						CreatedAt:     item.CreatedAt.Format(time.RFC3339),
						UpdatedAt:     item.UpdatedAt.Format(time.RFC3339),
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
						item.DisplayStatus(),
						cs.Yellow(item.Priority.String()),
						cs.Dim(item.CreatedAt.Format(time.DateTime)),
						item.Title)
				} else {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						cs.Bold(item.ID.String()),
						cs.Dim(item.Role.String()),
						item.DisplayStatus(),
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

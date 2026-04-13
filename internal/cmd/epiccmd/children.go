package epiccmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// childOutput is the JSON representation of a single child item.
type childOutput struct {
	ID            string `json:"id"`
	Role          string `json:"role"`
	State         string `json:"state"`
	DisplayStatus string `json:"display_status"`
	Priority      string `json:"priority"`
	Title         string `json:"title"`
	CreatedAt     string `json:"created_at"`
}

// childrenOutput is the JSON representation of the children list.
type childrenOutput struct {
	Items   []childOutput `json:"items"`
	HasMore bool          `json:"has_more"`
}

// newChildrenCmd constructs "epic children" which lists all children of an
// epic, including closed issues.
func newChildrenCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		limit      int
		noLimit    bool
	)

	return &cli.Command{
		Name:      "children",
		Usage:     "List all children of an epic",
		ArgsUsage: "<EPIC-ID>",
		Description: `Lists all child issues of an epic, including closed children. Unlike
"np list" which excludes closed issues by default, this command shows the
complete picture of an epic's decomposition regardless of child state.

Use this to inspect the full scope of an epic — for example, to review which
tasks remain open, to verify that all children have been closed before running
"epic close-completed", or to understand the decomposition of an epic you are
about to claim. Results are sorted by priority.`,
		Flags: []cli.Flag{
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
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("epic ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			epicID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid epic ID: %s", err)
			}

			effectiveLimit, err := cmdutil.ResolveLimit(limit, noLimit)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Include closed children (unlike the default list behavior).
			result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
				Filter:  driving.IssueFilterInput{ParentIDs: []string{epicID.String()}},
				OrderBy: driving.OrderByPriority,
				Limit:   effectiveLimit,
			})
			if err != nil {
				return fmt.Errorf("listing children: %w", err)
			}

			if jsonOutput {
				out := childrenOutput{
					HasMore: result.HasMore,
					Items:   make([]childOutput, 0, len(result.Items)),
				}
				for _, item := range result.Items {
					out.Items = append(out.Items, childOutput{
						ID:            item.ID,
						Role:          item.Role.String(),
						State:         item.State.String(),
						DisplayStatus: item.DisplayStatus,
						Priority:      item.Priority.String(),
						Title:         item.Title,
						CreatedAt:     cmdutil.FormatJSONTimestamp(item.CreatedAt),
					})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "No children found.")
				return nil
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, item := range result.Items {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					cs.Bold(item.ID),
					cs.Dim(item.Role.String()),
					cmdutil.FormatState(cs, item.State, item.SecondaryState),
					cs.Yellow(item.Priority.String()),
					item.Title)
			}
			_ = tw.Flush()

			shown := len(result.Items)
			_, _ = fmt.Fprintf(w, "\n%s\n",
				cs.Dim(fmt.Sprintf("%d children", shown)))
			if result.HasMore {
				_, _ = fmt.Fprintf(f.IOStreams.ErrOut,
					"Showing %d children (use --no-limit for all)\n", shown)
			}

			return nil
		},
	}
}

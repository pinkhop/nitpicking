package epiccmd

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/port"
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
	)

	return &cli.Command{
		Name:      "children",
		Usage:     "List all children of an epic",
		ArgsUsage: "<EPIC-ID>",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results (0 = default, negative = unlimited)",
				Category:    "Options",
				Destination: &limit,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
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

			// Include closed children (unlike the default list behavior).
			result, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{ParentID: epicID},
				OrderBy: port.OrderByPriority,
				Limit:   limit,
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
						ID:            item.ID.String(),
						Role:          item.Role.String(),
						State:         item.State.String(),
						DisplayStatus: item.DisplayStatus(),
						Priority:      item.Priority.String(),
						Title:         item.Title,
						CreatedAt:     item.CreatedAt.Format(time.RFC3339),
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
					cs.Bold(item.ID.String()),
					cs.Dim(item.Role.String()),
					item.DisplayStatus(),
					cs.Yellow(item.Priority.String()),
					item.Title)
			}
			_ = tw.Flush()

			shown := len(result.Items)
			_, _ = fmt.Fprintf(w, "\n%s\n",
				cs.Dim(fmt.Sprintf("%d children", shown)))
			if result.HasMore {
				_, _ = fmt.Fprintf(f.IOStreams.ErrOut,
					"Showing %d children (use --limit -1 for all)\n", shown)
			}

			return nil
		},
	}
}

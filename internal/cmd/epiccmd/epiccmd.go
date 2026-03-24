// Package epiccmd provides the "epic" parent command with subcommands for
// epic-specific operations such as completion status tracking.
package epiccmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// Progress holds the computed completion state of an epic.
type Progress struct {
	Total    int
	Closed   int
	Percent  int
	Eligible bool
}

// ComputeProgress derives completion metrics from a list of child statuses.
// An epic is eligible for closure when it has at least one child and all
// children are closed.
func ComputeProgress(children []issue.ChildStatus) Progress {
	total := len(children)
	if total == 0 {
		return Progress{}
	}

	closed := 0
	for _, c := range children {
		if c.State == issue.StateClosed {
			closed++
		}
	}

	return Progress{
		Total:    total,
		Closed:   closed,
		Percent:  closed * 100 / total,
		Eligible: closed == total,
	}
}

// epicStatusItem holds the data for a single epic's status display.
type epicStatusItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Total    int    `json:"total_children"`
	Closed   int    `json:"closed_children"`
	Percent  int    `json:"percent"`
	Eligible bool   `json:"eligible_for_closure"`
}

// NewCmd constructs the "epic" parent command.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "epic",
		Usage: "Epic management commands",
		Commands: []*cli.Command{
			newStatusCmd(f),
			newCloseEligibleCmd(f),
			newChildrenCmd(f),
		},
	}
}

// newStatusCmd constructs "epic status" which shows completion breakdown for
// open epics.
func newStatusCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput   bool
		eligibleOnly bool
	)

	return &cli.Command{
		Name:      "status",
		Usage:     "Show completion status for open epics",
		ArgsUsage: "[EPIC-ID]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "eligible-only",
				Usage:       "Show only epics eligible for closure",
				Category:    "Options",
				Destination: &eligibleOnly,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			var epics []port.IssueListItem

			issueArg := cmd.Args().Get(0)
			if issueArg != "" {
				// Single epic mode.
				resolver := cmdutil.NewIDResolver(svc)
				epicID, resolveErr := resolver.Resolve(ctx, issueArg)
				if resolveErr != nil {
					return cmdutil.FlagErrorf("invalid issue ID: %s", resolveErr)
				}
				shown, showErr := svc.ShowIssue(ctx, epicID)
				if showErr != nil {
					return fmt.Errorf("looking up epic: %w", showErr)
				}
				if !shown.Issue.IsEpic() {
					return fmt.Errorf("issue %s is not an epic", epicID)
				}
				epics = []port.IssueListItem{{
					ID:       shown.Issue.ID(),
					Role:     shown.Issue.Role(),
					State:    shown.Issue.State(),
					Priority: shown.Issue.Priority(),
					Title:    shown.Issue.Title(),
				}}
			} else {
				// List all open epics.
				result, listErr := svc.ListIssues(ctx, service.ListIssuesInput{
					Filter:  port.IssueFilter{Role: issue.RoleEpic, ExcludeClosed: true},
					OrderBy: port.OrderByPriority,
					Limit:   -1,
				})
				if listErr != nil {
					return fmt.Errorf("listing epics: %w", listErr)
				}
				epics = result.Items
			}

			// Compute progress for each epic.
			var items []epicStatusItem
			for _, epic := range epics {
				// List children to compute completion status.
				childResult, childListErr := svc.ListIssues(ctx, service.ListIssuesInput{
					Filter: port.IssueFilter{ParentID: epic.ID},
					Limit:  -1,
				})
				if childListErr != nil {
					continue
				}

				// Convert to ChildStatus for progress computation.
				var childStatuses []issue.ChildStatus
				for _, child := range childResult.Items {
					childStatuses = append(childStatuses, issue.ChildStatus{State: child.State})
				}

				prog := ComputeProgress(childStatuses)

				if eligibleOnly && !prog.Eligible {
					continue
				}

				// An epic is only eligible for closure if it's still open.
				// Closed epics have already been resolved.
				eligible := prog.Eligible && epic.State != issue.StateClosed

				items = append(items, epicStatusItem{
					ID:       epic.ID.String(),
					Title:    epic.Title,
					Total:    prog.Total,
					Closed:   prog.Closed,
					Percent:  prog.Percent,
					Eligible: eligible,
				})
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"epics": items,
					"count": len(items),
				})
			}

			w := f.IOStreams.Out
			cs := f.IOStreams.ColorScheme()

			if len(items) == 0 {
				_, _ = fmt.Fprintln(w, "No epics found.")
				return nil
			}

			for _, item := range items {
				icon := statusIcon(cs, item)
				eligibleLabel := ""
				if item.Eligible {
					eligibleLabel = cs.Green(" — Eligible for closure")
				}
				_, _ = fmt.Fprintf(w, "%s %s %s%s\n",
					icon,
					cs.Bold(item.ID),
					item.Title,
					eligibleLabel)
				_, _ = fmt.Fprintf(w, "  %d/%d children closed (%d%%)\n",
					item.Closed, item.Total, item.Percent)
			}

			return nil
		},
	}
}

// statusIcon returns a colored icon based on progress.
func statusIcon(cs *iostreams.ColorScheme, item epicStatusItem) string {
	if item.Eligible {
		return cs.Green("✓")
	}
	if item.Closed > 0 {
		return cs.Yellow("●")
	}
	return "○"
}

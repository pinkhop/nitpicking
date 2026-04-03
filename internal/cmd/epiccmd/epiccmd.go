// Package epiccmd provides the "epic" parent command with subcommands for
// epic-specific operations such as completion status tracking.
package epiccmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// epicStatusItem holds the data for a single epic's status display.
type epicStatusItem struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	SecondaryState string `json:"secondary_state,omitempty"`
	Total          int    `json:"total_children"`
	Closed         int    `json:"closed_children"`
	Percent        int    `json:"percent"`
	Completed      bool   `json:"completed"`

	// secondaryVal retains the typed secondary state for coloring. Not
	// serialized — the string version above handles JSON.
	secondaryVal domain.SecondaryState
}

// NewCmd constructs the "epic" parent command.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "epic",
		Usage: "Epic management commands",
		Description: `Groups commands for managing epics — the organizational layer above
tasks. Epics are never closed directly; they complete automatically when all
their children are closed, at which point they enter the "completed" secondary
state and can be batch-closed with "epic close-completed".

Use these subcommands to inspect epic progress, view an epic's children, and
close completed epics. For creating epics, use "np create" with role "epic". For
attaching children to an epic, set the parent field when creating the child
issue.`,
		Commands: []*cli.Command{
			newStatusCmd(f),
			newCloseCompletedCmd(f),
			newChildrenCmd(f),
		},
	}
}

// newStatusCmd constructs "epic status" which shows completion breakdown for
// open epics. Delegates to the service's EpicProgress method for efficient
// child status lookups.
func newStatusCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput    bool
		completedOnly bool
	)

	return &cli.Command{
		Name:      "status",
		Usage:     "Show secondary state and completion status for open epics",
		ArgsUsage: "[EPIC-ID]",
		Description: `Shows the completion progress of open epics, including child counts,
percentage complete, secondary state (active, completed), and whether the epic
is eligible for closing. When called without arguments, displays all open epics.
When given an epic ID, shows the status of that single epic.

Use this to monitor progress across the project — for example, to identify which
epics are close to completion, or to find epics that have stalled. The
--completed-only flag narrows the output to epics that are fully complete and
ready to be closed with "epic close-completed".`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "completed-only",
				Usage:       "Show only completed epics",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &completedOnly,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			var input driving.EpicProgressInput

			issueArg := cmd.Args().Get(0)
			if issueArg != "" {
				resolver := cmdutil.NewIDResolver(svc)
				epicID, resolveErr := resolver.Resolve(ctx, issueArg)
				if resolveErr != nil {
					return cmdutil.FlagErrorf("invalid issue ID: %s", resolveErr)
				}
				input.EpicID = epicID.String()
			}

			progressOut, err := svc.EpicProgress(ctx, input)
			if err != nil {
				return err
			}

			var items []epicStatusItem
			for _, epic := range progressOut.Items {
				if completedOnly && !epic.Completed {
					continue
				}

				// An epic is only completed if it's still open.
				// Closed epics have already been resolved.
				completed := epic.Completed && epic.State != domain.StateClosed

				items = append(items, epicStatusItem{
					ID:             epic.ID,
					Title:          epic.Title,
					SecondaryState: epic.SecondaryState.String(),
					Total:          epic.Total,
					Closed:         epic.Closed,
					Percent:        epic.Percent,
					Completed:      completed,
					secondaryVal:   epic.SecondaryState,
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
				stateLabel := ""
				if item.SecondaryState != "" {
					stateLabel = " " + cmdutil.ColorSecondaryText(cs, item.secondaryVal, "["+item.SecondaryState+"]")
				}
				completedLabel := ""
				if item.Completed {
					completedLabel = cmdutil.ColorStateText(cs, domain.StateClosed, " — Completed")
				}
				_, _ = fmt.Fprintf(w, "%s %s %s%s%s\n",
					icon,
					cs.Bold(item.ID),
					item.Title,
					stateLabel,
					completedLabel)
				_, _ = fmt.Fprintf(w, "  %d/%d children closed (%d%%)\n",
					item.Closed, item.Total, item.Percent)
			}

			return nil
		},
	}
}

// statusIcon returns a colored icon based on progress.
func statusIcon(cs *iostreams.ColorScheme, item epicStatusItem) string {
	if item.Completed {
		return cmdutil.ColorStateText(cs, domain.StateClosed, "✓")
	}
	if item.Closed > 0 {
		return cmdutil.ColorStateText(cs, domain.StateClaimed, "●")
	}
	return "○"
}

// Package blocked provides the "blocked" shortcut command — lists issues
// that have unresolved blocked_by relationships.
package blocked

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// NewCmd constructs the "blocked" command, which lists issues that have
// unresolved blocked_by relationships.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "blocked",
		Usage: "List issues blocked by unresolved dependencies",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			result, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{Blocked: true, ExcludeClosed: true},
				OrderBy: port.OrderByPriority,
			})
			if err != nil {
				return fmt.Errorf("listing blocked issues: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, result)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "No blocked issues.")
				return nil
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, item := range result.Items {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					cs.Bold(item.ID.String()),
					cs.Dim(item.Role.String()),
					item.State.String(),
					cs.Yellow(item.Priority.String()),
					item.Title)
			}
			_ = tw.Flush()

			_, _ = fmt.Fprintf(w, "\n%s\n",
				cs.Dim(fmt.Sprintf("%d blocked", result.TotalCount)))

			return nil
		},
	}
}

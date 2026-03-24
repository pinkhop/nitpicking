// Package status provides the "status" shortcut command — a dashboard
// showing summary statistics about the issue database.
package status

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// statusOutput is the JSON representation of the status dashboard.
type statusOutput struct {
	Open     int `json:"open"`
	Claimed  int `json:"claimed"`
	Deferred int `json:"deferred"`
	Closed   int `json:"closed"`
	Ready    int `json:"ready"`
	Blocked  int `json:"blocked"`
	Total    int `json:"total"`
}

// NewCmd constructs the "status" command, which displays a summary dashboard
// of issue counts by state.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "status",
		Usage: "Show issue database summary",
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

			// Count by state.
			states := []issue.State{
				issue.StateOpen,
				issue.StateClaimed,
				issue.StateDeferred,
				issue.StateClosed,
			}

			out := statusOutput{}
			for _, s := range states {
				result, err := svc.ListIssues(ctx, service.ListIssuesInput{
					Filter: port.IssueFilter{States: []issue.State{s}},
					Limit:  -1,
				})
				if err != nil {
					return fmt.Errorf("counting %s issues: %w", s, err)
				}
				switch s {
				case issue.StateOpen:
					out.Open = len(result.Items)
				case issue.StateClaimed:
					out.Claimed = len(result.Items)
				case issue.StateDeferred:
					out.Deferred = len(result.Items)
				case issue.StateClosed:
					out.Closed = len(result.Items)
				}
			}
			out.Total = out.Open + out.Claimed + out.Deferred + out.Closed

			// Count ready.
			readyResult, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter: port.IssueFilter{Ready: true},
				Limit:  -1,
			})
			if err != nil {
				return fmt.Errorf("counting ready issues: %w", err)
			}
			out.Ready = len(readyResult.Items)

			// Count blocked.
			blockedResult, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter: port.IssueFilter{Blocked: true, ExcludeClosed: true},
				Limit:  -1,
			})
			if err != nil {
				return fmt.Errorf("counting blocked issues: %w", err)
			}
			out.Blocked = len(blockedResult.Items)

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

			_, _ = fmt.Fprintf(tw, "Open\t%s\n", cs.Bold(fmt.Sprintf("%d", out.Open)))
			_, _ = fmt.Fprintf(tw, "Claimed\t%s\n", cs.Bold(fmt.Sprintf("%d", out.Claimed)))
			_, _ = fmt.Fprintf(tw, "Deferred\t%s\n", cs.Bold(fmt.Sprintf("%d", out.Deferred)))
			_, _ = fmt.Fprintf(tw, "Closed\t%s\n", cs.Bold(fmt.Sprintf("%d", out.Closed)))
			_, _ = fmt.Fprintf(tw, "Ready\t%s\n", cs.Green(fmt.Sprintf("%d", out.Ready)))
			_, _ = fmt.Fprintf(tw, "Blocked\t%s\n", cs.Red(fmt.Sprintf("%d", out.Blocked)))
			_ = tw.Flush()
			_, _ = fmt.Fprintf(w, "\n%s\n", cs.Dim(fmt.Sprintf("%d total", out.Total)))

			return nil
		},
	}
}

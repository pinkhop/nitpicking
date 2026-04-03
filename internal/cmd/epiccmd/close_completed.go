package epiccmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// closeResult records the outcome of attempting to close a single epic.
type closeResult struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Closed  bool   `json:"closed"`
	Message string `json:"message,omitzero"`
}

// newCloseCompletedCmd constructs "epic close-completed" which finds epics
// where all children are closed and batch-closes them. Delegates to the
// service's CloseCompletedEpics method for the full workflow.
func newCloseCompletedCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput   bool
		author       string
		dryRun       bool
		includeTasks bool
	)

	return &cli.Command{
		Name:  "close-completed",
		Usage: "Close all epics in the completed secondary state",
		Description: `Finds all epics whose children are entirely closed (the "completed"
secondary state) and batch-closes them. This is the standard way to finalize
epics — since epics cannot be closed directly, this command handles the
claim-close-release cycle for each eligible epic automatically.

Run this after closing the last child task of an epic to keep the issue database
tidy. Use --dry-run to preview which epics would be closed without actually
closing them. The --include-tasks flag extends the operation to parent tasks
(not just epics) whose children are all closed.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (for claiming and commenting) (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &author,
			},
			&cli.BoolFlag{
				Name:        "include-tasks",
				Usage:       "Also close parent tasks whose children are all closed",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &includeTasks,
			},
			&cli.BoolFlag{
				Name:        "dry-run",
				Usage:       "List completed epics without closing them",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &dryRun,
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

			out, err := svc.CloseCompletedEpics(ctx, driving.CloseCompletedEpicsInput{
				Author:       author,
				DryRun:       dryRun,
				IncludeTasks: includeTasks,
			})
			if err != nil {
				return err
			}

			if len(out.Results) == 0 {
				if jsonOutput {
					return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
						"results": []closeResult{},
						"closed":  0,
					})
				}
				_, _ = fmt.Fprintln(f.IOStreams.Out, "No completed epics found.")
				return nil
			}

			// Convert service results to CLI display format.
			results := make([]closeResult, 0, len(out.Results))
			for _, r := range out.Results {
				results = append(results, closeResult{
					ID:      r.ID,
					Title:   r.Title,
					Closed:  r.Closed,
					Message: r.Message,
				})
			}

			if dryRun {
				if jsonOutput {
					return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
						"results": results,
						"count":   len(results),
					})
				}
				cs := f.IOStreams.ColorScheme()
				_, _ = fmt.Fprintf(f.IOStreams.Out, "Would close %d completed epics:\n", len(results))
				for _, r := range results {
					_, _ = fmt.Fprintf(f.IOStreams.Out, "  %s %s\n",
						cs.Bold(r.ID), r.Title)
				}
				return nil
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"results": results,
					"closed":  out.ClosedCount,
				})
			}

			cs := f.IOStreams.ColorScheme()
			for _, r := range results {
				if r.Closed {
					_, _ = fmt.Fprintf(f.IOStreams.Out, "%s Closed %s %s\n",
						cs.SuccessIcon(), cs.Bold(r.ID), r.Title)
				} else {
					_, _ = fmt.Fprintf(f.IOStreams.Out, "%s Skipped %s: %s\n",
						cs.Yellow("!"), cs.Bold(r.ID), r.Message)
				}
			}
			_, _ = fmt.Fprintf(f.IOStreams.Out, "\n%d of %d completed epics closed.\n",
				out.ClosedCount, len(results))

			return nil
		},
	}
}

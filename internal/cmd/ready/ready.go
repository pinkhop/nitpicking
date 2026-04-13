// Package ready provides the "ready" shortcut command — a quick way to list
// issues that are available for work.
package ready

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RunInput holds the parameters for the ready command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	JSON          bool
	Limit         int
	WriteTo       io.Writer
	ColorScheme   *iostreams.ColorScheme
	TerminalWidth int
}

// Run executes the ready workflow: queries for ready issues ordered by priority
// and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Service.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  driving.IssueFilterInput{Ready: true},
		OrderBy: driving.OrderByPriority,
		Limit:   input.Limit,
	})
	if err != nil {
		return fmt.Errorf("listing ready issues: %w", err)
	}

	if input.JSON {
		out := cmdutil.ListOutput{
			HasMore: result.HasMore,
			Items:   cmdutil.ConvertListItems(result.Items),
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	cs := input.ColorScheme
	w := input.WriteTo

	if len(result.Items) == 0 {
		_, _ = fmt.Fprintln(w, "No ready issues.")
		return nil
	}

	// Columns: ID, role, state, priority, title. Estimate non-title overhead
	// as ID(~8) + role(4) + state(~13) + priority(2) + 4 tab paddings(8) = ~35.
	maxTitle := cmdutil.AvailableTitleWidth(input.TerminalWidth, 35)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, item := range result.Items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			cs.Bold(item.ID),
			cs.Dim(item.Role.String()),
			cmdutil.FormatState(cs, item.State, item.SecondaryState),
			cs.Yellow(item.Priority.String()),
			cmdutil.TruncateTitle(item.Title, maxTitle))
	}
	_ = tw.Flush()

	shown := len(result.Items)
	if result.HasMore {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d ready (more available)", shown)))
	} else {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d ready", shown)))
	}

	return nil
}

// NewCmd constructs the "ready" command, which lists issues that are ready
// for work. This is a shortcut for "np list --ready".
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		limit      int
		noLimit    bool
	)

	return &cli.Command{
		Name:  "ready",
		Usage: "List the open issues with no blockers",
		Description: `Shows all issues that are currently available for work — open issues with
no unresolved blocked_by relationships and no ancestor epics that are
themselves blocked. The results are ordered by priority (P0 first).

This is the starting point for finding work. Agents should typically use
"claim ready" to atomically pick and lock the top issue, but "ready" is
useful when you want to browse the queue without committing to anything.
It is equivalent to "list --ready" but shorter to type and easier to
remember.`,
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
			effectiveLimit, err := cmdutil.ResolveLimit(limit, noLimit)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return Run(ctx, RunInput{
				Service:       svc,
				JSON:          jsonOutput,
				Limit:         effectiveLimit,
				WriteTo:       f.IOStreams.Out,
				ColorScheme:   f.IOStreams.ColorScheme(),
				TerminalWidth: f.IOStreams.TerminalWidth(),
			})
		},
	}
}

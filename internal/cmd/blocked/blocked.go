// Package blocked provides the "blocked" shortcut command — lists issues
// that have unresolved blocked_by relationships.
package blocked

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

// RunInput holds the parameters for the blocked command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	JSON          bool
	Limit         int
	WriteTo       io.Writer
	ColorScheme   *iostreams.ColorScheme
	TerminalWidth int
}

// Run executes the blocked workflow: queries for blocked issues (excluding
// closed ones), ordered by priority, and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Service.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  driving.IssueFilterInput{Blocked: true, ExcludeClosed: true},
		OrderBy: driving.OrderByPriority,
		Limit:   input.Limit,
	})
	if err != nil {
		return fmt.Errorf("listing blocked issues: %w", err)
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
		_, _ = fmt.Fprintln(w, "No blocked issues.")
		return nil
	}

	// Columns: ID, role, state, priority, title. Estimate non-title overhead
	// as ID(~8) + role(4) + state(~18) + priority(2) + 4 tab paddings(8) = ~40.
	maxTitle := cmdutil.AvailableTitleWidth(input.TerminalWidth, 40)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, item := range result.Items {
		title := item.Title
		if len(item.BlockerIDs) > 0 {
			title += " " + cmdutil.FormatBlockerSuffix(item.BlockerIDs)
		}
		title = cmdutil.TruncateTitle(title, maxTitle)
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			cs.Bold(item.ID),
			cs.Dim(item.Role.String()),
			cmdutil.FormatState(cs, item.State, item.SecondaryState),
			cs.Yellow(item.Priority.String()),
			title)
	}
	_ = tw.Flush()

	shown := len(result.Items)
	if result.HasMore {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d blocked (more available)", shown)))
	} else {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d blocked", shown)))
	}

	return nil
}

// NewCmd constructs the "blocked" command, which lists issues that have
// unresolved blocked_by relationships.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		limit      int
		noLimit    bool
	)

	return &cli.Command{
		Name:  "blocked",
		Usage: "List open and deferred issues that are blocked",
		Description: `Shows all open and deferred issues that have at least one unresolved
blocked_by relationship. Each issue's title is annotated with the IDs of
its blockers so you can see at a glance what needs to be resolved first.

Use this to diagnose pipeline stalls: if "ready" returns nothing but there
is plenty of work in the tracker, "blocked" reveals which issues are stuck
and what is holding them up. Resolving or closing the blocking issues will
cause the blocked issues to transition to the ready state automatically.

Closed issues are excluded from the output. Results are ordered by
priority so that the highest-impact blockages appear first.`,
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

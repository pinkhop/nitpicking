// Package tally provides the "tally" admin subcommand — a dashboard
// showing summary statistics about the issue database.
package tally

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// tallyOutput is the JSON representation of the tally dashboard.
type tallyOutput struct {
	Open     int `json:"open"`
	Deferred int `json:"deferred"`
	Closed   int `json:"closed"`
	Ready    int `json:"ready"`
	Blocked  int `json:"blocked"`
	Total    int `json:"total"`
}

// RunInput holds the parameters for the tally command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service     driving.Service
	JSON        bool
	WriteTo     io.Writer
	ColorScheme *iostreams.ColorScheme
}

// Run executes the tally workflow: queries the issue summary from the
// service and writes the dashboard to the output writer.
func Run(ctx context.Context, input RunInput) error {
	summary, err := input.Service.GetIssueSummary(ctx)
	if err != nil {
		return fmt.Errorf("fetching issue summary: %w", err)
	}

	out := tallyOutput{
		Open:     summary.Open,
		Deferred: summary.Deferred,
		Closed:   summary.Closed,
		Ready:    summary.Ready,
		Blocked:  summary.Blocked,
		Total:    summary.Total,
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	cs := input.ColorScheme
	w := input.WriteTo
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintf(tw, "Open\t%s\n", stateCount(cs, domain.StateOpen, out.Open))
	_, _ = fmt.Fprintf(tw, "Deferred\t%s\n", stateCount(cs, domain.StateDeferred, out.Deferred))
	_, _ = fmt.Fprintf(tw, "Closed\t%s\n", stateCount(cs, domain.StateClosed, out.Closed))
	_, _ = fmt.Fprintf(tw, "Ready\t%s\n", stateCount(cs, domain.StateOpen, out.Ready))
	_, _ = fmt.Fprintf(tw, "Blocked\t%s\n", blockedCount(cs, out.Blocked))
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "\n%s\n", cs.Dim(fmt.Sprintf("%d total", out.Total)))

	return nil
}

// NewCmd constructs the "tally" command, which displays a summary dashboard
// of issue counts by state.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "tally",
		Usage: "Show issue database summary",
		Description: `Displays a compact dashboard of issue counts broken down by state: open,
deferred, closed, ready, and blocked, plus a total. This gives a quick
health check on the project without listing individual issues.

Open includes issues regardless of claim status — an issue with an active
claim remains open in the primary state; only the secondary state reflects
whether it is ready (no active claim) or claimed (active claim). Zero ready
issues means agents have no work to pick up. In JSON mode (--json) the
output is a flat object suitable for dashboards or trend tracking.`,
		Flags: []cli.Flag{
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

			return Run(ctx, RunInput{
				Service:     svc,
				JSON:        jsonOutput,
				WriteTo:     f.IOStreams.Out,
				ColorScheme: f.IOStreams.ColorScheme(),
			})
		},
	}
}

// stateCount formats a numeric count using the canonical color for the given
// primary state. When the count is zero, the "empty" color (dark gray) is
// applied instead.
func stateCount(cs *iostreams.ColorScheme, state domain.State, count int) string {
	s := fmt.Sprintf("%d", count)
	if count == 0 {
		return cmdutil.ColorEmpty(cs, s)
	}
	return cmdutil.ColorStateText(cs, state, s)
}

// blockedCount formats a numeric count using the canonical "blocked" color.
// When the count is zero, the "empty" color (dark gray) is applied instead.
func blockedCount(cs *iostreams.ColorScheme, count int) string {
	s := fmt.Sprintf("%d", count)
	if count == 0 {
		return cmdutil.ColorEmpty(cs, s)
	}
	return cmdutil.ColorBlockedText(cs, s)
}

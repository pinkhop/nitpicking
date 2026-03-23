package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// findingOutput is the JSON representation of a single diagnostic finding.
type findingOutput struct {
	Category  string   `json:"category"`
	Severity  string   `json:"severity"`
	Message   string   `json:"message"`
	TicketIDs []string `json:"ticket_ids,omitzero"`
}

// doctorOutput is the JSON representation of the doctor command result.
type doctorOutput struct {
	Findings []findingOutput `json:"findings"`
	Healthy  bool            `json:"healthy"`
}

// NewCmd constructs the "doctor" command, which runs diagnostics on the
// database and reports any integrity issues or inconsistencies.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "doctor",
		Usage: "Run diagnostics on the database",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc := f.Service()
			result, err := svc.Doctor(ctx)
			if err != nil {
				return fmt.Errorf("running diagnostics: %w", err)
			}

			healthy := len(result.Findings) == 0

			if jsonOutput {
				out := doctorOutput{
					Healthy:  healthy,
					Findings: make([]findingOutput, 0, len(result.Findings)),
				}
				for _, finding := range result.Findings {
					out.Findings = append(out.Findings, findingOutput{
						Category:  finding.Category,
						Severity:  finding.Severity,
						Message:   finding.Message,
						TicketIDs: finding.TicketIDs,
					})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if healthy {
				_, err := fmt.Fprintf(w, "%s No issues found.\n", cs.SuccessIcon())
				return err
			}

			for _, finding := range result.Findings {
				icon := cs.WarningIcon()
				if finding.Severity == "error" {
					icon = cs.ErrorIcon()
				}

				_, _ = fmt.Fprintf(w, "%s [%s] %s\n", icon, finding.Category, finding.Message)
				if len(finding.TicketIDs) > 0 {
					_, _ = fmt.Fprintf(w, "  Affected tickets: %s\n",
						strings.Join(finding.TicketIDs, ", "))
				}
			}

			return nil
		},
	}
}

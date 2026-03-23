package agentinstructions

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// agentInstructionsOutput is the JSON representation of the
// agent-instructions command result.
type agentInstructionsOutput struct {
	Instructions string `json:"instructions"`
}

// NewCmd constructs the "agent-instructions" command, which returns Markdown
// instructions that agents should follow when interacting with the tracker.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "agent-instructions",
		Usage: "Print agent workflow instructions in Markdown",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc := f.Tracker()
			instructions, err := svc.AgentInstructions(ctx)
			if err != nil {
				return fmt.Errorf("retrieving agent instructions: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, agentInstructionsOutput{
					Instructions: instructions,
				})
			}

			_, err = fmt.Fprintln(f.IOStreams.Out, instructions)
			return err
		},
	}
}

package agentname

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// agentNameOutput is the JSON representation of the agent-name command result.
type agentNameOutput struct {
	Name string `json:"name"`
}

// NewCmd constructs the "agent-name" command, which generates a random agent
// name suitable for use as an author identity.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "agent-name",
		Usage: "Generate a random agent name",
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
			name, err := svc.AgentName(ctx)
			if err != nil {
				return fmt.Errorf("generating agent name: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, agentNameOutput{Name: name})
			}

			_, err = fmt.Fprintln(f.IOStreams.Out, name)
			return err
		},
	}
}

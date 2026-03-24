package agent

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// --- JSON output types ---

// nameOutput is the JSON representation of the "agent name" subcommand result.
type nameOutput struct {
	Name string `json:"name"`
}

// primeOutput is the JSON representation of the "agent prime" subcommand result.
type primeOutput struct {
	Instructions string `json:"instructions"`
}

// NewCmd constructs the "agent" parent command with "name" and "prime"
// subcommands for agent-related operations that do not require database
// discovery.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "agent",
		Usage: "Agent utilities (name generation, workflow instructions)",
		Commands: []*cli.Command{
			newNameCmd(f),
			newPrimeCmd(f),
		},
	}
}

// newNameCmd constructs the "name" subcommand, which generates a random agent
// name suitable for use as an author identity.
func newNameCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "name",
		Usage: "Generate a random agent name",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
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
				return cmdutil.WriteJSON(f.IOStreams.Out, nameOutput{Name: name})
			}

			_, err = fmt.Fprintln(f.IOStreams.Out, name)
			return err
		},
	}
}

// newPrimeCmd constructs the "prime" subcommand, which returns Markdown
// instructions that agents should follow when interacting with the tracker.
// The name "prime" is chosen for brevity and consistency with the beads project.
func newPrimeCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "prime",
		Usage: "Print agent workflow instructions in Markdown",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			instructions, err := svc.AgentInstructions(ctx)
			if err != nil {
				return fmt.Errorf("retrieving agent instructions: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, primeOutput{
					Instructions: instructions,
				})
			}

			_, err = fmt.Fprintln(f.IOStreams.Out, instructions)
			return err
		},
	}
}

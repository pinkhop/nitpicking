package agent

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// --- JSON output types ---

type agentService interface {
	AgentName(ctx context.Context) (string, error)
}

var newAgentService = func(f *cmdutil.Factory) (agentService, error) {
	return cmdutil.NewTracker(f)
}

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
		Description: `The agent command group contains utilities that AI agents (and humans
scripting np) use to bootstrap a session. It does not require an initialized
np workspace — the subcommands operate without database discovery.

Use "agent name" to generate a random, human-readable identity for the
--author flag that every mutation command requires. Use "agent prime" to
retrieve the full Markdown workflow instructions that an agent should follow
when interacting with the tracker. Together, these two subcommands are
typically the first commands an agent runs at the start of a session.`,
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
		Description: `Generates a random three-word agent name (e.g., "keen-flint-trace") suitable
for use as the --author identity on every np mutation command. The name is
deterministic per invocation but not reproducible — each call produces a
fresh name.

Run this once at the start of an agent session and reuse the returned name
for all subsequent commands. The generated name is designed to be memorable
and collision-resistant across concurrent agents working in the same
workspace.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := newAgentService(f)
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
		Description: `Prints the canonical Markdown instructions that define how an agent should
interact with the np issue tracker. The output covers the full agent
workflow: finding work, claiming issues, making mutations, adding comments,
and transitioning state.

Pipe the output into your agent's system prompt or rule file so that the
agent always has up-to-date instructions. The name "prime" follows the
convention established by the beads project — it primes an agent with the
knowledge it needs to operate autonomously.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			instructions := AgentInstructions()

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, primeOutput{
					Instructions: instructions,
				})
			}

			_, err := fmt.Fprintln(f.IOStreams.Out, instructions)
			return err
		},
	}
}

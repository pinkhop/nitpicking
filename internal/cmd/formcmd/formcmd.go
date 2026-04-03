package formcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// NewCmd constructs the "form" parent command, which groups interactive,
// TUI-based subcommands for human-driven issue creation and modification. The
// parent command has no action of its own — it exists only to namespace the
// form subcommands.
//
// Form commands prompt the user for input interactively. They produce
// human-readable text only — there is no --json flag. Agents should use the
// json command tree instead.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "form",
		Usage: "Interactive TUI commands for human-driven issue management",
		Description: `The form command group provides interactive, terminal-based (TUI) workflows
for creating, updating, and commenting on issues. Each subcommand presents a
multi-field form that guides you through the operation step by step, with
validation and sensible defaults built in.

Use form commands when you are a human working at a terminal and want a guided
experience. The form subcommands produce human-readable text output only —
there is no --json flag. AI agents and scripts that need structured I/O should
use the json command group instead.`,
		Commands: []*cli.Command{
			newCreateCmd(f),
			newUpdateCmd(f),
			newCommentCmd(f),
		},
	}
}

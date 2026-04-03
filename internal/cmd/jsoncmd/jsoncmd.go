package jsoncmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// NewCmd constructs the "json" parent command, which groups agent-oriented
// subcommands that read structured JSON from stdin and write JSON to stdout.
// The parent command has no action of its own — it exists only to namespace the
// json subcommands.
//
// All subcommands output JSON unconditionally; there is no --json flag.
// Identity and context flags (--author, --claim) remain on the command line;
// the JSON object on stdin provides content fields only.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "json",
		Usage: "structured JSON input/output commands for agents",
		Description: `The json command group provides structured, machine-readable commands for
creating, updating, and commenting on issues. Each subcommand reads a JSON
object from stdin for content fields and writes a JSON object to stdout —
there is no --json flag because output is always JSON. Identity and context
flags (--author, --claim) remain on the command line.

Use json commands when you are an AI agent, a script, or any automated
workflow that needs predictable, parseable input and output. The json
subcommands mirror the capabilities of the form command group but are
designed for non-interactive use. Humans working at a terminal may prefer
the form commands, which provide a guided TUI experience.`,
		Commands: []*cli.Command{
			newCreateCmd(f),
			newUpdateCmd(f),
			newCommentCmd(f),
		},
	}
}

package importcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// NewCmd constructs the "import" parent command, which groups subcommands for
// importing issues from various file formats. The parent command has no action
// of its own — it exists only to namespace the import subcommands.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "import",
		Usage: "Import issues from structured files",
		Description: `Groups subcommands for bulk-creating issues from structured input files.
Use "import jsonl" for multi-issue batch imports from a JSONL file.

The import pipeline is two-phase: all records are validated against the
database prefix and domain rules before any mutations occur. If
validation fails, no issues are created. This makes import safe to retry
— combine with idempotency keys to ensure repeated runs do not create
duplicates.`,
		Commands: []*cli.Command{
			newJSONLCmd(f),
		},
	}
}

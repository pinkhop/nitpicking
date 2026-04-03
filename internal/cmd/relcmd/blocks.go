package relcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// newBlocksCmd constructs the "rel blocks" parent command with unblock and
// list subcommands for managing blocking relationships.
func newBlocksCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "blocks",
		Usage: "Manage blocking relationships",
		Description: `Groups commands for inspecting and removing blocking dependencies between
issues. A blocking relationship (blocked_by/blocks) prevents the blocked issue
from becoming ready until the blocker is closed.

Use "blocks list" to see all blocking relationships for a given issue, and
"blocks unblock" to remove a blocking edge. To create a new blocking
relationship, use "rel add <A> blocked_by <B>".`,
		Commands: []*cli.Command{
			newUnblockCmd(f),
			newRelTypeListCmd(f, "blocks", "blocked_by", "blocks"),
		},
	}
}

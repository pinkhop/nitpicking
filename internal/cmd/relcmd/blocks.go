package relcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// newBlocksCmd constructs the "rel blocks" parent command with unblock and
// list subcommands for managing blocking relationships.
func newBlocksCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "blocks",
		Usage: "Manage blocking relationships",
		Commands: []*cli.Command{
			newUnblockCmd(f),
			newRelTypeListCmd(f, "blocks", issue.RelBlockedBy, issue.RelBlocks),
		},
	}
}

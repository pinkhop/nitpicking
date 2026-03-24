package relcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// newCitesCmd constructs the "rel cites" parent command with add, remove,
// and list subcommands for managing citation relationships.
func newCitesCmd(f *cmdutil.Factory) *cli.Command {
	removeCmd := newRelRemoveCmd(f, "cites", issue.RelCites)
	removeCmd.Hidden = true

	return &cli.Command{
		Name:  "cites",
		Usage: "Manage citation relationships",
		Commands: []*cli.Command{
			newRelAddCmd(f, "cites", issue.RelCites),
			newUnciteCmd(f),
			removeCmd,
			newRelTypeListCmd(f, "cites", issue.RelCites, issue.RelCitedBy),
		},
	}
}

package relcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// newCitesCmd constructs the "rel cites" parent command with uncite and
// list subcommands for managing citation relationships.
func newCitesCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "cites",
		Usage: "Manage citation relationships",
		Commands: []*cli.Command{
			newUnciteCmd(f),
			newRelTypeListCmd(f, "cites", issue.RelCites, issue.RelCitedBy),
		},
	}
}

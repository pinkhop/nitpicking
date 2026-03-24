package relcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// newRefsCmd constructs the "rel refs" parent command with unref and list
// subcommands for managing contextual reference relationships.
func newRefsCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "refs",
		Usage: "Manage contextual reference relationships",
		Commands: []*cli.Command{
			newUnrefCmd(f),
			newRelTypeListCmd(f, "refs", issue.RelRefs),
		},
	}
}

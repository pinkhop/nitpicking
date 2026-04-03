package relcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// newRefsCmd constructs the "rel refs" parent command with unref and list
// subcommands for managing contextual reference relationships.
func newRefsCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "refs",
		Usage: "Manage contextual reference relationships",
		Description: `Groups commands for inspecting and removing contextual reference relationships.
A "refs" relationship is symmetric and informational — it links two related
issues without creating a dependency. Neither issue blocks the other.

Use "refs list" to see all reference relationships for a given issue, and
"refs unref" to remove one. To create a new reference, use
"rel add <A> refs <B>".`,
		Commands: []*cli.Command{
			newUnrefCmd(f),
			newRelTypeListCmd(f, "refs", "refs"),
		},
	}
}

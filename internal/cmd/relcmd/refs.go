package relcmd

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// newRefsCmd constructs the "rel refs" parent command with a list subcommand
// for inspecting contextual reference relationships.
func newRefsCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "refs",
		Usage: "Inspect contextual reference relationships",
		Description: `Groups commands for inspecting contextual reference relationships. A "refs"
relationship is symmetric and informational — it links two related issues
without creating a dependency. Neither issue blocks the other.

Use "refs list" to see all reference relationships for a given issue. To
create a new reference, use "rel add <A> refs <B>". To remove one, use
"rel remove <A> refs <B>".`,
		Commands: []*cli.Command{
			newRelTypeListCmd(f, "refs", "refs"),
		},
	}
}

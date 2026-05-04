// Package fix implements the "admin fix" parent command group, which houses
// automated remediations for conditions that "np admin doctor" detects and
// that no other np command already addresses.
package fix

import (
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/fix/gitignore"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/fix/invalidparent"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// NewCmd constructs the "fix" parent command, which groups automated
// remediation subcommands for doctor-detected conditions. Invoking "np admin
// fix" with no subcommand prints help and exits 0; invoking it with an unknown
// subcommand exits non-zero (urfave/cli/v3 default).
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "fix",
		Usage: "Automated remediations for doctor-detected conditions",
		Description: `Home for automated remediations of conditions that 'np admin doctor'
detects and that no other np command already addresses. Each fix
subcommand corresponds to a doctor check slug; running the subcommand
applies the remediation mechanically and idempotently.

There is no '--all' mode. Each fix is invoked explicitly so that the
operator (or agent) decides per-fix rather than blanket-approving a
batch. Use '--dry-run' on any subcommand to preview changes before
applying them.`,
		Commands: []*cli.Command{
			gitignore.NewCmd(f),
			invalidparent.NewCmd(f),
		},
	}
}

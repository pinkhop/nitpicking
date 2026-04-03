package relcmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// newUnrefCmd constructs "rel refs unref <A> <B>" which removes a refs
// relationship between two issues. Since refs is symmetric, direction does
// not matter — the stored relationship is deleted regardless of which
// issue was originally the source.
func newUnrefCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "unref",
		Usage:     "Remove a reference relationship between two issues",
		ArgsUsage: "<A> <B>",
		Description: `Removes the "refs" relationship between two issues. Since refs is a symmetric
relationship type, the argument order does not matter — the stored edge is
deleted regardless of which issue was originally the source.

Use this when a contextual reference is no longer relevant, such as when the
related issue has been closed or the connection was added by mistake.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &author,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 2 {
				return cmdutil.FlagErrorf(
					"expected 2 arguments: <A> <B>, got %d", cmd.NArg(),
				)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			aID, err := resolver.Resolve(ctx, cmd.Args().Get(0))
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID %q: %s", cmd.Args().Get(0), err)
			}
			bID, err := resolver.Resolve(ctx, cmd.Args().Get(1))
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID %q: %s", cmd.Args().Get(1), err)
			}

			// Symmetric delete — the storage layer handles direction.
			err = svc.RemoveRelationship(ctx, aID.String(), driving.RelationshipInput{
				Type:     domain.RelRefs,
				TargetID: bID.String(),
			}, author)
			if err != nil {
				return fmt.Errorf("removing refs: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"a":      aID.String(),
					"b":      bID.String(),
					"action": "unrefed",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Removed ref between %s and %s\n",
				cs.SuccessIcon(),
				cs.Bold(aID.String()),
				cs.Bold(bID.String()))
			return err
		},
	}
}

package relcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RunUnblockInput holds the parameters for the unblock command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunUnblockInput struct {
	Service driving.Service
	A       string
	B       string
	Author  string
	JSON    bool
	WriteTo io.Writer
}

// RunUnblock removes any blocking relationship between A and B regardless
// of direction. Delegates to the service's RemoveBidirectionalBlock method
// which tries both "A blocked_by B" and "B blocked_by A" directions.
func RunUnblock(ctx context.Context, input RunUnblockInput) error {
	err := input.Service.RemoveBidirectionalBlock(ctx, input.A, input.B, input.Author)
	if err != nil {
		return err
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"a":      input.A,
			"b":      input.B,
			"action": "unblocked",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Unblocked %s and %s\n", input.A, input.B)
	return err
}

// newUnblockCmd constructs "rel blocks unblock <A> <B>" which removes any
// blocking relationship between two issues regardless of direction.
func newUnblockCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "unblock",
		Usage:     "Remove blocking relationships between two issues",
		ArgsUsage: "<A> <B>",
		Description: `Removes any blocking relationship between two issues regardless of direction.
If A blocks B, or B blocks A, or both, all blocking edges between the pair are
deleted.

Use this when a dependency is no longer relevant — for example, when the
blocking issue's work has been absorbed into another task, or when the
dependency was added in error. The argument order does not matter.`,
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

			return RunUnblock(ctx, RunUnblockInput{
				Service: svc,
				A:       aID.String(),
				B:       bID.String(),
				Author:  author,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

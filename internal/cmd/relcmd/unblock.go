package relcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// RunUnblockInput holds the parameters for the unblock command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunUnblockInput struct {
	Service service.Service
	A       issue.ID
	B       issue.ID
	Author  identity.Author
	JSON    bool
	WriteTo io.Writer
}

// RunUnblock removes any blocking relationship between A and B regardless
// of direction. It attempts to remove both "A blocked_by B" and
// "B blocked_by A". Both removals are idempotent — if neither relationship
// exists, the command succeeds silently.
func RunUnblock(ctx context.Context, input RunUnblockInput) error {
	// Remove A blocked_by B.
	err := input.Service.RemoveRelationship(ctx, input.A, service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: input.B,
	}, input.Author)
	if err != nil {
		return fmt.Errorf("removing blocked_by from %s: %w", input.A, err)
	}

	// Remove B blocked_by A (reverse direction).
	err = input.Service.RemoveRelationship(ctx, input.B, service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: input.A,
	}, input.Author)
	if err != nil {
		return fmt.Errorf("removing blocked_by from %s: %w", input.B, err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"a":      input.A.String(),
			"b":      input.B.String(),
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
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 2 {
				return cmdutil.FlagErrorf(
					"expected 2 arguments: <A> <B>, got %d", cmd.NArg(),
				)
			}

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
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
				A:       aID,
				B:       bID,
				Author:  parsedAuthor,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

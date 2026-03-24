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

// RunUnciteInput holds the parameters for the uncite command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunUnciteInput struct {
	Service service.Service
	A       issue.ID
	B       issue.ID
	Author  identity.Author
	JSON    bool
	WriteTo io.Writer
}

// RunUncite removes any citation relationship between A and B regardless
// of direction. It attempts to remove both "A cites B" and "B cites A".
// Both removals are idempotent — if neither relationship exists, the
// command succeeds silently.
func RunUncite(ctx context.Context, input RunUnciteInput) error {
	// Remove A cites B.
	err := input.Service.RemoveRelationship(ctx, input.A, service.RelationshipInput{
		Type:     issue.RelCites,
		TargetID: input.B,
	}, input.Author)
	if err != nil {
		return fmt.Errorf("removing cites from %s: %w", input.A, err)
	}

	// Remove B cites A (reverse direction).
	err = input.Service.RemoveRelationship(ctx, input.B, service.RelationshipInput{
		Type:     issue.RelCites,
		TargetID: input.A,
	}, input.Author)
	if err != nil {
		return fmt.Errorf("removing cites from %s: %w", input.B, err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"a":      input.A.String(),
			"b":      input.B.String(),
			"action": "uncited",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Uncited %s and %s\n", input.A, input.B)
	return err
}

// newUnciteCmd constructs "rel cites uncite <A> <B>" which removes any
// citation relationship between two issues regardless of direction.
func newUnciteCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "uncite",
		Usage:     "Remove citation relationships between two issues",
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

			return RunUncite(ctx, RunUnciteInput{
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

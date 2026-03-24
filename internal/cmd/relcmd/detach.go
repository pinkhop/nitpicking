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

// RunDetachInput holds the parameters for the detach command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunDetachInput struct {
	Service service.Service
	A       issue.ID
	B       issue.ID
	Author  identity.Author
	JSON    bool
	WriteTo io.Writer
}

// RunDetach removes the parent-child relationship between A and B. The order
// of A and B does not matter — the command inspects both issues to determine
// which is the child. Uses one-shot update (atomic claim→update→release) so
// no explicit claim is needed.
func RunDetach(ctx context.Context, input RunDetachInput) error {
	// Determine which of A/B is the child by checking parent IDs.
	childID, parentID, err := resolveParentChild(ctx, input.Service, input.A, input.B)
	if err != nil {
		return err
	}

	zeroID := issue.ID{}
	err = input.Service.OneShotUpdate(ctx, service.OneShotUpdateInput{
		IssueID:  childID,
		Author:   input.Author,
		ParentID: &zeroID,
	})
	if err != nil {
		return fmt.Errorf("detaching parent: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"child":  childID.String(),
			"parent": parentID.String(),
			"action": "detached",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Detached %s from parent %s\n", childID, parentID)
	return err
}

// resolveParentChild determines which of issueA and issueB is the child in a
// parent-child relationship. Returns (childID, parentID, nil) if found, or an
// error if neither issue is a child of the other.
func resolveParentChild(ctx context.Context, svc service.Service, issueA, issueB issue.ID) (issue.ID, issue.ID, error) {
	shownA, err := svc.ShowIssue(ctx, issueA)
	if err != nil {
		return issue.ID{}, issue.ID{}, fmt.Errorf("looking up %s: %w", issueA, err)
	}
	if shownA.Issue.ParentID() == issueB {
		return issueA, issueB, nil
	}

	shownB, err := svc.ShowIssue(ctx, issueB)
	if err != nil {
		return issue.ID{}, issue.ID{}, fmt.Errorf("looking up %s: %w", issueB, err)
	}
	if shownB.Issue.ParentID() == issueA {
		return issueB, issueA, nil
	}

	return issue.ID{}, issue.ID{}, fmt.Errorf("no parent-child relationship between %s and %s", issueA, issueB)
}

// newPositionalDetachCmd constructs "rel parent detach <A> <B>" which removes
// the parent-child relationship between two issues using positional arguments
// and one-shot update (no claim required).
func newPositionalDetachCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "detach",
		Usage:     "Remove the parent-child relationship between two issues",
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

			return RunDetach(ctx, RunDetachInput{
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

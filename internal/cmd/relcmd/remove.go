package relcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RunRemoveInput holds the parameters for the remove command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunRemoveInput struct {
	// Service is the tracker service used to remove the relationship.
	Service driving.Service
	// A is the source issue ID string.
	A string
	// Rel is the relationship type string (e.g., "blocked_by", "blocks", "refs",
	// "parent_of", "child_of").
	Rel string
	// B is the target issue ID string.
	B string
	// Author identifies who is performing the removal.
	Author string
	// JSON, when true, emits a machine-readable JSON response instead of prose.
	JSON bool
	// WriteTo is the writer for command output.
	WriteTo io.Writer
}

// RunRemove executes the rel remove workflow. It parses the relationship
// argument, dispatches to the appropriate service method, and writes output.
// This mirrors RunAdd in structure — the same <rel> values are accepted, and
// the same dispatch rules apply: parent_of/child_of delegate to the detach
// logic, while blocked_by/blocks/refs delegate to RemoveRelationship.
func RunRemove(ctx context.Context, input RunRemoveInput) error {
	parsed, err := ParseRelArg(input.Rel)
	if err != nil {
		return err
	}

	switch parsed.Type {
	case RelArgParentOf:
		// A parent_of B → detach B from A.
		return runRemoveParent(ctx, input, input.B, input.A)
	case RelArgChildOf:
		// A child_of B → detach A from B.
		return runRemoveParent(ctx, input, input.A, input.B)
	case RelArgRelationship:
		return runRemoveRelationship(ctx, input, parsed)
	default:
		return fmt.Errorf("unexpected relationship dispatch type: %d", parsed.Type)
	}
}

// runRemoveParent verifies that childID is actually a child of parentID, then
// detaches it using the one-shot update path. Unlike RunDetach (which is
// order-independent), this function enforces the exact direction stated by the
// caller — "A parent_of B" only removes the edge where B's parent is A, not the
// reverse. Without this check, the command would silently clear a child's
// parent even when the named edge does not exist.
func runRemoveParent(ctx context.Context, input RunRemoveInput, childID, parentID string) error {
	// Fetch the child issue and confirm its parent matches parentID. This
	// enforces the caller's stated direction rather than accepting either
	// permutation, which is what RunDetach's resolveParentChild allows.
	shown, err := input.Service.ShowIssue(ctx, childID)
	if err != nil {
		return fmt.Errorf("looking up %s: %w", childID, err)
	}
	if shown.ParentID != parentID {
		return fmt.Errorf("no parent-child relationship between %s and %s", parentID, childID)
	}

	emptyParent := ""
	if err := input.Service.OneShotUpdate(ctx, driving.OneShotUpdateInput{
		IssueID:  childID,
		Author:   input.Author,
		ParentID: &emptyParent,
	}); err != nil {
		return fmt.Errorf("detaching parent: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"child":  childID,
			"parent": parentID,
			"action": "removed",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Detached %s from parent %s\n", childID, parentID)
	return err
}

// runRemoveRelationship removes a standard relationship (blocked_by, blocks, refs)
// via RemoveRelationship. For "blocks", the direction is inverted: "A blocks B"
// is stored as "B blocked_by A", so we remove B's blocked_by A edge.
func runRemoveRelationship(ctx context.Context, input RunRemoveInput, parsed RelArgResult) error {
	// "A blocks B" is the inverse of "B blocked_by A". To remove it, we must
	// delete the blocked_by edge stored on B, not on A.
	sourceID := input.A
	targetID := input.B
	relType := parsed.RelType
	if parsed.Label == "blocks" {
		sourceID = input.B
		targetID = input.A
		relType = domain.RelBlockedBy
	}

	rel := driving.RelationshipInput{
		Type:     relType,
		TargetID: targetID,
	}
	if err := input.Service.RemoveRelationship(ctx, sourceID, rel, input.Author); err != nil {
		return fmt.Errorf("removing %s relationship: %w", parsed.Label, err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"source": input.A,
			"type":   parsed.Label,
			"target": input.B,
			"action": "removed",
		})
	}

	_, err := fmt.Fprintf(input.WriteTo, "Removed %s: %s → %s\n", parsed.Label, input.A, input.B)
	return err
}

// newRemoveCmd constructs "rel remove <A> <rel> <B>" which removes a
// relationship between two issues using the same positional argument syntax as
// "rel add". This command replaces the type-specific removal subcommands
// ("rel blocks unblock", "rel refs unref") with a single, predictable surface.
func newRemoveCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "remove",
		Usage:     "Remove a relationship between two issues",
		ArgsUsage: "<A> <rel> <B>  where <rel> is: " + validRelArgs,
		Description: `Removes a directional relationship between two issues. The argument syntax
mirrors "rel add" exactly: the first argument is the source issue, the second
is the relationship type, and the third is the target issue.

Supported relationship types:
  blocked_by / blocks  — Removes the blocking dependency. "A blocked_by B" and
                         "A blocks B" are both accepted and handle the stored
                         direction correctly.
  refs                 — Removes the symmetric contextual reference.
  parent_of / child_of — Removes the parent-child link using a one-shot update
                         (no explicit claim required).`,
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
			if cmd.NArg() != 3 {
				return cmdutil.FlagErrorf(
					"expected 3 arguments: <A> <rel> <B>, got %d", cmd.NArg(),
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
			bID, err := resolver.Resolve(ctx, cmd.Args().Get(2))
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID %q: %s", cmd.Args().Get(2), err)
			}

			return RunRemove(ctx, RunRemoveInput{
				Service: svc,
				A:       aID.String(),
				Rel:     cmd.Args().Get(1),
				B:       bID.String(),
				Author:  author,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

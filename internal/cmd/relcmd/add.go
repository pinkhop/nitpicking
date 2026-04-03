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

// RelArgType classifies how a relationship argument should be dispatched.
type RelArgType int

const (
	// RelArgRelationship means the argument maps to a standard
	// AddRelationship call (blocks, blocked_by, cites, cited_by, refs).
	RelArgRelationship RelArgType = iota + 1

	// RelArgParentOf means A is the parent of B — sets B's parent to A.
	RelArgParentOf

	// RelArgChildOf means A is a child of B — sets A's parent to B.
	RelArgChildOf
)

// RelArgResult holds the parsed result of a relationship argument string.
type RelArgResult struct {
	// Type classifies the dispatch path.
	Type RelArgType
	// Label is the canonical string form of the relationship (e.g. "blocked_by").
	Label string
	// RelType is the domain relationship type, populated only when Type is
	// RelArgRelationship.
	RelType domain.RelationType
}

// validRelArgs enumerates all accepted <rel> values for help text.
const validRelArgs = "blocked_by, blocks, refs, cites, cited_by, parent_of, child_of"

// ParseRelArg parses a relationship argument string into a dispatch decision.
// Returns an error if the argument is not one of the six accepted values.
func ParseRelArg(s string) (RelArgResult, error) {
	switch s {
	case "parent_of":
		return RelArgResult{Type: RelArgParentOf, Label: "parent_of"}, nil
	case "child_of":
		return RelArgResult{Type: RelArgChildOf, Label: "child_of"}, nil
	default:
		rt, err := domain.ParseRelationType(s)
		if err != nil {
			return RelArgResult{}, fmt.Errorf(
				"invalid relationship %q: must be one of %s", s, validRelArgs,
			)
		}
		return RelArgResult{
			Type:    RelArgRelationship,
			Label:   s,
			RelType: rt,
		}, nil
	}
}

// RunAddInput holds the parameters for the add command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunAddInput struct {
	Service driving.Service
	A       string
	Rel     string
	B       string
	ClaimID string
	Author  string
	JSON    bool
	WriteTo io.Writer
}

// RunAdd executes the rel add workflow. It parses the relationship argument,
// dispatches to the appropriate service method, and writes output.
func RunAdd(ctx context.Context, input RunAddInput) error {
	parsed, err := ParseRelArg(input.Rel)
	if err != nil {
		return err
	}

	switch parsed.Type {
	case RelArgParentOf:
		return runAddParent(ctx, input, input.B, input.A)
	case RelArgChildOf:
		return runAddParent(ctx, input, input.A, input.B)
	case RelArgRelationship:
		return runAddRelationship(ctx, input, parsed)
	default:
		return fmt.Errorf("unexpected relationship dispatch type: %d", parsed.Type)
	}
}

// runAddParent sets childID's parent to parentID via UpdateIssue.
func runAddParent(ctx context.Context, input RunAddInput, childID, parentID string) error {
	if input.ClaimID == "" {
		return fmt.Errorf("--claim is required for parent_of/child_of relationships")
	}

	err := input.Service.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID:  childID,
		ClaimID:  input.ClaimID,
		ParentID: &parentID,
	})
	if err != nil {
		return fmt.Errorf("setting parent: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"child":  childID,
			"parent": parentID,
			"action": "added",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Set parent: %s → %s\n", childID, parentID)
	return err
}

// runAddRelationship creates a standard relationship (blocks, cites, etc.)
// via AddRelationship.
func runAddRelationship(ctx context.Context, input RunAddInput, parsed RelArgResult) error {
	rel := driving.RelationshipInput{
		Type:     parsed.RelType,
		TargetID: input.B,
	}
	if err := input.Service.AddRelationship(ctx, input.A, rel, input.Author); err != nil {
		return fmt.Errorf("adding %s relationship: %w", parsed.Label, err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, map[string]string{
			"source": input.A,
			"type":   parsed.Label,
			"target": input.B,
			"action": "added",
		})
	}

	_, err := fmt.Fprintf(input.WriteTo, "Added %s: %s → %s\n", parsed.Label, input.A, input.B)
	return err
}

// newAddCmd constructs "rel add <A> <rel> <B>" which creates a relationship
// between two issues using positional arguments.
func newAddCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
		claimID    string
	)

	return &cli.Command{
		Name:      "add",
		Usage:     "Add a relationship between two issues",
		ArgsUsage: "<A> <rel> <B>  where <rel> is: " + validRelArgs,
		Description: `Creates a directional relationship between two issues. The first argument is
the source issue, the second is the relationship type, and the third is the
target issue.

Supported relationship types:
  blocked_by / blocks  — Dependency ordering. A "blocked_by" B means A cannot
                         become ready until B is closed.
  refs                 — Symmetric contextual reference. Neither issue blocks
                         the other; the link is informational.
  cites / cited_by     — Directional citation for traceability.
  parent_of / child_of — Structural hierarchy. Requires --claim because it
                         mutates the child issue's parent field.

Use "rel blocks unblock", "rel refs unref", or "rel parent detach" to remove
relationships.`,
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
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Claim ID (required for parent_of/child_of)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &claimID,
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

			return RunAdd(ctx, RunAddInput{
				Service: svc,
				A:       aID.String(),
				Rel:     cmd.Args().Get(1),
				B:       bID.String(),
				ClaimID: claimID,
				Author:  author,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

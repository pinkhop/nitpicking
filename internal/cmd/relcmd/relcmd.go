// Package relcmd provides the "rel" parent command, which groups relationship
// management operations under a single namespace. Subcommands are organized by
// relationship type (blocks, refs, parent) and include utilities for listing
// and tree views.
package relcmd

import (
	"context"
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd/graphcmd"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// FilterRelationships returns only the relationships whose type string matches
// any of the given type strings. This is the shared filtering logic used by
// blocks list and refs list.
func FilterRelationships(rels []driving.RelationshipDTO, types ...string) []driving.RelationshipDTO {
	var result []driving.RelationshipDTO
	for _, r := range rels {
		if slices.Contains(types, r.Type) {
			result = append(result, r)
		}
	}
	return result
}

// NewCmd constructs the "rel" parent command with subcommands for managing
// issue relationships by type.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:    "rel",
		Aliases: []string{"r"},
		Usage:   "Manage relationships between issues",
		Description: `Provides commands for creating, listing, and removing relationships between
issues. Relationships express structural and dependency information: blocking
dependencies (blocked_by/blocks), contextual references (refs), and
parent-child hierarchy (parent_of/child_of).

Use "rel add" to create relationships, "rel remove" to delete them, and
"rel issue" or "rel tree" to inspect all relationships from a given issue.
Both commands accept the same <rel> argument values, making the surface
predictable. To detect circular blocking dependencies, use "np admin doctor".`,
		Commands: []*cli.Command{
			newAddCmd(f),
			newRemoveCmd(f),
			newBlocksCmd(f),
			newRefsCmd(f),
			newParentCmd(f),
			newIssueCmd(f),
			newTreeCmd(f),
			graphcmd.NewCmd(f),
		},
	}
}

// newIssueCmd constructs "rel issue <ID>" which shows all relationships for a
// single issue. The name "issue" distinguishes this per-issue view from the
// forthcoming "rel list" command, which will enumerate relationships across all
// active issues.
func newIssueCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "issue",
		Usage: "List all relationships for an issue",
		Description: `Shows every relationship attached to the given issue, including blocking
dependencies, contextual references, and parent-child links. The output
includes both directions — for example, if issue A blocks issue B, running
"rel issue" on A shows the "blocks" edge and running it on B shows the
"blocked_by" edge.

Use this command when you need a complete picture of how an issue connects to
the rest of the tracker. For type-specific views, use the "rel blocks list",
"rel refs list", or "rel parent children" subcommands instead.`,
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("issue ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			shown, err := svc.ShowIssue(ctx, issueID.String())
			if err != nil {
				return fmt.Errorf("looking up issue: %w", err)
			}

			// ShowIssueOutput.Relationships already includes synthetic
			// parent/child entries — no augmentation needed.
			return renderRelationships(f, shown.Relationships, issueID, jsonOutput)
		},
	}
}

// newTreeCmd constructs "rel tree <ID>" which shows the parent-child hierarchy
// from a given root domain.
func newTreeCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "tree",
		Usage: "Show the hierarchy tree starting from an issue",
		Description: `Renders the parent-child hierarchy rooted at the given issue as a flat list
of descendants. Each descendant is shown with its ID, role, state, and title.
This is a convenience shortcut that is equivalent to listing all issues with
a "descendants_of" filter.

Use this when you want to see the full scope of work beneath an epic or any
issue that has children. For a structured tree view with indentation and
connectors, use "rel parent tree" instead.`,
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("issue ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
				Filter:  driving.IssueFilterInput{DescendantsOf: issueID.String()},
				OrderBy: driving.OrderByPriority,
				Limit:   -1, // Always return the full hierarchy; truncating a tree is misleading.
			})
			if err != nil {
				return fmt.Errorf("listing descendants: %w", err)
			}

			if jsonOutput {
				out := cmdutil.ListOutput{
					Items:   cmdutil.ConvertListItems(result.Items),
					HasMore: result.HasMore,
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			// Show the root issue first.
			root, err := svc.ShowIssue(ctx, issueID.String())
			if err != nil {
				return fmt.Errorf("looking up root issue: %w", err)
			}
			_, _ = fmt.Fprintf(w, "%s %s (%s)\n",
				cs.Bold(issueID.String()),
				root.Title,
				cmdutil.ColorState(cs, root.State))

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "  (no descendants)")
				return nil
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, item := range result.Items {
				_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n",
					cs.Bold(item.ID),
					cs.Dim(item.Role.String()),
					cmdutil.FormatState(cs, item.State, item.SecondaryState),
					item.Title)
			}
			_ = tw.Flush()

			return nil
		},
	}
}

// newRelTypeListCmd constructs a "list <ID>" subcommand that shows
// relationships filtered by the given type strings.
func newRelTypeListCmd(f *cmdutil.Factory, typeName string, types ...string) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "list",
		Usage: fmt.Sprintf("List %s relationships for an issue", typeName),
		Description: fmt.Sprintf(`Shows only the %s relationships for the given issue, filtering out all
other relationship types. This is a focused alternative to "rel issue" when you
only care about one category of relationships.`, typeName),
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("issue ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			shown, err := svc.ShowIssue(ctx, issueID.String())
			if err != nil {
				return fmt.Errorf("looking up issue: %w", err)
			}

			filtered := FilterRelationships(shown.Relationships, types...)
			return renderRelationships(f, filtered, issueID, jsonOutput)
		},
	}
}

// renderRelationships renders a list of relationships to the output.
func renderRelationships(f *cmdutil.Factory, rels []driving.RelationshipDTO, issueID domain.ID, jsonOutput bool) error {
	if jsonOutput {
		type relJSON struct {
			SourceID string `json:"source_id"`
			Type     string `json:"type"`
			TargetID string `json:"target_id"`
		}
		type output struct {
			IssueID       string    `json:"issue_id"`
			Relationships []relJSON `json:"relationships"`
		}
		out := output{
			IssueID:       issueID.String(),
			Relationships: make([]relJSON, 0, len(rels)),
		}
		for _, r := range rels {
			out.Relationships = append(out.Relationships, relJSON{
				SourceID: r.SourceID,
				Type:     r.Type,
				TargetID: r.TargetID,
			})
		}
		return cmdutil.WriteJSON(f.IOStreams.Out, out)
	}

	w := f.IOStreams.Out
	cs := f.IOStreams.ColorScheme()

	if len(rels) == 0 {
		_, _ = fmt.Fprintln(w, "No relationships found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rels {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n",
			cs.Bold(r.SourceID),
			r.Type,
			cs.Bold(r.TargetID))
	}
	_ = tw.Flush()

	_, _ = fmt.Fprintf(w, "\n%s\n",
		cs.Dim(fmt.Sprintf("%d relationships", len(rels))))
	return nil
}

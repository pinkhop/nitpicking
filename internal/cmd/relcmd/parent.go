package relcmd

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// newParentCmd constructs the "rel parent" parent command with detach,
// children, and tree subcommands for managing parent-child hierarchy.
func newParentCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "parent",
		Usage: "Manage parent-child hierarchy",
		Description: `Groups commands for inspecting and modifying the parent-child hierarchy
between issues. Parent-child relationships define structural organization:
epics contain tasks, and tasks may be nested under sub-epics.

Use "parent children" to list direct children of an issue, "parent tree" to
see the full descendant hierarchy with indentation, and "parent detach" to
remove a parent-child link. To set a new parent, use
"rel add <child> child_of <parent>".`,
		Commands: []*cli.Command{
			newPositionalDetachCmd(f),
			newChildrenCmd(f),
			newParentTreeCmd(f),
		},
	}
}

// newChildrenCmd constructs "rel parent children <ID>" which lists direct
// children of an issue.
func newChildrenCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "children",
		Usage: "List direct children of an issue",
		Description: `Lists the immediate children of the given issue — tasks and sub-epics that
have it set as their parent. The output includes each child's ID, role, state,
priority, and title, ordered by priority.

Use this when you want to see what work is organized under an epic or when
checking whether a parent issue's children are all closed (which determines
whether the epic can complete). For the full recursive hierarchy including
grandchildren and beyond, use "rel parent tree" instead.`,
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
				Filter:  driving.IssueFilterInput{ParentIDs: []string{issueID.String()}},
				OrderBy: driving.OrderByPriority,
			})
			if err != nil {
				return fmt.Errorf("listing children: %w", err)
			}

			if jsonOutput {
				out := cmdutil.ListOutput{
					HasMore: result.HasMore,
					Items:   cmdutil.ConvertListItems(result.Items),
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "No children found.")
				return nil
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, item := range result.Items {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					cs.Bold(item.ID),
					cs.Dim(item.Role.String()),
					cmdutil.FormatState(cs, item.State, item.SecondaryState),
					cs.Yellow(item.Priority.String()),
					item.Title)
			}
			_ = tw.Flush()

			shown := len(result.Items)
			if result.HasMore {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d children (more available)", shown)))
			} else {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d children", shown)))
			}
			return nil
		},
	}
}

// newParentTreeCmd constructs "rel parent tree <ID>" which shows the full
// descendant hierarchy from a given issue.
func newParentTreeCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "tree",
		Usage: "Show the full descendant hierarchy of an issue",
		Description: `Renders the complete descendant hierarchy of the given issue as an indented
tree with filesystem-style connectors (using the characters that indicate
nesting depth and sibling position). Each node shows its ID, role, state,
priority, and title.

Use this when you need to visualize the full structure beneath an epic,
including nested sub-epics and their children. For a flat list of direct
children only, use "rel parent children" instead.`,
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

			// Fetch the root issue for display.
			rootShown, err := svc.ShowIssue(ctx, issueID.String())
			if err != nil {
				return fmt.Errorf("looking up root issue: %w", err)
			}

			result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
				Filter:  driving.IssueFilterInput{DescendantsOf: issueID.String()},
				OrderBy: driving.OrderByPriority,
				Limit:   -1,
			})
			if err != nil {
				return fmt.Errorf("listing descendants: %w", err)
			}

			if jsonOutput {
				out := cmdutil.ListOutput{
					HasMore: result.HasMore,
					Items:   cmdutil.ConvertListItems(result.Items),
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			// Render root issue.
			_, _ = fmt.Fprintf(w, "%s  %s  %s  %s  %s\n",
				cs.Bold(rootShown.ID),
				cs.Dim(rootShown.Role.String()),
				cmdutil.ColorState(cs, rootShown.State),
				cs.Yellow(rootShown.Priority.String()),
				rootShown.Title)

			if len(result.Items) == 0 {
				return nil
			}

			// Build parent→children index for tree rendering.
			childrenOf := make(map[string][]driving.IssueListItemDTO)
			for _, item := range result.Items {
				childrenOf[item.ParentID] = append(childrenOf[item.ParentID], item)
			}

			// Render tree recursively from root.
			renderTree(w, cs, childrenOf, issueID.String(), "")

			return nil
		},
	}
}

// renderTree recursively renders children of the given parentID with
// filesystem-style tree connectors (├── and └──).
func renderTree(w io.Writer, cs *iostreams.ColorScheme, childrenOf map[string][]driving.IssueListItemDTO, parentID, prefix string) {
	children := childrenOf[parentID]
	for i, item := range children {
		isLast := i == len(children)-1

		// Choose connector based on position.
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		_, _ = fmt.Fprintf(w, "%s%s%s  %s  %s  %s  %s\n",
			prefix,
			connector,
			cs.Bold(item.ID),
			cs.Dim(item.Role.String()),
			cmdutil.FormatState(cs, item.State, item.SecondaryState),
			cs.Yellow(item.Priority.String()),
			item.Title)

		// Recurse into children with adjusted prefix.
		childPrefix := prefix + "│   "
		if isLast {
			childPrefix = prefix + "    "
		}
		renderTree(w, cs, childrenOf, item.ID, childPrefix)
	}
}

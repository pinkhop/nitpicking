package relcmd

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// newParentCmd constructs the "rel parent" parent command with detach,
// children, and tree subcommands for managing parent-child hierarchy.
func newParentCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "parent",
		Usage: "Manage parent-child hierarchy",
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
		Name:      "children",
		Usage:     "List direct children of an issue",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
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

			result, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{ParentID: issueID},
				OrderBy: port.OrderByPriority,
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
					cs.Bold(item.ID.String()),
					cs.Dim(item.Role.String()),
					item.DisplayStatus(),
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
		Name:      "tree",
		Usage:     "Show the full descendant hierarchy of an issue",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
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
			rootShown, err := svc.ShowIssue(ctx, issueID)
			if err != nil {
				return fmt.Errorf("looking up root issue: %w", err)
			}

			result, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{DescendantsOf: issueID},
				OrderBy: port.OrderByPriority,
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
			root := rootShown.Issue
			_, _ = fmt.Fprintf(w, "%s  %s  %s  %s  %s\n",
				cs.Bold(root.ID().String()),
				cs.Dim(root.Role().String()),
				root.State().String(),
				cs.Yellow(root.Priority().String()),
				root.Title())

			if len(result.Items) == 0 {
				return nil
			}

			// Build parent→children index for tree rendering.
			childrenOf := make(map[string][]port.IssueListItem)
			for _, item := range result.Items {
				parentKey := item.ParentID.String()
				childrenOf[parentKey] = append(childrenOf[parentKey], item)
			}

			// Render tree recursively from root.
			renderTree(w, cs, childrenOf, issueID.String(), "")

			return nil
		},
	}
}

// renderTree recursively renders children of the given parentID with
// filesystem-style tree connectors (├── and └──).
func renderTree(w io.Writer, cs *iostreams.ColorScheme, childrenOf map[string][]port.IssueListItem, parentID, prefix string) {
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
			cs.Bold(item.ID.String()),
			cs.Dim(item.Role.String()),
			item.DisplayStatus(),
			cs.Yellow(item.Priority.String()),
			item.Title)

		// Recurse into children with adjusted prefix.
		childPrefix := prefix + "│   "
		if isLast {
			childPrefix = prefix + "    "
		}
		renderTree(w, cs, childrenOf, item.ID.String(), childPrefix)
	}
}

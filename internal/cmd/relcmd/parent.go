package relcmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// newParentCmd constructs the "rel parent" parent command with detach,
// children, and tree subcommands for managing parent-child hierarchy.
func newParentCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "parent",
		Usage: "Manage parent-child hierarchy",
		Commands: []*cli.Command{
			newDetachCmd(f),
			newChildrenCmd(f),
			newParentTreeCmd(f),
		},
	}
}

// newDetachCmd constructs "rel parent detach" which removes the parent
// assignment from a claimed issue.
func newDetachCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		claimID    string
	)

	return &cli.Command{
		Name:  "detach",
		Usage: "Remove the parent from a claimed issue",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"i"},
				Usage:       "Issue ID",
				Required:    true,
				Destination: &issueArg,
			},
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue",
				Required:    true,
				Destination: &claimID,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, issueArg)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			zeroID := issue.ID{}
			input := service.UpdateIssueInput{
				IssueID:  issueID,
				ClaimID:  claimID,
				ParentID: &zeroID,
			}
			if err := svc.UpdateIssue(ctx, input); err != nil {
				return fmt.Errorf("detaching parent: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID.String(),
					"action":   "detached",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Detached %s from parent\n",
				cs.SuccessIcon(), cs.Bold(issueID.String()))
			return err
		},
	}
}

// newChildrenCmd constructs "rel parent children" which lists direct children
// of an epic.
func newChildrenCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
	)

	return &cli.Command{
		Name:  "children",
		Usage: "List direct children of an issue",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"i"},
				Usage:       "Parent issue ID",
				Required:    true,
				Destination: &issueArg,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, issueArg)
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
				return cmdutil.WriteJSON(f.IOStreams.Out, result)
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
					item.State.String(),
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

// newParentTreeCmd constructs "rel parent tree" which shows the full descendant
// hierarchy from a given issue.
func newParentTreeCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
	)

	return &cli.Command{
		Name:  "tree",
		Usage: "Show the full descendant hierarchy of an issue",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"i"},
				Usage:       "Root issue ID",
				Required:    true,
				Destination: &issueArg,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, issueArg)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			result, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{DescendantsOf: issueID},
				OrderBy: port.OrderByPriority,
			})
			if err != nil {
				return fmt.Errorf("listing descendants: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, result)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "No descendants found.")
				return nil
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, item := range result.Items {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					cs.Bold(item.ID.String()),
					cs.Dim(item.Role.String()),
					item.State.String(),
					cs.Yellow(item.Priority.String()),
					item.Title)
			}
			_ = tw.Flush()

			shown := len(result.Items)
			if result.HasMore {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d descendants (more available)", shown)))
			} else {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d descendants", shown)))
			}
			return nil
		},
	}
}

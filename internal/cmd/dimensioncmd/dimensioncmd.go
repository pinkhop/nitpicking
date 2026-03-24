// Package dimensioncmd provides the "dimension" parent command, which groups
// dimension management operations under a single namespace.
package dimensioncmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// NewCmd constructs the "dimension" parent command with subcommands for
// managing issue dimensions.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:    "dimension",
		Aliases: []string{"dim"},
		Usage:   "Manage issue dimensions (key-value metadata)",
		Commands: []*cli.Command{
			newAddCmd(f),
			newRemoveCmd(f),
			newListCmd(f),
			newListAllCmd(f),
			newPropagateCmd(f),
		},
	}
}

// newAddCmd constructs "dimension add" which sets a dimension on a claimed
// issue.
func newAddCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		claimID    string
		key        string
		value      string
	)

	return &cli.Command{
		Name:  "add",
		Usage: "Set a dimension on a claimed issue",
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
			&cli.StringFlag{
				Name:        "key",
				Aliases:     []string{"k"},
				Usage:       "Dimension key",
				Required:    true,
				Destination: &key,
			},
			&cli.StringFlag{
				Name:        "value",
				Aliases:     []string{"v"},
				Usage:       "Dimension value",
				Required:    true,
				Destination: &value,
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

			dim, err := issue.NewDimension(key, value)
			if err != nil {
				return cmdutil.FlagErrorf("invalid dimension: %s", err)
			}

			input := service.UpdateIssueInput{
				IssueID:      issueID,
				ClaimID:      claimID,
				DimensionSet: []issue.Dimension{dim},
			}
			if err := svc.UpdateIssue(ctx, input); err != nil {
				return fmt.Errorf("setting dimension: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID.String(),
					"key":      key,
					"value":    value,
					"action":   "set",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Set %s=%s on %s\n",
				cs.SuccessIcon(), key, value, cs.Bold(issueID.String()))
			return err
		},
	}
}

// newRemoveCmd constructs "dimension remove" which removes a dimension from a
// claimed issue.
func newRemoveCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		claimID    string
		key        string
	)

	return &cli.Command{
		Name:  "remove",
		Usage: "Remove a dimension from a claimed issue",
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
			&cli.StringFlag{
				Name:        "key",
				Aliases:     []string{"k"},
				Usage:       "Dimension key to remove",
				Required:    true,
				Destination: &key,
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

			input := service.UpdateIssueInput{
				IssueID:         issueID,
				ClaimID:         claimID,
				DimensionRemove: []string{key},
			}
			if err := svc.UpdateIssue(ctx, input); err != nil {
				return fmt.Errorf("removing dimension: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID.String(),
					"key":      key,
					"action":   "removed",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Removed %s from %s\n",
				cs.SuccessIcon(), key, cs.Bold(issueID.String()))
			return err
		},
	}
}

// newListCmd constructs "dimension list" which shows dimensions for a specific
// issue.
func newListCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
	)

	return &cli.Command{
		Name:  "list",
		Usage: "List dimensions for an issue",
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

			shown, err := svc.ShowIssue(ctx, issueID)
			if err != nil {
				return fmt.Errorf("looking up issue: %w", err)
			}

			dims := shown.Issue.Dimensions()

			if jsonOutput {
				type dimJSON struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				}
				var out []dimJSON
				for k, v := range dims.All() {
					out = append(out, dimJSON{Key: k, Value: v})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"issue_id":   issueID.String(),
					"dimensions": out,
				})
			}

			w := f.IOStreams.Out
			if dims.Len() == 0 {
				_, _ = fmt.Fprintln(w, "No dimensions set.")
				return nil
			}

			for k, v := range dims.All() {
				_, _ = fmt.Fprintf(w, "%s: %s\n", k, v)
			}
			return nil
		},
	}
}

// newListAllCmd constructs "dimension list-all" which shows all unique
// dimension key-value pairs across all issues.
func newListAllCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "list-all",
		Usage: "List all unique dimensions across all issues",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			dims, err := svc.ListDistinctDimensions(ctx)
			if err != nil {
				return fmt.Errorf("listing dimensions: %w", err)
			}

			if jsonOutput {
				type dimJSON struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				}
				out := make([]dimJSON, 0, len(dims))
				for _, d := range dims {
					out = append(out, dimJSON{Key: d.Key(), Value: d.Value()})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"dimensions": out,
					"count":      len(dims),
				})
			}

			w := f.IOStreams.Out
			if len(dims) == 0 {
				_, _ = fmt.Fprintln(w, "No dimensions found.")
				return nil
			}

			// Group by key for readability.
			groups := make(map[string][]string)
			var keys []string
			for _, d := range dims {
				if _, exists := groups[d.Key()]; !exists {
					keys = append(keys, d.Key())
				}
				groups[d.Key()] = append(groups[d.Key()], d.Value())
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, k := range keys {
				_, _ = fmt.Fprintf(tw, "%s\t%s\n", k, strings.Join(groups[k], ", "))
			}
			_ = tw.Flush()

			return nil
		},
	}
}

// newPropagateCmd constructs "dimension propagate" which copies a dimension
// from a parent issue to all its descendants that lack it.
func newPropagateCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		author     string
		key        string
	)

	return &cli.Command{
		Name:  "propagate",
		Usage: "Propagate a dimension from a parent to all descendants",
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
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (for claiming descendants)",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "key",
				Aliases:     []string{"k"},
				Usage:       "Dimension key to propagate",
				Required:    true,
				Destination: &key,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, issueArg)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			// Get the dimension value from the parent.
			parent, err := svc.ShowIssue(ctx, issueID)
			if err != nil {
				return fmt.Errorf("looking up parent issue: %w", err)
			}
			value, exists := parent.Issue.Dimensions().Get(key)
			if !exists {
				return fmt.Errorf("issue %s does not have dimension %q", issueID, key)
			}

			dim, err := issue.NewDimension(key, value)
			if err != nil {
				return fmt.Errorf("invalid dimension: %w", err)
			}

			// List all descendants (unlimited).
			descendants, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{DescendantsOf: issueID},
				OrderBy: port.OrderByPriority,
				Limit:   -1,
			})
			if err != nil {
				return fmt.Errorf("listing descendants: %w", err)
			}

			// Propagate to each descendant that lacks the dimension.
			var propagated int
			for _, item := range descendants.Items {
				child, showErr := svc.ShowIssue(ctx, item.ID)
				if showErr != nil {
					continue
				}
				existingVal, hasDim := child.Issue.Dimensions().Get(key)
				if hasDim && existingVal == value {
					continue
				}

				// Use one-shot update to set the dimension.
				editErr := svc.OneShotUpdate(ctx, service.OneShotUpdateInput{
					IssueID:      item.ID,
					Author:       parsedAuthor,
					DimensionSet: []issue.Dimension{dim},
				})
				if editErr != nil {
					continue
				}
				propagated++
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"issue_id":   issueID.String(),
					"key":        key,
					"value":      value,
					"propagated": propagated,
					"total":      len(descendants.Items),
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Propagated %s=%s to %d of %d descendants\n",
				cs.SuccessIcon(), key, value, propagated, len(descendants.Items))
			return err
		},
	}
}

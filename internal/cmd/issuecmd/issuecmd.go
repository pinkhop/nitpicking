// Package issuecmd provides the "issue" parent command, which groups issue
// management operations under a single namespace. Where possible, subcommands
// delegate to existing command implementations to avoid duplication.
package issuecmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmd/done"
	"github.com/pinkhop/nitpicking/internal/cmd/list"
	"github.com/pinkhop/nitpicking/internal/cmd/search"
	"github.com/pinkhop/nitpicking/internal/cmd/update"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"

	cmddelete "github.com/pinkhop/nitpicking/internal/cmd/delete"
)

// NewCmd constructs the "issue" parent command with all issue management
// subcommands. Some subcommands wrap existing top-level commands (list,
// search, update, delete); others are new (close, reopen, defer, note,
// orphans).
func NewCmd(f *cmdutil.Factory) *cli.Command {
	queryCmd := search.NewCmd(f)
	queryCmd.Name = "query"
	queryCmd.Aliases = []string{"search", "q"}
	queryCmd.Usage = "Search issues by text query"

	return &cli.Command{
		Name:    "issue",
		Aliases: []string{"i"},
		Usage:   "Issue management commands",
		Commands: []*cli.Command{
			list.NewCmd(f),
			queryCmd,
			update.NewCmd(f),
			newEditCmd(f),
			newCloseCmd(f),
			newReleaseCmd(f),
			newReopenCmd(f),
			newUndeferCmd(f),
			newDeferCmd(f),
			cmddelete.NewCmd(f),
			newNoteCmd(f),
			newOrphansCmd(f),
		},
	}
}

// newCloseCmd constructs the "issue close" subcommand, which closes an issue
// with a required reason. Delegates to the done.Run function.
func newCloseCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
		author     string
		reason     string
	)

	return &cli.Command{
		Name:      "close",
		Usage:     "Close a claimed issue with a required reason",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Destination: &claimID,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name for the closing comment (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "reason",
				Aliases:     []string{"r"},
				Usage:       "Reason for closing (added as a comment) (required)",
				Required:    true,
				Destination: &reason,
			},
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

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
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

			return done.Run(ctx, done.RunInput{
				Service: svc,
				IssueID: issueID,
				ClaimID: claimID,
				Author:  parsedAuthor,
				Reason:  reason,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

// newReopenCmd constructs the "issue reopen" subcommand, which transitions
// closed issues back to open. Supports multiple issue IDs in one invocation.
func newReopenCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "reopen",
		Usage:     "Reopen closed issues (transition back to open)",
		ArgsUsage: "<ISSUE-ID> [ISSUE-ID...]",
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
			if cmd.NArg() == 0 {
				return cmdutil.FlagErrorf("at least one issue ID argument is required")
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

			var lastErr error
			for i := range cmd.NArg() {
				rawID := cmd.Args().Get(i)
				issueID, resolveErr := resolver.Resolve(ctx, rawID)
				if resolveErr != nil {
					lastErr = fmt.Errorf("invalid issue ID %q: %w", rawID, resolveErr)
					_, _ = fmt.Fprintf(f.IOStreams.ErrOut, "Error: %v\n", lastErr)
					continue
				}
				reopenErr := Reopen(ctx, ReopenInput{
					Service: svc,
					IssueID: issueID,
					Author:  parsedAuthor,
					JSON:    jsonOutput,
					WriteTo: f.IOStreams.Out,
				})
				if reopenErr != nil {
					lastErr = reopenErr
					_, _ = fmt.Fprintf(f.IOStreams.ErrOut, "Error reopening %s: %v\n", issueID, reopenErr)
				}
			}
			return lastErr
		},
	}
}

// newUndeferCmd constructs the "issue undefer" subcommand, which transitions
// deferred issues back to open. Supports multiple issue IDs in one invocation.
func newUndeferCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "undefer",
		Usage:     "Restore deferred issues (transition back to open)",
		ArgsUsage: "<ISSUE-ID> [ISSUE-ID...]",
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
			if cmd.NArg() == 0 {
				return cmdutil.FlagErrorf("at least one issue ID argument is required")
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

			var lastErr error
			for i := range cmd.NArg() {
				rawID := cmd.Args().Get(i)
				issueID, resolveErr := resolver.Resolve(ctx, rawID)
				if resolveErr != nil {
					lastErr = fmt.Errorf("invalid issue ID %q: %w", rawID, resolveErr)
					_, _ = fmt.Fprintf(f.IOStreams.ErrOut, "Error: %v\n", lastErr)
					continue
				}
				reopenErr := Reopen(ctx, ReopenInput{
					Service: svc,
					IssueID: issueID,
					Author:  parsedAuthor,
					JSON:    jsonOutput,
					WriteTo: f.IOStreams.Out,
				})
				if reopenErr != nil {
					lastErr = reopenErr
					_, _ = fmt.Fprintf(f.IOStreams.ErrOut, "Error undeferring %s: %v\n", issueID, reopenErr)
				}
			}
			return lastErr
		},
	}
}

// newDeferCmd constructs the "issue defer" subcommand, which defers a claimed
// issue for later work.
func newDeferCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
	)

	return &cli.Command{
		Name:      "defer",
		Usage:     "Defer a claimed issue for later",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Destination: &claimID,
			},
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

			input := service.TransitionInput{
				IssueID: issueID,
				ClaimID: claimID,
				Action:  service.ActionDefer,
			}
			if err := svc.TransitionState(ctx, input); err != nil {
				return fmt.Errorf("deferring issue: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID.String(),
					"action":   "defer",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Deferred %s\n",
				cs.SuccessIcon(), cs.Bold(issueID.String()))
			return err
		},
	}
}

// newReleaseCmd constructs the "issue release" subcommand, which returns a
// claimed issue to its default unclaimed state without closing it.
func newReleaseCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
	)

	return &cli.Command{
		Name:      "release",
		Usage:     "Release a claimed issue without closing",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Destination: &claimID,
			},
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

			input := service.TransitionInput{
				IssueID: issueID,
				ClaimID: claimID,
				Action:  service.ActionRelease,
			}
			if err := svc.TransitionState(ctx, input); err != nil {
				return fmt.Errorf("releasing issue: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID.String(),
					"action":   "release",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Released %s\n",
				cs.SuccessIcon(), cs.Bold(issueID.String()))
			return err
		},
	}
}

// newEditCmd constructs the "issue edit" subcommand, which performs an atomic
// claim-update-release on an issue without requiring a separate claim step.
func newEditCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput         bool
		author             string
		title              string
		description        string
		acceptanceCriteria string
		priority           string
		parent             string
	)

	return &cli.Command{
		Name:      "edit",
		Usage:     "Atomically claim, update, and release an issue",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name for the one-shot claim (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "New title",
				Category:    "Options",
				Destination: &title,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "New description",
				Category:    "Options",
				Destination: &description,
			},
			&cli.StringFlag{
				Name:        "acceptance-criteria",
				Usage:       "New acceptance criteria",
				Category:    "Options",
				Destination: &acceptanceCriteria,
			},
			&cli.StringFlag{
				Name:        "priority",
				Aliases:     []string{"p"},
				Usage:       "New priority: P0–P4",
				Category:    "Options",
				Destination: &priority,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "New parent epic ID (empty string to remove parent)",
				Category:    "Options",
				Destination: &parent,
			},
			&cli.StringSliceFlag{
				Name:     "dimension",
				Usage:    "Set a dimension in key:value format (repeatable)",
				Category: "Options",
			},
			&cli.StringSliceFlag{
				Name:     "dimension-remove",
				Usage:    "Remove a dimension by key (repeatable)",
				Category: "Options",
			},
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

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
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

			input := service.OneShotUpdateInput{
				IssueID:         issueID,
				Author:          parsedAuthor,
				DimensionRemove: cmd.StringSlice("dimension-remove"),
			}

			if cmd.IsSet("title") {
				input.Title = &title
			}
			if cmd.IsSet("description") {
				input.Description = &description
			}
			if cmd.IsSet("acceptance-criteria") {
				input.AcceptanceCriteria = &acceptanceCriteria
			}
			if cmd.IsSet("priority") {
				p, parseErr := issue.ParsePriority(priority)
				if parseErr != nil {
					return cmdutil.FlagErrorf("%s", parseErr)
				}
				input.Priority = &p
			}
			if cmd.IsSet("parent") {
				if parent == "" {
					zeroID := issue.ID{}
					input.ParentID = &zeroID
				} else {
					pid, resolveErr := resolver.Resolve(ctx, parent)
					if resolveErr != nil {
						return cmdutil.FlagErrorf("invalid parent ID: %s", resolveErr)
					}
					input.ParentID = &pid
				}
			}

			rawDimensionSet := cmd.StringSlice("dimension")
			for _, s := range rawDimensionSet {
				key, value, ok := strings.Cut(s, ":")
				if !ok {
					return cmdutil.FlagErrorf("invalid dimension %q: must be in key:value format", s)
				}
				dimension, dimErr := issue.NewDimension(key, value)
				if dimErr != nil {
					return cmdutil.FlagErrorf("invalid dimension %q: %s", s, dimErr)
				}
				input.DimensionSet = append(input.DimensionSet, dimension)
			}

			if err := svc.OneShotUpdate(ctx, input); err != nil {
				return fmt.Errorf("editing issue: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID.String(),
					"action":   "edited",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Edited %s\n",
				cs.SuccessIcon(), cs.Bold(issueID.String()))
			return err
		},
	}
}

// newNoteCmd constructs the "issue note" subcommand, which adds a comment to
// an issue. A simplified wrapper around "comment add" with the issue ID as a
// positional argument.
func newNoteCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
		body       string
	)

	return &cli.Command{
		Name:      "note",
		Aliases:   []string{"comment"},
		Usage:     "Add a note (comment) to an issue",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "body",
				Aliases:     []string{"b", "m"},
				Usage:       "Note body text (required)",
				Required:    true,
				Destination: &body,
			},
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

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
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

			result, err := svc.AddComment(ctx, service.AddCommentInput{
				IssueID: issueID,
				Author:  parsedAuthor,
				Body:    body,
			})
			if err != nil {
				return fmt.Errorf("adding note: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"comment_id": result.Comment.DisplayID(),
					"issue_id":   issueID.String(),
					"author":     author,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Added %s to %s\n",
				cs.SuccessIcon(),
				cs.Bold(result.Comment.DisplayID()),
				cs.Bold(issueID.String()))
			return err
		},
	}
}

// newOrphansCmd constructs the "issue orphans" subcommand, which lists issues
// that have no parent epic.
func newOrphansCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		limit      int
	)

	return &cli.Command{
		Name:  "orphans",
		Usage: "List issues that have no parent epic",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results (0 = default, negative = unlimited)",
				Category:    "Options",
				Destination: &limit,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			result, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{Orphan: true, ExcludeClosed: true},
				OrderBy: port.OrderByPriority,
				Limit:   limit,
			})
			if err != nil {
				return fmt.Errorf("listing orphan issues: %w", err)
			}

			if jsonOutput {
				type orphanItem struct {
					ID       string `json:"id"`
					Role     string `json:"role"`
					State    string `json:"state"`
					Priority string `json:"priority"`
					Title    string `json:"title"`
				}
				type orphanOutput struct {
					Items   []orphanItem `json:"items"`
					HasMore bool         `json:"has_more"`
				}
				out := orphanOutput{
					HasMore: result.HasMore,
					Items:   make([]orphanItem, 0, len(result.Items)),
				}
				for _, item := range result.Items {
					out.Items = append(out.Items, orphanItem{
						ID:       item.ID.String(),
						Role:     item.Role.String(),
						State:    item.State.String(),
						Priority: item.Priority.String(),
						Title:    item.Title,
					})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "No orphan issues found.")
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
					cs.Dim(fmt.Sprintf("%d orphan issues (more available)", shown)))
			} else {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d orphan issues", shown)))
			}

			return nil
		},
	}
}

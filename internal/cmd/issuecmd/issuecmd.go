// Package issuecmd provides the "issue" parent command, which groups issue
// management operations under a single namespace. Core workflow commands
// (list, close) live at root; this package holds issue-specific operations
// that are not part of the core create-claim-work-close loop.
package issuecmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/historyview"
	"github.com/pinkhop/nitpicking/internal/cmd/search"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"

	cmddelete "github.com/pinkhop/nitpicking/internal/cmd/delete"
)

// NewCmd constructs the "issue" parent command with all issue management
// subcommands. Core workflow commands (list, close) are only available at
// root; search and other issue-specific operations live here.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:    "issue",
		Aliases: []string{"i"},
		Usage:   "Issue management commands",
		Description: `Groups issue lifecycle operations that fall outside the core
create-claim-work-close loop. Use these subcommands to search for issues,
inspect mutation history, reopen closed issues, defer or undefer work, release
claims, delete issues, and find orphans (issues with no parent epic).

Most day-to-day work flows through "np create", "np claim", and "np close".
Reach for "np issue" when you need to manage issue state transitions that those
core commands do not cover, or when you need to query and inspect the issue
database in ways that "np list" and "np show" do not support.`,
		Commands: []*cli.Command{
			search.NewCmd(f),
			newReleaseCmd(f),
			newReopenCmd(f),
			newUndeferCmd(f),
			newDeferCmd(f),
			cmddelete.NewCmd(f),
			historyview.NewCmd(f),
			newOrphansCmd(f),
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
		Description: `Transitions one or more closed issues back to the open state. Use this
when a closed issue needs further work — for example, when a bug fix turns out
to be incomplete or when acceptance criteria were not fully met.

Reopened issues return to the open state and become eligible for claiming again.
Multiple issue IDs can be specified in a single invocation; each is processed
independently, and errors on individual issues do not prevent the others from
being reopened.`,
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
			if cmd.NArg() == 0 {
				return cmdutil.FlagErrorf("at least one issue ID argument is required")
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
					IssueID: issueID.String(),
					Author:  author,
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
		Description: `Transitions one or more deferred issues back to the open state, making
them eligible for claiming again. Use this when previously shelved work is ready
to be picked up — for example, when a dependency has been resolved or a deferred
task's revisit date has arrived.

Multiple issue IDs can be specified in a single invocation; each is processed
independently. If any individual issue cannot be undeferred (e.g., it is not in
the deferred state), the error is reported but processing continues for the
remaining issues.`,
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
			if cmd.NArg() == 0 {
				return cmdutil.FlagErrorf("at least one issue ID argument is required")
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
					IssueID: issueID.String(),
					Author:  author,
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
// issue for later work. An optional --until flag records a revisit date as a
// label (informational only — np has no scheduler, but doctor can report
// overdue deferrals).
func newDeferCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
		until      string
	)

	return &cli.Command{
		Name:  "defer",
		Usage: "Defer a claimed issue for later",
		Description: `Shelves a claimed issue for later work by transitioning it to the
deferred state. Use this when you have claimed an issue but cannot complete it
now — for example, because it depends on work that has not been done yet, or
because higher-priority work has come in.

The optional --until flag records a target revisit date as a label on the issue.
This date is informational only (np has no scheduler), but "np admin doctor" can
report overdue deferrals. The claim is released as part of the deferral, so the
issue is no longer held by any agent.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &claimID,
			},
			&cli.StringFlag{
				Name:        "until",
				Usage:       "Date to revisit (YYYY-MM-DD); recorded as defer-until label",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &until,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			issueID, err := svc.LookupClaimIssueID(ctx, claimID)
			if err != nil {
				return fmt.Errorf("looking up claim: %w", err)
			}

			return Defer(ctx, DeferInput{
				Service: svc,
				IssueID: issueID,
				ClaimID: claimID,
				Until:   until,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
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
		Name:  "release",
		Usage: "Release a claimed issue without closing",
		Description: `Returns a claimed issue to the open state without closing it. Use this
when you have finished your work on an issue but the issue itself is not done —
for example, after decomposing an epic into child tasks, or when you need to
hand off a task to another agent.

The claim is released and the issue becomes eligible for claiming by any agent.
This is the normal way to finish working on an epic after decomposition.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
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
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			issueID, err := svc.LookupClaimIssueID(ctx, claimID)
			if err != nil {
				return fmt.Errorf("looking up claim: %w", err)
			}

			return Release(ctx, ReleaseInput{
				Service: svc,
				IssueID: issueID,
				ClaimID: claimID,
				JSON:    jsonOutput,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

// newOrphansCmd constructs the "issue orphans" subcommand, which lists issues
// that have no parent epic.
func newOrphansCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		limit      int
		noLimit    bool
	)

	return &cli.Command{
		Name:  "orphans",
		Usage: "List issues that have no parent epic",
		Description: `Lists all open issues that have no parent epic. Orphan issues exist
outside the epic hierarchy and may represent work that was created ad hoc, or
issues whose parent was deleted or whose parent relationship was removed.

Use this command to audit the issue database for organizational hygiene — for
example, to find tasks that should be attached to an epic, or to identify stray
issues that were created during triage but never organized. Results are sorted
by priority and exclude closed issues.`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results",
				Value:       cmdutil.DefaultLimit,
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &limit,
			},
			&cli.BoolFlag{
				Name:        "no-limit",
				Usage:       "Return all matching results",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &noLimit,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			effectiveLimit, err := cmdutil.ResolveLimit(limit, noLimit)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
				Filter:  driving.IssueFilterInput{Orphan: true, ExcludeClosed: true},
				OrderBy: driving.OrderByPriority,
				Limit:   effectiveLimit,
			})
			if err != nil {
				return fmt.Errorf("listing orphan issues: %w", err)
			}

			if jsonOutput {
				type orphanItem struct {
					ID            string `json:"id"`
					Role          string `json:"role"`
					State         string `json:"state"`
					DisplayStatus string `json:"display_status"`
					Priority      string `json:"priority"`
					Title         string `json:"title"`
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
						ID:            item.ID,
						Role:          item.Role.String(),
						State:         item.State.String(),
						DisplayStatus: item.DisplayStatus,
						Priority:      item.Priority.String(),
						Title:         item.Title,
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
					cs.Dim(fmt.Sprintf("%d orphan issues (more available)", shown)))
			} else {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d orphan issues", shown)))
			}

			return nil
		},
	}
}

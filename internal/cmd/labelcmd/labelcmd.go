// Package labelcmd provides the "label" parent command, which groups
// label management operations under a single namespace.
package labelcmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// NewCmd constructs the "label" parent command with subcommands for managing
// issue labels (key-value pairs attached to issues).
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:    "label",
		Aliases: []string{"l"},
		Usage:   "Manage issue labels",
		Description: `Labels are free-form key:value pairs attached to issues. They serve as a
lightweight taxonomy for filtering, searching, and organizing work — common
keys include "kind" (bug, feature, refactor), "area" (cli, domain, storage),
and "priority-override" for ad-hoc triage.

Use the subcommands to add or remove labels on claimed issues, list labels
for a single issue or across the entire tracker, and propagate a label from
a parent issue to all its descendants. Labels do not affect issue state or
readiness; they are purely metadata for human and agent consumption.`,
		Commands: []*cli.Command{
			newAddCmd(f),
			newRemoveCmd(f),
			newListCmd(f),
			newListAllCmd(f),
			newPropagateCmd(f),
		},
	}
}

// resolveIssueFromClaim looks up the issue ID associated with an active claim.
// The caller provides only the claim ID; the issue identity is derived from
// the claim record.
func resolveIssueFromClaim(ctx context.Context, svc driving.Service, claimID string) (string, error) {
	issueID, err := svc.LookupClaimIssueID(ctx, claimID)
	if err != nil {
		return "", fmt.Errorf("looking up claim: %w", err)
	}
	return issueID, nil
}

// newAddCmd constructs "label add" which sets a label on a claimed issue.
// Takes a positional key:value argument; parses on the first colon so values
// may contain colons.
func newAddCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
	)

	return &cli.Command{
		Name:  "add",
		Usage: "Set a label on a claimed issue",
		Description: `Sets a label on an issue you currently hold a claim for. The label is
specified as a positional argument in key:value format — the split happens
on the first colon, so values may contain colons (e.g., "url:https://example.com").
If the key already exists, its value is overwritten.

Use this command during the implementation phase of a claimed issue to tag it
with metadata such as "kind:bug", "area:cli", or "defer-reason:confusion".
Labels are also used by "np list" and "np comment search" filters, so
consistent labeling improves discoverability for future agents and humans.`,
		ArgsUsage: "<key:value>",
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
			raw := cmd.Args().Get(0)
			if raw == "" {
				return cmdutil.FlagErrorf("label argument is required (key:value)")
			}
			key, value, ok := strings.Cut(raw, ":")
			if !ok {
				return cmdutil.FlagErrorf("label must be in key:value format, got %q", raw)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			issueID, err := resolveIssueFromClaim(ctx, svc, claimID)
			if err != nil {
				return err
			}

			input := driving.UpdateIssueInput{
				IssueID:  issueID,
				ClaimID:  claimID,
				LabelSet: []driving.LabelInput{{Key: key, Value: value}},
			}
			if err := svc.UpdateIssue(ctx, input); err != nil {
				return fmt.Errorf("setting label: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID,
					"key":      key,
					"value":    value,
					"action":   "set",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Set %s=%s on %s\n",
				cs.SuccessIcon(), key, value, cs.Bold(issueID))
			return err
		},
	}
}

// newRemoveCmd constructs "label remove" which removes a label from a
// claimed issue.
func newRemoveCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
	)

	return &cli.Command{
		Name:  "remove",
		Usage: "Remove a label from a claimed issue",
		Description: `Removes a label from an issue you currently hold a claim for. Pass the
label key as a positional argument; the entire key-value pair is deleted.
If the key does not exist on the issue, the command succeeds silently.

Use this command to clean up labels that were added in error, are no longer
relevant, or need to be replaced with a different value (remove the old key
first, then "label add" the new one). Like "label add", this command
requires an active claim.`,
		ArgsUsage: "<key>",
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
			key := cmd.Args().Get(0)
			if key == "" {
				return cmdutil.FlagErrorf("label key argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			issueID, err := resolveIssueFromClaim(ctx, svc, claimID)
			if err != nil {
				return err
			}

			input := driving.UpdateIssueInput{
				IssueID:     issueID,
				ClaimID:     claimID,
				LabelRemove: []string{key},
			}
			if err := svc.UpdateIssue(ctx, input); err != nil {
				return fmt.Errorf("removing label: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"issue_id": issueID,
					"key":      key,
					"action":   "removed",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Removed %s from %s\n",
				cs.SuccessIcon(), key, cs.Bold(issueID))
			return err
		},
	}
}

// newListCmd constructs "label list" which shows labels for a specific issue.
// The issue ID is a positional argument, consistent with other single-issue
// commands that have no competing positional arguments.
func newListCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "list",
		Usage: "List labels for an issue",
		Description: `Shows all labels currently attached to the specified issue. Each label is
displayed as a key-value pair. This command does not require a claim — any
user or agent can inspect labels on any issue.

Use this command to check what metadata is already present before adding or
removing labels, or to verify that a "label add" or "label propagate"
operation had the expected effect.`,
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

			labels := shown.Labels

			if jsonOutput {
				type labelJSON struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				}
				var out []labelJSON
				for k, v := range labels {
					out = append(out, labelJSON{Key: k, Value: v})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"issue_id": issueID.String(),
					"labels":   out,
				})
			}

			w := f.IOStreams.Out
			if len(labels) == 0 {
				_, _ = fmt.Fprintln(w, "No labels set.")
				return nil
			}

			for k, v := range labels {
				_, _ = fmt.Fprintf(w, "%s: %s\n", k, v)
			}
			return nil
		},
	}
}

// newListAllCmd constructs "label list-all" which shows all unique
// label key-value pairs across all issues.
func newListAllCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "list-all",
		Usage: "List all unique labels across all issues",
		Description: `Shows every distinct label key-value pair that exists across all issues in the
tracker, grouped by key. This gives a bird's-eye view of the labeling
taxonomy without requiring you to inspect issues one at a time.

Use this command to discover what label keys and values are in use — for
example, to check whether a "kind:refactor" label exists before using it,
or to audit labeling consistency across the project. The output is read-only
and does not require a claim.`,
		Flags: []cli.Flag{
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

			labels, err := svc.ListDistinctLabels(ctx)
			if err != nil {
				return fmt.Errorf("listing labels: %w", err)
			}

			if jsonOutput {
				type labelJSON struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				}
				out := make([]labelJSON, 0, len(labels))
				for _, l := range labels {
					out = append(out, labelJSON{Key: l.Key, Value: l.Value})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"labels": out,
					"count":  len(labels),
				})
			}

			w := f.IOStreams.Out
			if len(labels) == 0 {
				_, _ = fmt.Fprintln(w, "No labels found.")
				return nil
			}

			// Group by key for readability.
			groups := make(map[string][]string)
			var keys []string
			for _, l := range labels {
				if _, exists := groups[l.Key]; !exists {
					keys = append(keys, l.Key)
				}
				groups[l.Key] = append(groups[l.Key], l.Value)
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

// newPropagateCmd constructs "label propagate" which copies a label from a
// parent issue to all its descendants that lack it. The label key is a
// positional argument.
func newPropagateCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		author     string
	)

	return &cli.Command{
		Name:  "propagate",
		Usage: "Propagate a label from a parent to all descendants",
		Description: `Copies a label from a parent issue to every descendant in its subtree that
does not already have that label key set. The label key is specified as a
positional argument; the value is read from the parent issue. Descendants
that already carry the key (even with a different value) are skipped.

This is useful for bulk-tagging an entire epic tree — for example,
propagating "area:cli" from a top-level epic to all its children and
grandchildren. The command temporarily claims each descendant to apply the
label, so an --author flag is required. The claim is released immediately
after the label is set.`,
		ArgsUsage: "<key>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"i"},
				Usage:       "Parent issue ID (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &issueArg,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (for claiming descendants) (required)",
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
			key := cmd.Args().Get(0)
			if key == "" {
				return cmdutil.FlagErrorf("label key argument is required")
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

			out, err := svc.PropagateLabel(ctx, driving.PropagateLabelInput{
				IssueID: issueID.String(),
				Key:     key,
				Author:  author,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"issue_id":   issueID.String(),
					"key":        key,
					"value":      out.Value,
					"propagated": out.Propagated,
					"total":      out.Total,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Propagated %s=%s to %d of %d descendants\n",
				cs.SuccessIcon(), key, out.Value, out.Propagated, out.Total)
			return err
		},
	}
}

// Package relcmd provides the "rel" parent command, which groups relationship
// management operations under a single namespace. Subcommands are organized by
// relationship type (blocks, cites, parent) and include utilities for listing,
// tree views, and cycle detection.
package relcmd

import (
	"context"
	"fmt"
	"slices"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// FilterRelationships returns only the relationships whose type matches any of
// the given types. This is the shared filtering logic used by blocks list and
// cites list.
func FilterRelationships(rels []issue.Relationship, types ...issue.RelationType) []issue.Relationship {
	var result []issue.Relationship
	for _, r := range rels {
		if slices.Contains(types, r.Type()) {
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
		Commands: []*cli.Command{
			newBlocksCmd(f),
			newCitesCmd(f),
			newParentCmd(f),
			newListCmd(f),
			newTreeCmd(f),
			newCyclesCmd(f),
		},
	}
}

// newListCmd constructs "rel list" which shows all relationships for an issue.
func newListCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
	)

	return &cli.Command{
		Name:  "list",
		Usage: "List all relationships for an issue",
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

			return renderRelationships(f, shown.Relationships, issueID, jsonOutput)
		},
	}
}

// newTreeCmd constructs "rel tree" which shows the parent-child hierarchy
// from a given root issue.
func newTreeCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
	)

	return &cli.Command{
		Name:  "tree",
		Usage: "Show the hierarchy tree starting from an issue",
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

			// Show the root issue first.
			root, err := svc.ShowIssue(ctx, issueID)
			if err != nil {
				return fmt.Errorf("looking up root issue: %w", err)
			}
			_, _ = fmt.Fprintf(w, "%s %s (%s)\n",
				cs.Bold(issueID.String()),
				root.Issue.Title(),
				cs.Dim(root.Issue.State().String()))

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "  (no descendants)")
				return nil
			}

			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for _, item := range result.Items {
				_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n",
					cs.Bold(item.ID.String()),
					cs.Dim(item.Role.String()),
					item.State.String(),
					item.Title)
			}
			_ = tw.Flush()

			return nil
		},
	}
}

// newCyclesCmd constructs "rel cycles" which runs the doctor diagnostic and
// filters for cycle-related findings.
func newCyclesCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "cycles",
		Usage: "Detect relationship cycles",
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

			result, err := svc.Doctor(ctx)
			if err != nil {
				return fmt.Errorf("running diagnostics: %w", err)
			}

			// Filter for cycle-related findings.
			var cycleFindings []service.DoctorFinding
			for _, finding := range result.Findings {
				if finding.Category == "cycle" {
					cycleFindings = append(cycleFindings, finding)
				}
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"findings": cycleFindings,
					"count":    len(cycleFindings),
				})
			}

			w := f.IOStreams.Out
			if len(cycleFindings) == 0 {
				_, _ = fmt.Fprintln(w, "No cycles detected.")
				return nil
			}

			cs := f.IOStreams.ColorScheme()
			for _, finding := range cycleFindings {
				_, _ = fmt.Fprintf(w, "%s %s\n",
					cs.Red(finding.Severity+":"),
					finding.Message)
			}
			return nil
		},
	}
}

// renderRelationships renders a list of relationships to the output.
func renderRelationships(f *cmdutil.Factory, rels []issue.Relationship, issueID issue.ID, jsonOutput bool) error {
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
				SourceID: r.SourceID().String(),
				Type:     r.Type().String(),
				TargetID: r.TargetID().String(),
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
			cs.Bold(r.SourceID().String()),
			r.Type().String(),
			cs.Bold(r.TargetID().String()))
	}
	_ = tw.Flush()

	_, _ = fmt.Fprintf(w, "\n%s\n",
		cs.Dim(fmt.Sprintf("%d relationships", len(rels))))
	return nil
}

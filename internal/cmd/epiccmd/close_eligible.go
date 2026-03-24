package epiccmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// closeResult records the outcome of attempting to close a single epic.
type closeResult struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Closed  bool   `json:"closed"`
	Message string `json:"message,omitempty"`
}

// newCloseEligibleCmd constructs "epic close-eligible" which finds epics where
// all children are closed and batch-closes them. Each epic is claimed, a
// closing comment is added, and the epic is transitioned to closed.
func newCloseEligibleCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
		dryRun     bool
	)

	return &cli.Command{
		Name:  "close-eligible",
		Usage: "Close all epics whose children are fully resolved",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (for claiming and commenting) (required)",
				Required:    true,
				Destination: &author,
			},
			&cli.BoolFlag{
				Name:        "dry-run",
				Usage:       "List eligible epics without closing them",
				Category:    "Options",
				Destination: &dryRun,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
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

			// List all open epics.
			epicsResult, err := svc.ListIssues(ctx, service.ListIssuesInput{
				Filter:  port.IssueFilter{Role: issue.RoleEpic, ExcludeClosed: true},
				OrderBy: port.OrderByPriority,
				Limit:   -1,
			})
			if err != nil {
				return fmt.Errorf("listing epics: %w", err)
			}

			// Find eligible epics.
			var eligible []port.IssueListItem
			for _, epic := range epicsResult.Items {
				childResult, childErr := svc.ListIssues(ctx, service.ListIssuesInput{
					Filter: port.IssueFilter{ParentID: epic.ID},
					Limit:  -1,
				})
				if childErr != nil {
					continue
				}

				var childStatuses []issue.ChildStatus
				for _, child := range childResult.Items {
					childStatuses = append(childStatuses, issue.ChildStatus{State: child.State})
				}

				prog := ComputeProgress(childStatuses)
				if prog.Eligible {
					eligible = append(eligible, epic)
				}
			}

			if len(eligible) == 0 {
				if jsonOutput {
					return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
						"results": []closeResult{},
						"closed":  0,
					})
				}
				_, _ = fmt.Fprintln(f.IOStreams.Out, "No eligible epics found.")
				return nil
			}

			if dryRun {
				if jsonOutput {
					items := make([]closeResult, 0, len(eligible))
					for _, e := range eligible {
						items = append(items, closeResult{
							ID:      e.ID.String(),
							Title:   e.Title,
							Closed:  false,
							Message: "dry run",
						})
					}
					return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
						"results": items,
						"count":   len(items),
					})
				}
				cs := f.IOStreams.ColorScheme()
				_, _ = fmt.Fprintf(f.IOStreams.Out, "Would close %d eligible epics:\n", len(eligible))
				for _, e := range eligible {
					_, _ = fmt.Fprintf(f.IOStreams.Out, "  %s %s\n",
						cs.Bold(e.ID.String()), e.Title)
				}
				return nil
			}

			// Close each eligible epic.
			var results []closeResult
			closedCount := 0
			for _, epic := range eligible {
				result := closeEpic(ctx, svc, epic, parsedAuthor)
				results = append(results, result)
				if result.Closed {
					closedCount++
				}
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]any{
					"results": results,
					"closed":  closedCount,
				})
			}

			cs := f.IOStreams.ColorScheme()
			for _, r := range results {
				if r.Closed {
					_, _ = fmt.Fprintf(f.IOStreams.Out, "%s Closed %s %s\n",
						cs.SuccessIcon(), cs.Bold(r.ID), r.Title)
				} else {
					_, _ = fmt.Fprintf(f.IOStreams.Out, "%s Skipped %s: %s\n",
						cs.Yellow("!"), cs.Bold(r.ID), r.Message)
				}
			}
			_, _ = fmt.Fprintf(f.IOStreams.Out, "\n%d of %d eligible epics closed.\n",
				closedCount, len(eligible))

			return nil
		},
	}
}

// closeEpic claims an epic, adds a closing comment, and transitions it to
// closed. Returns a result indicating success or failure.
func closeEpic(ctx context.Context, svc service.Service, epic port.IssueListItem, author identity.Author) closeResult {
	// Claim the epic.
	claimOut, err := svc.ClaimByID(ctx, service.ClaimInput{
		IssueID: epic.ID,
		Author:  author,
	})
	if err != nil {
		return closeResult{
			ID:      epic.ID.String(),
			Title:   epic.Title,
			Closed:  false,
			Message: fmt.Sprintf("claim failed: %v", err),
		}
	}

	// Add a closing comment.
	_, err = svc.AddComment(ctx, service.AddCommentInput{
		IssueID: epic.ID,
		Author:  author,
		Body:    "All children are closed. Closing epic via batch close-eligible.",
	})
	if err != nil {
		// Release the claim on comment failure.
		_ = svc.TransitionState(ctx, service.TransitionInput{
			IssueID: epic.ID,
			ClaimID: claimOut.ClaimID,
			Action:  service.ActionRelease,
		})
		return closeResult{
			ID:      epic.ID.String(),
			Title:   epic.Title,
			Closed:  false,
			Message: fmt.Sprintf("comment failed: %v", err),
		}
	}

	// Close the epic.
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: epic.ID,
		ClaimID: claimOut.ClaimID,
		Action:  service.ActionClose,
	})
	if err != nil {
		return closeResult{
			ID:      epic.ID.String(),
			Title:   epic.Title,
			Closed:  false,
			Message: fmt.Sprintf("close failed: %v", err),
		}
	}

	return closeResult{
		ID:     epic.ID.String(),
		Title:  epic.Title,
		Closed: true,
	}
}

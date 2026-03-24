package historyview

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// fieldChangeOutput is the JSON representation of a single field change.
type fieldChangeOutput struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// historyEntryOutput is the JSON representation of a single history entry.
type historyEntryOutput struct {
	ID        int64               `json:"id"`
	Revision  int                 `json:"revision"`
	Author    string              `json:"author"`
	EventType string              `json:"event_type"`
	Timestamp string              `json:"timestamp"`
	Changes   []fieldChangeOutput `json:"changes,omitzero"`
}

// historyOutput is the JSON representation of the history command result.
type historyOutput struct {
	IssueID    string               `json:"issue_id"`
	Entries    []historyEntryOutput `json:"entries"`
	TotalCount int                  `json:"total_count"`
}

// NewCmd constructs the "history" command, which displays the mutation history
// of an issue.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		pageSize   int
	)

	return &cli.Command{
		Name:      "history",
		Usage:     "Show the mutation history of an issue",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.IntFlag{
				Name:        "page-size",
				Usage:       "Number of entries per page",
				Value:       20,
				Destination: &pageSize,
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

			input := service.ListHistoryInput{
				IssueID: issueID,
				Page:    port.PageRequest{PageSize: pageSize},
			}
			result, err := svc.ShowHistory(ctx, input)
			if err != nil {
				return fmt.Errorf("showing history: %w", err)
			}

			if jsonOutput {
				out := historyOutput{
					IssueID:    issueID.String(),
					TotalCount: result.TotalCount,
					Entries:    make([]historyEntryOutput, 0, len(result.Entries)),
				}
				for _, e := range result.Entries {
					entry := historyEntryOutput{
						ID:        e.ID(),
						Revision:  e.Revision(),
						Author:    e.Author().String(),
						EventType: e.EventType().String(),
						Timestamp: e.Timestamp().Format(time.RFC3339),
					}
					for _, c := range e.Changes() {
						entry.Changes = append(entry.Changes, fieldChangeOutput{
							Field:  c.Field,
							Before: c.Before,
							After:  c.After,
						})
					}
					out.Entries = append(out.Entries, entry)
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			// Human-readable output.
			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if len(result.Entries) == 0 {
				_, _ = fmt.Fprintln(w, "No history entries found.")
				return nil
			}

			for _, e := range result.Entries {
				_, _ = fmt.Fprintf(w, "%s  r%d  %s  %s  %s\n",
					cs.Dim(fmt.Sprintf("#%d", e.ID())),
					e.Revision(),
					cs.Bold(e.EventType().String()),
					e.Author().String(),
					cs.Dim(e.Timestamp().Format(time.RFC3339)))

				for _, c := range e.Changes() {
					_, _ = fmt.Fprintf(w, "    %s: %s → %s\n",
						cs.Cyan(c.Field), c.Before, c.After)
				}
			}

			_, _ = fmt.Fprintf(w, "\n%s total entries\n",
				cs.Dim(fmt.Sprintf("%d", result.TotalCount)))

			return nil
		},
	}
}

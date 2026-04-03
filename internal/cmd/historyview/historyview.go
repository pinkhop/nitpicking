package historyview

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// fieldChangeOutput is the JSON representation of a single field change.
type fieldChangeOutput struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// historyEntryOutput is the JSON representation of a single history entry.
type historyEntryOutput struct {
	Revision  int                 `json:"revision"`
	Author    string              `json:"author"`
	EventType string              `json:"event_type"`
	Timestamp string              `json:"timestamp"`
	Changes   []fieldChangeOutput `json:"changes,omitzero"`
}

// historyOutput is the JSON representation of the history command result.
type historyOutput struct {
	IssueID string               `json:"issue_id"`
	Entries []historyEntryOutput `json:"entries"`
	HasMore bool                 `json:"has_more"`
}

// RunInput holds the parameters for the history command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service     driving.Service
	IssueID     string
	Limit       int
	JSON        bool
	WriteTo     io.Writer
	ColorScheme *iostreams.ColorScheme
}

// Run executes the history workflow: queries the issue's mutation history and
// writes the result to the output writer as either JSON or human-readable text.
func Run(ctx context.Context, input RunInput) error {
	histInput := driving.ListHistoryInput{
		IssueID: input.IssueID,
		Limit:   input.Limit,
	}
	result, err := input.Service.ShowHistory(ctx, histInput)
	if err != nil {
		return fmt.Errorf("showing history: %w", err)
	}

	if input.JSON {
		out := historyOutput{
			IssueID: input.IssueID,
			HasMore: result.HasMore,
			Entries: make([]historyEntryOutput, 0, len(result.Entries)),
		}
		for _, e := range result.Entries {
			entry := historyEntryOutput{
				Revision:  e.Revision,
				Author:    e.Author,
				EventType: e.EventType,
				Timestamp: cmdutil.FormatJSONTimestamp(e.Timestamp),
			}
			for _, c := range e.Changes {
				entry.Changes = append(entry.Changes, fieldChangeOutput{
					Field:  c.Field,
					Before: c.Before,
					After:  c.After,
				})
			}
			out.Entries = append(out.Entries, entry)
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	// Human-readable output.
	cs := input.ColorScheme
	w := input.WriteTo

	if len(result.Entries) == 0 {
		_, _ = fmt.Fprintln(w, "No history entries found.")
		return nil
	}

	for _, e := range result.Entries {
		_, _ = fmt.Fprintf(w, "  r%d  %s  %s  %s\n",
			e.Revision,
			cs.Bold(e.EventType),
			e.Author,
			cs.Dim(e.Timestamp.Format(time.RFC3339)))

		for _, c := range e.Changes {
			_, _ = fmt.Fprintf(w, "    %s: %s → %s\n",
				cs.Cyan(c.Field), c.Before, c.After)
		}
	}

	shown := len(result.Entries)
	if result.HasMore {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d entries (more available)", shown)))
	} else {
		_, _ = fmt.Fprintf(w, "\n%s\n",
			cs.Dim(fmt.Sprintf("%d entries", shown)))
	}

	return nil
}

// NewCmd constructs the "history" command, which displays the mutation history
// of an issue.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		limit      int
	)

	return &cli.Command{
		Name:      "history",
		Usage:     "Show the mutation history of an issue",
		ArgsUsage: "<ISSUE-ID>",
		Description: `Displays the complete mutation history of an issue as a chronological
audit trail. Each entry shows the revision number, event type, author, timestamp,
and the specific fields that changed (with before and after values).

Use this when you need to understand how an issue reached its current state —
for example, to see who changed a priority, when a claim was acquired, or what
sequence of state transitions occurred. This is especially useful for debugging
workflow problems or reviewing the history of a contested issue.`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of entries (0 = default, negative = unlimited)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &limit,
			},
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

			return Run(ctx, RunInput{
				Service:     svc,
				IssueID:     issueID.String(),
				Limit:       limit,
				JSON:        jsonOutput,
				WriteTo:     f.IOStreams.Out,
				ColorScheme: f.IOStreams.ColorScheme(),
			})
		},
	}
}

package note

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// --- JSON output types ---

// addNoteOutput is the JSON representation of the note add result.
type addNoteOutput struct {
	NoteID  string `json:"note_id"`
	IssueID string `json:"issue_id"`
	Author  string `json:"author"`
}

// noteOutput is the JSON representation of a single note.
type noteOutput struct {
	NoteID    string `json:"note_id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// noteListOutput is the JSON representation of a note listing.
type noteListOutput struct {
	Notes      []noteOutput `json:"notes"`
	TotalCount int          `json:"total_count"`
}

// NewCmd constructs the "note" command with add, show, list, and search
// subcommands for managing issue notes.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "note",
		Usage: "Manage issue notes",
		Commands: []*cli.Command{
			newAddCmd(f),
			newShowCmd(f),
			newListCmd(f),
			newSearchCmd(f),
		},
	}
}

// newAddCmd constructs the "note add" subcommand, which adds a new note to
// an issue.
func newAddCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		author     string
		body       string
	)

	return &cli.Command{
		Name:  "add",
		Usage: "Add a note to an issue",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"t"},
				Usage:       "Issue ID",
				Required:    true,
				Destination: &issueArg,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "body",
				Aliases:     []string{"b"},
				Usage:       "Note body text",
				Required:    true,
				Destination: &body,
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

			input := service.AddNoteInput{
				IssueID: issueID,
				Author:  parsedAuthor,
				Body:    body,
			}
			result, err := svc.AddNote(ctx, input)
			if err != nil {
				return fmt.Errorf("adding note: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, addNoteOutput{
					NoteID:  result.Note.DisplayID(),
					IssueID: issueID.String(),
					Author:  author,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Added %s to %s\n",
				cs.SuccessIcon(),
				cs.Bold(result.Note.DisplayID()),
				cs.Bold(issueID.String()))
			return err
		},
	}
}

// newShowCmd constructs the "note show" subcommand, which retrieves a single
// note by its numeric ID.
func newShowCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "show",
		Usage:     "Show a note by ID",
		ArgsUsage: "<NOTE-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("note ID argument is required")
			}

			noteID, err := parseNoteID(rawID)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			n, err := svc.ShowNote(ctx, noteID)
			if err != nil {
				return fmt.Errorf("showing note: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, noteOutput{
					NoteID:    n.DisplayID(),
					IssueID:   n.IssueID().String(),
					Author:    n.Author().String(),
					Body:      n.Body(),
					CreatedAt: n.CreatedAt().Format(time.RFC3339),
				})
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out
			_, _ = fmt.Fprintf(w, "%s  on %s  by %s  at %s\n",
				cs.Bold(n.DisplayID()),
				cs.Bold(n.IssueID().String()),
				n.Author().String(),
				n.CreatedAt().Format(time.RFC3339))
			_, _ = fmt.Fprintf(w, "\n%s\n", n.Body())

			return nil
		},
	}
}

// newListCmd constructs the "note list" subcommand, which lists notes for a
// specific issue.
func newListCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		pageSize   int
	)

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List notes for an issue",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"t"},
				Usage:       "Issue ID",
				Required:    true,
				Destination: &issueArg,
			},
			&cli.IntFlag{
				Name:        "page-size",
				Usage:       "Number of results per page",
				Value:       20,
				Destination: &pageSize,
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

			input := service.ListNotesInput{
				IssueID: issueID,
				Page:    port.PageRequest{PageSize: pageSize},
			}
			result, err := svc.ListNotes(ctx, input)
			if err != nil {
				return fmt.Errorf("listing notes: %w", err)
			}

			if jsonOutput {
				out := noteListOutput{
					TotalCount: result.TotalCount,
					Notes:      make([]noteOutput, 0, len(result.Notes)),
				}
				for _, n := range result.Notes {
					out.Notes = append(out.Notes, noteOutput{
						NoteID:    n.DisplayID(),
						IssueID:   n.IssueID().String(),
						Author:    n.Author().String(),
						Body:      n.Body(),
						CreatedAt: n.CreatedAt().Format(time.RFC3339),
					})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			w := f.IOStreams.Out
			cs := f.IOStreams.ColorScheme()

			if len(result.Notes) == 0 {
				_, _ = fmt.Fprintln(w, "No notes found.")
				return nil
			}

			for _, n := range result.Notes {
				_, _ = fmt.Fprintf(w, "%s  %s  %s  %s\n",
					cs.Bold(n.DisplayID()),
					n.Author().String(),
					cs.Dim(n.CreatedAt().Format(time.RFC3339)),
					truncate(n.Body(), 80))
			}

			_, _ = fmt.Fprintf(w, "\n%s total\n", cs.Dim(fmt.Sprintf("%d", result.TotalCount)))

			return nil
		},
	}
}

// newSearchCmd constructs the "note search" subcommand, which performs
// full-text search across note bodies.
func newSearchCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		pageSize   int
	)

	return &cli.Command{
		Name:      "search",
		Usage:     "Search notes by text",
		ArgsUsage: "<QUERY>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"t"},
				Usage:       "Scope search to a specific issue ID",
				Destination: &issueArg,
			},
			&cli.IntFlag{
				Name:        "page-size",
				Usage:       "Number of results per page",
				Value:       20,
				Destination: &pageSize,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			query := cmd.Args().Get(0)
			if query == "" {
				return cmdutil.FlagErrorf("search query argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			input := service.SearchNotesInput{
				Query: query,
				Page:  port.PageRequest{PageSize: pageSize},
			}

			if issueArg != "" {
				tid, err := resolver.Resolve(ctx, issueArg)
				if err != nil {
					return cmdutil.FlagErrorf("invalid issue ID: %s", err)
				}
				input.IssueID = tid
			}
			result, err := svc.SearchNotes(ctx, input)
			if err != nil {
				return fmt.Errorf("searching notes: %w", err)
			}

			if jsonOutput {
				out := noteListOutput{
					TotalCount: result.TotalCount,
					Notes:      make([]noteOutput, 0, len(result.Notes)),
				}
				for _, n := range result.Notes {
					out.Notes = append(out.Notes, noteOutput{
						NoteID:    n.DisplayID(),
						IssueID:   n.IssueID().String(),
						Author:    n.Author().String(),
						Body:      n.Body(),
						CreatedAt: n.CreatedAt().Format(time.RFC3339),
					})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			w := f.IOStreams.Out
			cs := f.IOStreams.ColorScheme()

			if len(result.Notes) == 0 {
				_, _ = fmt.Fprintln(w, "No notes found.")
				return nil
			}

			for _, n := range result.Notes {
				_, _ = fmt.Fprintf(w, "%s  %s  %s  %s  %s\n",
					cs.Bold(n.DisplayID()),
					cs.Cyan(n.IssueID().String()),
					n.Author().String(),
					cs.Dim(n.CreatedAt().Format(time.RFC3339)),
					truncate(n.Body(), 60))
			}

			_, _ = fmt.Fprintf(w, "\n%s total\n", cs.Dim(fmt.Sprintf("%d", result.TotalCount)))

			return nil
		},
	}
}

// parseNoteID parses a note ID string. It accepts both "note-123" and "123"
// forms, returning the numeric portion.
func parseNoteID(s string) (int64, error) {
	s = strings.TrimPrefix(s, "note-")
	var id int64
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid note ID %q: must be a number or note-<number>", s)
	}
	if id <= 0 {
		return 0, fmt.Errorf("invalid note ID %q: must be a positive integer", s)
	}
	return id, nil
}

// truncate shortens a string to maxLen runes, appending "..." if truncated.
// Newlines are replaced with spaces for single-line display.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

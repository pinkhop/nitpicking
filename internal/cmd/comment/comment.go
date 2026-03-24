package comment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
)

// --- JSON output types ---

// addCommentOutput is the JSON representation of the comment add result.
type addCommentOutput struct {
	CommentID string `json:"comment_id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
}

// commentOutput is the JSON representation of a single comment.
type commentOutput struct {
	CommentID string `json:"comment_id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// commentListOutput is the JSON representation of a comment listing.
type commentListOutput struct {
	Comments []commentOutput `json:"comments"`
	HasMore  bool            `json:"has_more"`
}

// NewCmd constructs the "comment" command with add, show, list, and search
// subcommands for managing issue comments.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "comment",
		Usage: "Manage issue comments",
		Commands: []*cli.Command{
			newAddCmd(f),
			newShowCmd(f),
			newListCmd(f),
			newSearchCmd(f),
		},
	}
}

// newAddCmd constructs the "comment add" subcommand, which adds a new comment to
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
		Usage: "Add a comment to an issue",
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
				Usage:       "Comment body text",
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

			input := service.AddCommentInput{
				IssueID: issueID,
				Author:  parsedAuthor,
				Body:    body,
			}
			result, err := svc.AddComment(ctx, input)
			if err != nil {
				return fmt.Errorf("adding comment: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, addCommentOutput{
					CommentID: result.Comment.DisplayID(),
					IssueID:   issueID.String(),
					Author:    author,
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

// newShowCmd constructs the "comment show" subcommand, which retrieves a single
// comment by its numeric ID.
func newShowCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "show",
		Usage:     "Show a comment by ID",
		ArgsUsage: "<COMMENT-ID>",
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
				return cmdutil.FlagErrorf("comment ID argument is required")
			}

			commentID, err := parseCommentID(rawID)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			n, err := svc.ShowComment(ctx, commentID)
			if err != nil {
				return fmt.Errorf("showing comment: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, commentOutput{
					CommentID: n.DisplayID(),
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

// newListCmd constructs the "comment list" subcommand, which lists comments for a
// specific issue.
func newListCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		limit      int
	)

	return &cli.Command{
		Name:    "list",
		Aliases: []string{"ls"},
		Usage:   "List comments for an issue",
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
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results (0 = default, negative = unlimited)",
				Destination: &limit,
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

			input := service.ListCommentsInput{
				IssueID: issueID,
				Limit:   limit,
			}
			result, err := svc.ListComments(ctx, input)
			if err != nil {
				return fmt.Errorf("listing comments: %w", err)
			}

			if jsonOutput {
				out := commentListOutput{
					HasMore:  result.HasMore,
					Comments: make([]commentOutput, 0, len(result.Comments)),
				}
				for _, n := range result.Comments {
					out.Comments = append(out.Comments, commentOutput{
						CommentID: n.DisplayID(),
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

			if len(result.Comments) == 0 {
				_, _ = fmt.Fprintln(w, "No comments found.")
				return nil
			}

			for _, n := range result.Comments {
				_, _ = fmt.Fprintf(w, "%s  %s  %s  %s\n",
					cs.Bold(n.DisplayID()),
					n.Author().String(),
					cs.Dim(n.CreatedAt().Format(time.RFC3339)),
					truncate(n.Body(), 80))
			}

			shown := len(result.Comments)
			if result.HasMore {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d comments (more available)", shown)))
			} else {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d comments", shown)))
			}

			return nil
		},
	}
}

// newSearchCmd constructs the "comment search" subcommand, which performs
// full-text search across comment bodies.
func newSearchCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
		limit      int
	)

	return &cli.Command{
		Name:      "search",
		Usage:     "Search comments by text",
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
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results (0 = default, negative = unlimited)",
				Destination: &limit,
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

			input := service.SearchCommentsInput{
				Query: query,
				Limit: limit,
			}

			if issueArg != "" {
				tid, err := resolver.Resolve(ctx, issueArg)
				if err != nil {
					return cmdutil.FlagErrorf("invalid issue ID: %s", err)
				}
				input.IssueID = tid
			}
			result, err := svc.SearchComments(ctx, input)
			if err != nil {
				return fmt.Errorf("searching comments: %w", err)
			}

			if jsonOutput {
				out := commentListOutput{
					HasMore:  result.HasMore,
					Comments: make([]commentOutput, 0, len(result.Comments)),
				}
				for _, n := range result.Comments {
					out.Comments = append(out.Comments, commentOutput{
						CommentID: n.DisplayID(),
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

			if len(result.Comments) == 0 {
				_, _ = fmt.Fprintln(w, "No comments found.")
				return nil
			}

			for _, n := range result.Comments {
				_, _ = fmt.Fprintf(w, "%s  %s  %s  %s  %s\n",
					cs.Bold(n.DisplayID()),
					cs.Cyan(n.IssueID().String()),
					n.Author().String(),
					cs.Dim(n.CreatedAt().Format(time.RFC3339)),
					truncate(n.Body(), 60))
			}

			shown := len(result.Comments)
			if result.HasMore {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d comments (more available)", shown)))
			} else {
				_, _ = fmt.Fprintf(w, "\n%s\n",
					cs.Dim(fmt.Sprintf("%d comments", shown)))
			}

			return nil
		},
	}
}

// parseCommentID parses a comment ID string. It accepts both "comment-123" and "123"
// forms, returning the numeric portion.
func parseCommentID(s string) (int64, error) {
	s = strings.TrimPrefix(s, "comment-")
	var id int64
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid comment ID %q: must be a number or comment-<number>", s)
	}
	if id <= 0 {
		return 0, fmt.Errorf("invalid comment ID %q: must be a positive integer", s)
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

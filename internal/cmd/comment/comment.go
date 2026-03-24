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

// NewCmd constructs the "comment" command with add and list subcommands for
// managing issue comments.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "comment",
		Usage: "Manage issue comments",
		Commands: []*cli.Command{
			newAddCmd(f),
			newListCmd(f),
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
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"t"},
				Usage:       "Issue ID (required)",
				Required:    true,
				Destination: &issueArg,
			},
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
				Aliases:     []string{"b"},
				Usage:       "Comment body text (required)",
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
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"t"},
				Usage:       "Issue ID (required)",
				Required:    true,
				Destination: &issueArg,
			},
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

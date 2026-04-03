package jsoncmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// commentInput is the JSON object read from stdin for the "json comment"
// subcommand. It contains only content fields — identity and context flags
// (--author, --issue) remain on the command line.
type commentInput struct {
	Body string `json:"body"`
}

// commentOutput is the JSON representation of the comment add result.
type commentOutput struct {
	CommentID string `json:"comment_id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
}

// RunCommentInput holds the parameters for the json comment operation,
// decoupled from CLI flag parsing so it can be tested directly.
type RunCommentInput struct {
	Service driving.Service
	IssueID string
	Author  string
	Stdin   io.Reader
	WriteTo io.Writer
}

// RunComment reads a JSON object from stdin, validates it, and adds a comment
// to the specified issue via the service layer. Output is always JSON.
func RunComment(ctx context.Context, input RunCommentInput) error {
	payload, err := DecodeStdin[commentInput](input.Stdin)
	if err != nil {
		return fmt.Errorf("reading comment JSON from stdin: %w", err)
	}

	if payload.Body == "" {
		return fmt.Errorf("\"body\" field is required and must be non-empty")
	}

	result, err := input.Service.AddComment(ctx, driving.AddCommentInput{
		IssueID: input.IssueID,
		Author:  input.Author,
		Body:    payload.Body,
	})
	if err != nil {
		return fmt.Errorf("adding comment: %w", err)
	}

	return cmdutil.WriteJSON(input.WriteTo, commentOutput{
		CommentID: result.Comment.DisplayID,
		IssueID:   input.IssueID,
		Author:    input.Author,
	})
}

// newCommentCmd constructs the "json comment" subcommand, which adds a comment
// to an issue using structured JSON input from stdin. The --author and --issue
// flags identify the actor and target; the JSON object on stdin provides the
// comment body.
//
// Output is always JSON — there is no --json flag.
func newCommentCmd(f *cmdutil.Factory) *cli.Command {
	var author string

	return &cli.Command{
		Name:  "comment",
		Usage: "Add a comment to an issue (JSON stdin)",
		Description: `Adds a comment to an issue from a JSON object piped to stdin. The JSON
object must include a "body" field containing the comment text. The target
issue ID is provided as a positional argument, and the --author flag
identifies who is commenting.

Comments do not require an active claim — you can comment on any issue at
any time, including closed ones. This makes comments useful for recording
decisions, context, progress notes, and post-mortems at any point in an
issue's lifecycle. Output is a JSON object containing the new comment's ID,
the issue ID, and the author.`,
		ArgsUsage: "<ISSUE-ID>",
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

			return RunComment(ctx, RunCommentInput{
				Service: svc,
				IssueID: issueID.String(),
				Author:  author,
				Stdin:   f.IOStreams.In,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}

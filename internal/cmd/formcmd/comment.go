package formcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// CommentFormData holds the values collected by the interactive comment form.
// Exported so tests can populate it directly without running the TUI.
type CommentFormData struct {
	Author string
	Body   string
}

// RunFormCommentInput holds the parameters for the form comment operation,
// decoupled from CLI flag parsing so it can be tested directly.
type RunFormCommentInput struct {
	Service driving.Service
	IssueID string
	WriteTo io.Writer

	// FormRunner presents the interactive form and populates data. In
	// production this runs the huh TUI; in tests it is replaced with a
	// function that sets fields directly.
	FormRunner func(data *CommentFormData) error
}

// RunFormComment presents an interactive form for composing a comment, then
// adds it to the specified issue via the service layer. Output is
// human-readable text.
func RunFormComment(ctx context.Context, input RunFormCommentInput) error {
	data := &CommentFormData{}

	if err := input.FormRunner(data); err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}

	// Validate required fields.
	if data.Author == "" {
		return fmt.Errorf("author is required")
	}
	if strings.TrimSpace(data.Body) == "" {
		return fmt.Errorf("comment body is required")
	}

	result, err := input.Service.AddComment(ctx, driving.AddCommentInput{
		IssueID: input.IssueID,
		Author:  data.Author,
		Body:    data.Body,
	})
	if err != nil {
		return fmt.Errorf("adding comment: %w", err)
	}

	_, _ = fmt.Fprintf(input.WriteTo,
		"Added comment %s to %s\n",
		result.Comment.DisplayID, input.IssueID,
	)
	return nil
}

// defaultCommentFormRunner builds and runs the interactive huh form for
// composing a comment.
func defaultCommentFormRunner(data *CommentFormData) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Author").
				Placeholder("Your author name (required)").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("author is required")
					}
					return nil
				}).
				Value(&data.Author),

			huh.NewText().
				Title("Comment").
				Placeholder("Write your comment...").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("comment body is required")
					}
					return nil
				}).
				Value(&data.Body),
		),
	)

	return form.Run()
}

// newCommentCmd constructs the "form comment" subcommand, which interactively
// prompts the user to compose a comment on an issue. Takes a required issue ID
// argument. Output is human-readable text only — there is no --json flag.
func newCommentCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "comment",
		Usage: "Interactively compose a comment on an issue",
		Description: `Presents an interactive form for composing and adding a comment to an issue.
The form prompts for your author name and the comment body, with inline
validation to ensure neither is empty.

Comments do not require an active claim — you can comment on any issue,
including closed ones. This makes comments useful for recording context,
decisions, and observations at any point in an issue's lifecycle. For
scripted or agent-driven commenting, use "json comment" instead, which
reads a JSON object from stdin and produces machine-readable output.`,
		ArgsUsage: "<ISSUE-ID>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("issue ID argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return RunFormComment(ctx, RunFormCommentInput{
				Service:    svc,
				IssueID:    cmd.Args().First(),
				WriteTo:    f.IOStreams.Out,
				FormRunner: defaultCommentFormRunner,
			})
		},
	}
}

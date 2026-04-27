package formcmd_test

import (
	"bytes"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/formcmd"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Form Comment Test Helpers ---

// createTaskForComment creates a task and returns its ID string, for use as
// the target of a comment in tests.
func createTaskForComment(t *testing.T, svc driving.Service) string {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Commentable task",
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}
	return out.Issue.ID().String()
}

// --- RunFormComment Tests ---

func TestRunFormComment_SuccessfulSubmission_AddsComment(t *testing.T) {
	t.Parallel()

	// Given: an existing issue and a form runner that provides author and body.
	svc := setupCreateService(t)
	issueID := createTaskForComment(t, svc)
	var stdout bytes.Buffer

	input := formcmd.RunFormCommentInput{
		Service: svc,
		IssueID: issueID,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CommentFormData) error {
			data.Author = "alice"
			data.Body = "This is a test comment with some detail."
			return nil
		},
	}

	// When
	err := formcmd.RunFormComment(t.Context(), input)
	// Then: no error, and confirmation message is written.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte(issueID)) {
		t.Errorf("expected output to contain issue ID %s, got: %s", issueID, out)
	}
	if !bytes.Contains([]byte(out), []byte("comment")) {
		t.Errorf("expected output to mention 'comment', got: %s", out)
	}
}

func TestRunFormComment_UserAborts_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that simulates user abort.
	svc := setupCreateService(t)
	issueID := createTaskForComment(t, svc)
	var stdout bytes.Buffer

	input := formcmd.RunFormCommentInput{
		Service: svc,
		IssueID: issueID,
		WriteTo: &stdout,
		FormRunner: func(_ *formcmd.CommentFormData) error {
			return formcmd.ErrUserAborted
		},
	}

	// When
	err := formcmd.RunFormComment(t.Context(), input)

	// Then: the abort error is surfaced.
	if err == nil {
		t.Fatal("expected error for user abort, got nil")
	}
}

func TestRunFormComment_MissingAuthor_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that provides body but no author.
	svc := setupCreateService(t)
	issueID := createTaskForComment(t, svc)
	var stdout bytes.Buffer

	input := formcmd.RunFormCommentInput{
		Service: svc,
		IssueID: issueID,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CommentFormData) error {
			data.Body = "A comment body"
			return nil
		},
	}

	// When
	err := formcmd.RunFormComment(t.Context(), input)

	// Then: an error is returned because author is required.
	if err == nil {
		t.Fatal("expected error for missing author, got nil")
	}
}

func TestRunFormComment_MissingBody_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that provides author but no body.
	svc := setupCreateService(t)
	issueID := createTaskForComment(t, svc)
	var stdout bytes.Buffer

	input := formcmd.RunFormCommentInput{
		Service: svc,
		IssueID: issueID,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CommentFormData) error {
			data.Author = "alice"
			return nil
		},
	}

	// When
	err := formcmd.RunFormComment(t.Context(), input)

	// Then: an error is returned because body is required.
	if err == nil {
		t.Fatal("expected error for missing body, got nil")
	}
}

func TestRunFormComment_InvalidIssueID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a non-existent issue ID.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCommentInput{
		Service: svc,
		IssueID: "FOO-zzzzz",
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CommentFormData) error {
			data.Author = "alice"
			data.Body = "Comment on non-existent issue"
			return nil
		},
	}

	// When
	err := formcmd.RunFormComment(t.Context(), input)

	// Then: an error is returned because the issue does not exist.
	if err == nil {
		t.Fatal("expected error for invalid issue ID, got nil")
	}
}

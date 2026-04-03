package jsoncmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/jsoncmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Comment test helpers ---

// setupCommentService initialises a service backed by in-memory fakes and
// returns it ready for comment tests.
func setupCommentService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// createCommentTask creates a task and returns its ID.
func createCommentTask(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}
	return out.Issue.ID()
}

// --- RunComment Tests ---

func TestRunComment_AddsCommentAndReturnsJSON(t *testing.T) {
	t.Parallel()

	// Given: a task exists and a valid JSON body is provided on stdin.
	svc := setupCommentService(t)
	issueID := createCommentTask(t, svc, "Comment target")

	stdin := strings.NewReader(`{"body": "Found the root cause in auth.go"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCommentInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunComment(t.Context(), input)
	// Then: no error, and JSON output contains expected fields.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
	if result["author"] != "alice" {
		t.Errorf("author: got %q, want %q", result["author"], "alice")
	}
	if _, ok := result["comment_id"]; !ok {
		t.Error("expected comment_id in JSON output")
	}

	// Verify the comment was actually persisted.
	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: issueID.String(),
	})
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}
	if comments.Comments[0].Body != "Found the root cause in auth.go" {
		t.Errorf("comment body: got %q, want %q",
			comments.Comments[0].Body, "Found the root cause in auth.go")
	}
}

func TestRunComment_EmptyStdin_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: an empty stdin.
	svc := setupCommentService(t)
	issueID := createCommentTask(t, svc, "Empty stdin task")

	stdin := strings.NewReader("")
	var stdout bytes.Buffer

	input := jsoncmd.RunCommentInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunComment(t.Context(), input)

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}
}

func TestRunComment_MissingBodyField_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: JSON without the required "body" field.
	svc := setupCommentService(t)
	issueID := createCommentTask(t, svc, "Missing body task")

	stdin := strings.NewReader(`{}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCommentInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunComment(t.Context(), input)

	// Then: an error is returned because body is required.
	if err == nil {
		t.Fatal("expected error for missing body field, got nil")
	}
}

func TestRunComment_UnknownField_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: JSON with an unknown field.
	svc := setupCommentService(t)
	issueID := createCommentTask(t, svc, "Unknown field task")

	stdin := strings.NewReader(`{"body": "hello", "unknown": true}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCommentInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunComment(t.Context(), input)

	// Then: an error is returned because unknown fields are rejected.
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

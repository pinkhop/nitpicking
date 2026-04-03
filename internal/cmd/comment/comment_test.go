package comment_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/comment"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

// createTask creates a task and returns its ID.
func createTask(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}
	return out.Issue.ID()
}

// addComment adds a comment to an issue and returns the comment's display ID.
func addComment(t *testing.T, svc driving.Service, issueID domain.ID, body string) string {
	t.Helper()
	out, err := svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		Body:    body,
	})
	if err != nil {
		t.Fatalf("precondition: add comment failed: %v", err)
	}
	return out.Comment.DisplayID
}

// --- RunList Tests ---

func TestRunList_EmptyCommentList_ShowsNoCommentsMessage(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "No comments task")

	var buf bytes.Buffer
	input := comment.RunListInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := comment.RunList(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "No comments found.") {
		t.Errorf("expected 'No comments found.' message, got %q", buf.String())
	}
}

func TestRunList_WithComments_ShowsCommentDetails(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Commented task")
	addComment(t, svc, issueID, "First comment")
	addComment(t, svc, issueID, "Second comment")

	var buf bytes.Buffer
	input := comment.RunListInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := comment.RunList(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "First comment") {
		t.Errorf("expected 'First comment' in output, got %q", output)
	}
	if !strings.Contains(output, "Second comment") {
		t.Errorf("expected 'Second comment' in output, got %q", output)
	}
	if !strings.Contains(output, "2 comments") {
		t.Errorf("expected '2 comments' count in output, got %q", output)
	}
}

func TestRunList_LongCommentBody_TruncatesTo80Chars(t *testing.T) {
	t.Parallel()

	// Given — a comment with a body longer than 80 characters.
	svc := setupService(t)
	issueID := createTask(t, svc, "Long comment task")
	longBody := strings.Repeat("a", 120)
	addComment(t, svc, issueID, longBody)

	var buf bytes.Buffer
	input := comment.RunListInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := comment.RunList(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// The truncated body should end with "..." and not contain the full 120 chars.
	if strings.Contains(output, longBody) {
		t.Error("expected long body to be truncated, but found full body in output")
	}
	if !strings.Contains(output, "...") {
		t.Error("expected truncated body to end with '...'")
	}
}

func TestRunList_JSONOutput_ReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON list task")
	addComment(t, svc, issueID, "Comment for JSON")

	var buf bytes.Buffer
	input := comment.RunListInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := comment.RunList(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}

	comments, ok := result["comments"].([]any)
	if !ok {
		t.Fatal("expected 'comments' array in JSON output")
	}
	if len(comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments))
	}

	first := comments[0].(map[string]any)
	if first["body"] != "Comment for JSON" {
		t.Errorf("body: got %q, want %q", first["body"], "Comment for JSON")
	}
	if first["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", first["issue_id"], issueID.String())
	}
	if _, ok := first["comment_id"]; !ok {
		t.Error("expected comment_id in JSON comment output")
	}
	if _, ok := first["created_at"]; !ok {
		t.Error("expected created_at in JSON comment output")
	}
}

func TestRunList_JSONOutput_EmptyList_ReturnsEmptyArray(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Empty JSON list")

	var buf bytes.Buffer
	input := comment.RunListInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := comment.RunList(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}

	comments, ok := result["comments"].([]any)
	if !ok {
		t.Fatal("expected 'comments' array in JSON output")
	}
	if len(comments) != 0 {
		t.Errorf("comment count: got %d, want 0", len(comments))
	}
	if hasMore, ok := result["has_more"].(bool); !ok || hasMore {
		t.Errorf("has_more: got %v, want false", result["has_more"])
	}
}

func TestRunList_RespectsLimitFlag(t *testing.T) {
	t.Parallel()

	// Given — three comments, limit of 2.
	svc := setupService(t)
	issueID := createTask(t, svc, "Limited list task")
	addComment(t, svc, issueID, "Comment 1")
	addComment(t, svc, issueID, "Comment 2")
	addComment(t, svc, issueID, "Comment 3")

	var buf bytes.Buffer
	input := comment.RunListInput{
		Service: svc,
		IssueID: issueID.String(),
		Limit:   2,
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := comment.RunList(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}

	comments, ok := result["comments"].([]any)
	if !ok {
		t.Fatal("expected 'comments' array in JSON output")
	}
	if len(comments) != 2 {
		t.Errorf("comment count: got %d, want 2", len(comments))
	}
	if hasMore, ok := result["has_more"].(bool); !ok || !hasMore {
		t.Errorf("has_more: got %v, want true", result["has_more"])
	}
}

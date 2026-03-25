//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_CommentListJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — an issue with a comment.
	dir := initDB(t, "CL")
	taskID := createTask(t, dir, "Comment JSON audit", "comment-agent")
	_, stderr, code := runNP(t, dir, "comment", "add",
		"--issue", taskID,
		"--author", "comment-agent",
		"--body", "Test comment for JSON audit",
	)
	if code != 0 {
		t.Fatalf("precondition: comment add failed (exit %d): %s", code, stderr)
	}

	// When — list comments with JSON output.
	stdout, stderr, code := runNP(t, dir, "comment", "list", "--issue", taskID, "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("comment list --json failed (exit %d): %s", code, stderr)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	// AC: All top-level keys are snake_case.
	snakeCase := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range raw {
		if !snakeCase.MatchString(key) {
			t.Errorf("top-level key %q is not snake_case", key)
		}
	}

	result := parseJSON(t, stdout)
	comments, ok := result["comments"].([]any)
	if !ok || len(comments) == 0 {
		t.Fatalf("expected at least 1 comment, got %v", result["comments"])
	}

	comment, ok := comments[0].(map[string]any)
	if !ok {
		t.Fatalf("comment is not an object: %v", comments[0])
	}

	// AC: All comment keys are snake_case.
	for key := range comment {
		if !snakeCase.MatchString(key) {
			t.Errorf("comment key %q is not snake_case", key)
		}
	}

	// AC: comment_id is a non-empty string.
	commentID, ok := comment["comment_id"].(string)
	if !ok || commentID == "" {
		t.Errorf("comment_id must be a non-empty string, got %v", comment["comment_id"])
	}

	// AC: issue_id is a non-empty string.
	issueID, ok := comment["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", comment["issue_id"])
	}

	// AC: author is a string.
	author, ok := comment["author"].(string)
	if !ok || author != "comment-agent" {
		t.Errorf("author must be %q, got %v", "comment-agent", comment["author"])
	}

	// AC: created_at is UTC millisecond with Z suffix.
	createdAt, ok := comment["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}

	// AC: No is_deleted field.
	if _, exists := comment["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in comment JSON items")
	}
}

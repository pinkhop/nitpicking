//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

// assertCommentAddShape validates the JSON output of comment add / issue comment.
func assertCommentAddShape(t *testing.T, result map[string]any) {
	t.Helper()

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	// comment_id is a non-empty string.
	commentID, ok := result["comment_id"].(string)
	if !ok || commentID == "" {
		t.Errorf("comment_id should be a non-empty string, got %v (%T)", result["comment_id"], result["comment_id"])
	}

	// issue_id is a non-empty string.
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id should be a non-empty string, got %v (%T)", result["issue_id"], result["issue_id"])
	}

	// author is a non-empty string.
	authorVal, ok := result["author"].(string)
	if !ok || authorVal == "" {
		t.Errorf("author should be a non-empty string, got %v (%T)", result["author"], result["author"])
	}

	// No is_deleted.
	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}

	// No PascalCase leaks.
	for _, banned := range []string{"CommentID", "IssueID", "Author"} {
		if _, found := result[banned]; found {
			t.Errorf("PascalCase key %q leaked into JSON output", banned)
		}
	}
}

func TestE2E_CommentAddJSON_Shape(t *testing.T) {
	// Given — a task to add a comment to.
	dir := initDB(t, "CA")
	author := "comment-audit"
	taskID := createTask(t, dir, "Task for comment audit", author)

	// When — add a comment with np comment add --json.
	stdout, stderr, code := runNP(t, dir, "comment", "add",
		"--issue", taskID,
		"--author", author,
		"--body", "Audit comment body",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("comment add --json failed with exit code %d: %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertCommentAddShape(t, result)
	if result["issue_id"] != taskID {
		t.Errorf("expected issue_id %q, got %v", taskID, result["issue_id"])
	}
}

func TestE2E_IssueCommentJSON_Shape(t *testing.T) {
	// Given — a task to add a comment to.
	dir := initDB(t, "CA")
	author := "issuecmt-audit"
	taskID := createTask(t, dir, "Task for issue comment audit", author)

	// When — add a comment with np issue comment --json.
	stdout, stderr, code := runNP(t, dir, "issue", "comment", taskID,
		"--author", author,
		"--body", "Issue comment audit body",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("issue comment --json failed with exit code %d: %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertCommentAddShape(t, result)
	if result["issue_id"] != taskID {
		t.Errorf("expected issue_id %q, got %v", taskID, result["issue_id"])
	}
}

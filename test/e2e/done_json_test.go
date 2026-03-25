//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

// assertDoneOutputShape validates that the done/close JSON output has the
// correct shape according to the JSON audit AC.
func assertDoneOutputShape(t *testing.T, result map[string]any) {
	t.Helper()

	// AC1: All keys are snake_case.
	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	// AC2: issue_id is a non-empty string.
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id should be a non-empty string, got %v (%T)", result["issue_id"], result["issue_id"])
	}

	// action is a human-readable string.
	action, ok := result["action"].(string)
	if !ok || action == "" {
		t.Errorf("action should be a non-empty string, got %v (%T)", result["action"], result["action"])
	}

	// AC5: No is_deleted field.
	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}

	// No PascalCase leaks.
	for _, banned := range []string{"IssueID", "Action", "ClaimID"} {
		if _, found := result[banned]; found {
			t.Errorf("PascalCase key %q leaked into JSON output", banned)
		}
	}
}

func TestE2E_DoneJSON_Shape(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "DJ")
	author := "done-audit"
	taskID := createTask(t, dir, "Task for done audit", author)
	claimID := claimIssue(t, dir, taskID, author)

	// When — close the task with np done --json.
	stdout, stderr, code := runNP(t, dir, "done", taskID,
		"--claim", claimID,
		"--author", author,
		"--reason", "Completed: all tests pass.",
		"--json",
	)

	// Then — exit 0 and correct JSON shape.
	if code != 0 {
		t.Fatalf("done --json failed with exit code %d: %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertDoneOutputShape(t, result)

	if result["action"] != "done" {
		t.Errorf("expected action 'done', got %v", result["action"])
	}
	if result["issue_id"] != taskID {
		t.Errorf("expected issue_id %q, got %v", taskID, result["issue_id"])
	}
}

func TestE2E_CloseJSON_AliasShape(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "DJ")
	author := "close-audit"
	taskID := createTask(t, dir, "Task for close alias audit", author)
	claimID := claimIssue(t, dir, taskID, author)

	// When — close the task with np close --json (alias).
	stdout, stderr, code := runNP(t, dir, "close", taskID,
		"--claim", claimID,
		"--author", author,
		"--reason", "Done via close alias.",
		"--json",
	)

	// Then — identical shape to np done.
	if code != 0 {
		t.Fatalf("close --json failed with exit code %d: %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertDoneOutputShape(t, result)
}

func TestE2E_IssueCloseJSON_SubcommandShape(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "DJ")
	author := "issuecl-audit"
	taskID := createTask(t, dir, "Task for issue close audit", author)
	claimID := claimIssue(t, dir, taskID, author)

	// When — close the task with np issue close --json (subcommand).
	stdout, stderr, code := runNP(t, dir, "issue", "close", taskID,
		"--claim", claimID,
		"--author", author,
		"--reason", "Done via issue close.",
		"--json",
	)

	// Then — identical shape to np done.
	if code != 0 {
		t.Fatalf("issue close --json failed with exit code %d: %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertDoneOutputShape(t, result)
}

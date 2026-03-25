//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_IssueReopenJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a closed task.
	dir := initDB(t, "RO")
	taskID := createTask(t, dir, "Reopen JSON audit", "reopen-agent")
	claimID := claimIssue(t, dir, taskID, "reopen-agent")
	_, stderr, code := runNP(t, dir, "done", "--claim", claimID, "--author", "reopen-agent", "--reason", "closing for reopen test")
	if code != 0 {
		t.Fatalf("precondition: done failed (exit %d): %s", code, stderr)
	}

	// When — reopen the task with JSON output.
	stdout, stderr, code := runNP(t, dir, "issue", "reopen", taskID, "--author", "reopen-agent", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("issue reopen --json failed (exit %d): %s", code, stderr)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	// AC: All keys are snake_case.
	snakeCase := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range raw {
		if !snakeCase.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	result := parseJSON(t, stdout)

	// AC: issue_id is a non-empty string.
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", result["issue_id"])
	}
	if issueID != taskID {
		t.Errorf("issue_id %q must match the reopened issue %q", issueID, taskID)
	}

	// AC: action is a string.
	action, ok := result["action"].(string)
	if !ok || action != "reopen" {
		t.Errorf("action must be %q, got %v", "reopen", result["action"])
	}

	// AC: No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in reopen JSON output")
	}
}

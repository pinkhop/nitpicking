//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_IssueUpdateJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "UJ")
	taskID := createTask(t, dir, "Update JSON audit", "update-agent")
	claimID := claimIssue(t, dir, taskID, "update-agent")

	// When — update the task with JSON output.
	stdout, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--title", "Updated title",
		"--json",
	)

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("issue update --json failed (exit %d): %s", code, stderr)
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
		t.Errorf("issue_id %q must match the updated issue %q", issueID, taskID)
	}

	// AC: updated is a boolean.
	updated, ok := result["updated"].(bool)
	if !ok || !updated {
		t.Errorf("updated must be true, got %v", result["updated"])
	}

	// AC: No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in update JSON output")
	}
}

//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_LabelAddJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "LA")
	taskID := createTask(t, dir, "Label add JSON audit", "label-agent")
	claimID := claimIssue(t, dir, taskID, "label-agent")

	// When — add a label with JSON output.
	stdout, stderr, code := runNP(t, dir, "label", "add", "kind:bug",
		"--claim", claimID,
		"--json",
	)

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("label add --json failed (exit %d): %s", code, stderr)
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

	// issue_id is a non-empty string.
	if issueID, ok := result["issue_id"].(string); !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", result["issue_id"])
	}

	// key and value are strings.
	if key, ok := result["key"].(string); !ok || key != "kind" {
		t.Errorf("key must be %q, got %v", "kind", result["key"])
	}
	if value, ok := result["value"].(string); !ok || value != "bug" {
		t.Errorf("value must be %q, got %v", "bug", result["value"])
	}

	// action is "set".
	if action, ok := result["action"].(string); !ok || action != "set" {
		t.Errorf("action must be %q, got %v", "set", result["action"])
	}

	// No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in label add JSON output")
	}
}

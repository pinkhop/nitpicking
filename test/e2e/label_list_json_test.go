//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_LabelListJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a task with a label set.
	dir := initDB(t, "LL")
	taskID := createTask(t, dir, "Label list JSON audit", "label-agent")
	claimID := claimIssue(t, dir, taskID, "label-agent")
	_, stderr, code := runNP(t, dir, "label", "add", "kind:feature", "--claim", claimID)
	if code != 0 {
		t.Fatalf("precondition: label add failed (exit %d): %s", code, stderr)
	}

	// When — list labels with JSON output.
	stdout, stderr, code := runNP(t, dir, "label", "list", "--issue", taskID, "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("label list --json failed (exit %d): %s", code, stderr)
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

	// issue_id is a non-empty string.
	if issueID, ok := result["issue_id"].(string); !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", result["issue_id"])
	}

	// labels is an array.
	labels, ok := result["labels"].([]any)
	if !ok || len(labels) == 0 {
		t.Fatalf("expected at least 1 label, got %v", result["labels"])
	}

	label, ok := labels[0].(map[string]any)
	if !ok {
		t.Fatalf("label is not an object: %v", labels[0])
	}

	// All label keys are snake_case.
	for key := range label {
		if !snakeCase.MatchString(key) {
			t.Errorf("label key %q is not snake_case", key)
		}
	}

	// key and value are strings.
	if k, ok := label["key"].(string); !ok || k == "" {
		t.Errorf("key must be a non-empty string, got %v", label["key"])
	}
	if v, ok := label["value"].(string); !ok || v == "" {
		t.Errorf("value must be a non-empty string, got %v", label["value"])
	}

	// No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in label list JSON output")
	}
}

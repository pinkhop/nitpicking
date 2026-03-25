//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_LabelPropagateJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — an epic with a child and a label on the epic.
	dir := initDB(t, "LP")
	epicOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Propagate audit epic",
		"--author", "prop-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicID := parseJSON(t, epicOut)["id"].(string)

	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Child for propagation",
		"--author", "prop-agent",
		"--parent", epicID,
	)
	if code != 0 {
		t.Fatalf("precondition: create child failed (exit %d): %s", code, stderr)
	}

	// Set a label on the epic.
	epicClaimID := claimIssue(t, dir, epicID, "prop-agent")
	_, stderr, code = runNP(t, dir, "label", "add", "team:platform", "--claim", epicClaimID)
	if code != 0 {
		t.Fatalf("precondition: label add failed (exit %d): %s", code, stderr)
	}

	// When — propagate the label with JSON output.
	stdout, stderr, code := runNP(t, dir, "label", "propagate", "team",
		"--issue", epicID,
		"--author", "prop-agent",
		"--json",
	)

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("label propagate --json failed (exit %d): %s", code, stderr)
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

	// propagated is a number >= 1.
	propagated, ok := result["propagated"].(float64)
	if !ok || propagated < 1 {
		t.Errorf("propagated must be >= 1, got %v", result["propagated"])
	}

	// No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in propagate JSON output")
	}
}

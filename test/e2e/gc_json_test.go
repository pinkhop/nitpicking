//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_AdminGCJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a fresh database (no deleted or closed issues to GC).
	dir := initDB(t, "GC")

	// When — gc with --confirm and JSON output.
	stdout, stderr, code := runNP(t, dir, "admin", "gc", "--confirm", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("admin gc --confirm --json failed (exit %d): %s", code, stderr)
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

	// deleted_issues_removed is a number.
	if _, ok := result["deleted_issues_removed"].(float64); !ok {
		t.Errorf("deleted_issues_removed must be a number, got %v (%T)", result["deleted_issues_removed"], result["deleted_issues_removed"])
	}

	// closed_issues_removed is a number.
	if _, ok := result["closed_issues_removed"].(float64); !ok {
		t.Errorf("closed_issues_removed must be a number, got %v (%T)", result["closed_issues_removed"], result["closed_issues_removed"])
	}

	// No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in gc JSON output")
	}
}

func TestE2E_AdminGCJSON_IncludeClosed_ConformsToJSONStandards(t *testing.T) {
	// Given — a database with a closed task.
	dir := initDB(t, "GI")
	taskID := createTask(t, dir, "GC include-closed", "gc-agent")
	claimID := claimIssue(t, dir, taskID, "gc-agent")
	_, stderr, code := runNP(t, dir, "done", "--claim", claimID, "--author", "gc-agent", "--reason", "done")
	if code != 0 {
		t.Fatalf("precondition: done failed (exit %d): %s", code, stderr)
	}

	// When — gc with --include-closed and JSON output.
	stdout, stderr, code := runNP(t, dir, "admin", "gc", "--confirm", "--include-closed", "--json")

	// Then — the JSON body conforms to standards and includes both count keys.
	if code != 0 {
		t.Fatalf("admin gc --confirm --include-closed --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)

	// Both count fields are present and numeric.
	if _, ok := result["deleted_issues_removed"].(float64); !ok {
		t.Errorf("deleted_issues_removed must be a number, got %v", result["deleted_issues_removed"])
	}
	if _, ok := result["closed_issues_removed"].(float64); !ok {
		t.Errorf("closed_issues_removed must be a number, got %v", result["closed_issues_removed"])
	}
}

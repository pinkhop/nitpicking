//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_StatusJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a database with a mix of open and closed issues.
	dir := initDB(t, "ST")
	taskID := createTask(t, dir, "Status audit task", "status-agent")

	// Close one task so closed count is non-zero.
	claimID := claimIssue(t, dir, taskID, "status-agent")
	_, stderr, code := runNP(t, dir, "done", "--claim", claimID, "--author", "status-agent", "--reason", "done")
	if code != 0 {
		t.Fatalf("precondition: done failed (exit %d): %s", code, stderr)
	}

	// Create another open task so open count is non-zero.
	createTask(t, dir, "Open status task", "status-agent")

	// When — status with JSON output.
	stdout, stderr, code := runNP(t, dir, "status", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("status --json failed (exit %d): %s", code, stderr)
	}

	// Parse into raw map to inspect keys.
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

	// AC: No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in status JSON output")
	}

	result := parseJSON(t, stdout)

	// Verify expected keys are present and are numeric.
	expectedKeys := []string{"open", "claimed", "deferred", "closed", "ready", "blocked", "total"}
	for _, key := range expectedKeys {
		val, ok := result[key].(float64)
		if !ok {
			t.Errorf("key %q must be a number, got %v (%T)", key, result[key], result[key])
			continue
		}
		if val < 0 {
			t.Errorf("key %q must be non-negative, got %v", key, val)
		}
	}

	// Verify counts are consistent with the test data.
	open := result["open"].(float64)
	closed := result["closed"].(float64)
	total := result["total"].(float64)

	if open < 1 {
		t.Errorf("expected at least 1 open issue, got %v", open)
	}
	if closed < 1 {
		t.Errorf("expected at least 1 closed issue, got %v", closed)
	}
	if total < open+closed {
		t.Errorf("total (%v) must be >= open (%v) + closed (%v)", total, open, closed)
	}
}

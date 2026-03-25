//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_IssueHistoryJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a task with some history (create + update).
	dir := initDB(t, "HJ")
	taskID := createTask(t, dir, "History JSON audit", "history-agent")
	claimID := claimIssue(t, dir, taskID, "history-agent")
	_, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--title", "Updated for history",
	)
	if code != 0 {
		t.Fatalf("precondition: update failed (exit %d): %s", code, stderr)
	}

	// When — show history with JSON output.
	stdout, stderr, code := runNP(t, dir, "issue", "history", taskID, "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("issue history --json failed (exit %d): %s", code, stderr)
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

	// AC: issue_id is a non-empty string.
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", result["issue_id"])
	}

	// AC: entries is an array with at least 2 entries (create + update).
	entries, ok := result["entries"].([]any)
	if !ok || len(entries) < 2 {
		t.Fatalf("expected at least 2 history entries, got %v", len(entries))
	}

	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("entry[%d] is not an object", i)
		}

		// All entry keys are snake_case.
		for key := range entry {
			if !snakeCase.MatchString(key) {
				t.Errorf("entry[%d] key %q is not snake_case", i, key)
			}
		}

		// event_type is a string.
		if _, ok := entry["event_type"].(string); !ok {
			t.Errorf("entry[%d] event_type must be a string, got %v", i, entry["event_type"])
		}

		// author is a string.
		if _, ok := entry["author"].(string); !ok {
			t.Errorf("entry[%d] author must be a string, got %v", i, entry["author"])
		}

		// timestamp is UTC millisecond with Z suffix.
		ts, ok := entry["timestamp"].(string)
		if !ok || !utcMillisecondZ.MatchString(ts) {
			t.Errorf("entry[%d] timestamp %q does not match UTC millisecond Z format", i, ts)
		}

		// No is_deleted field in entries.
		if _, exists := entry["is_deleted"]; exists {
			t.Errorf("entry[%d] must not contain is_deleted", i)
		}

		// changes, if present, have snake_case keys.
		if changes, ok := entry["changes"].([]any); ok {
			for j, rawChange := range changes {
				change, ok := rawChange.(map[string]any)
				if !ok {
					t.Errorf("entry[%d] change[%d] is not an object", i, j)
					continue
				}
				for key := range change {
					if !snakeCase.MatchString(key) {
						t.Errorf("entry[%d] change[%d] key %q is not snake_case", i, j, key)
					}
				}
			}
		}
	}

	// AC: No is_deleted at top level.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in history JSON output")
	}
}

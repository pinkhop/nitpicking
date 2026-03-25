//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_IssueOrphansJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a database with an orphan task (no parent).
	dir := initDB(t, "OJ")
	createTask(t, dir, "Orphan JSON audit task", "orphan-agent")

	// When — list orphans with JSON output.
	stdout, stderr, code := runNP(t, dir, "issue", "orphans", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("issue orphans --json failed (exit %d): %s", code, stderr)
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
	items, ok := result["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected at least 1 orphan item, got %v", result["items"])
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not an object: %v", items[0])
	}

	// AC: All item keys are snake_case.
	for key := range item {
		if !snakeCase.MatchString(key) {
			t.Errorf("item key %q is not snake_case", key)
		}
	}

	// AC: ID is a non-empty string.
	id, ok := item["id"].(string)
	if !ok || id == "" {
		t.Errorf("id must be a non-empty string, got %v", item["id"])
	}

	// AC: Role is a human-readable string.
	if role, ok := item["role"].(string); !ok || role != "task" {
		t.Errorf("role must be %q, got %v", "task", item["role"])
	}

	// AC: State is a human-readable string.
	state, ok := item["state"].(string)
	if !ok {
		t.Errorf("state must be a string, got %v", item["state"])
	}
	validStates := map[string]bool{"open": true, "claimed": true, "closed": true, "deferred": true}
	if !validStates[state] {
		t.Errorf("state %q is not a valid human-readable state", state)
	}

	// AC: No is_deleted field.
	if _, exists := item["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in orphans JSON items")
	}
}

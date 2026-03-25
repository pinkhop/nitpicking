//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_SearchJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a database with a task to search for.
	dir := initDB(t, "SQ")
	createTask(t, dir, "Searchable audit task", "search-agent")

	// When — search with JSON output.
	stdout, stderr, code := runNP(t, dir, "search", "Searchable", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("search --json failed (exit %d): %s", code, stderr)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	// Top-level keys are snake_case.
	snakeCase := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range raw {
		if !snakeCase.MatchString(key) {
			t.Errorf("top-level key %q is not snake_case", key)
		}
	}

	result := parseJSON(t, stdout)
	items, ok := result["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected at least 1 item, got %v", result["items"])
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
	role, ok := item["role"].(string)
	if !ok || role != "task" {
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

	// AC: created_at is UTC millisecond with Z suffix.
	createdAt, ok := item["created_at"].(string)
	if !ok || createdAt == "" {
		t.Fatalf("created_at must be a non-empty string, got %v", item["created_at"])
	}
	if !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}

	// AC: updated_at, when present, is UTC millisecond with Z suffix.
	// Zero-value timestamps should be omitted, not "0001-01-01T00:00:00Z".
	if updatedAt, ok := item["updated_at"].(string); ok {
		if !utcMillisecondZ.MatchString(updatedAt) {
			t.Errorf("updated_at %q does not match UTC millisecond Z format", updatedAt)
		}
	}

	// AC: No is_deleted field.
	if _, exists := item["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in search JSON items")
	}
}

func TestE2E_IssueQueryJSON_AliasConformsToJSONStandards(t *testing.T) {
	// Given — a database with a task to search for.
	dir := initDB(t, "IQ")
	createTask(t, dir, "Query alias audit", "query-agent")

	// When — issue query (alias) with JSON output.
	stdout, stderr, code := runNP(t, dir, "issue", "query", "alias", "--json")

	// Then — the JSON body is valid and matches search output format.
	if code != 0 {
		t.Fatalf("issue query --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)
	items, ok := result["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected at least 1 item, got %v", result["items"])
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not an object: %v", items[0])
	}

	// Verify timestamps conform.
	createdAt, ok := item["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}
}

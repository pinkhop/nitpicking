//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_EpicCloseEligibleJSON_DryRun_ConformsToJSONStandards(t *testing.T) {
	// Given — an epic whose only child is closed.
	dir := initDB(t, "EC")
	epicOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Eligible epic",
		"--author", "ce-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicResult := parseJSON(t, epicOut)
	epicID := epicResult["id"].(string)

	childOut, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Child to close",
		"--author", "ce-agent",
		"--parent", epicID,
		"--claim",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create child failed (exit %d): %s", code, stderr)
	}
	childResult := parseJSON(t, childOut)
	_ = childResult["id"].(string)
	claimID := childResult["claim_id"].(string)
	_, stderr, code = runNP(t, dir, "done", "--claim", claimID, "--author", "ce-agent", "--reason", "done")
	if code != 0 {
		t.Fatalf("precondition: done failed (exit %d): %s", code, stderr)
	}

	// When — dry-run close-eligible with JSON output.
	stdout, stderr, code := runNP(t, dir, "epic", "close-eligible",
		"--author", "ce-agent",
		"--dry-run",
		"--json",
	)

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("epic close-eligible --dry-run --json failed (exit %d): %s", code, stderr)
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

	// results is an array.
	results, ok := result["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("expected at least 1 result, got %v", result["results"])
	}

	item, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("result item is not an object: %v", results[0])
	}

	// All result keys are snake_case.
	for key := range item {
		if !snakeCase.MatchString(key) {
			t.Errorf("result key %q is not snake_case", key)
		}
	}

	// ID is a non-empty string.
	id, ok := item["id"].(string)
	if !ok || id == "" {
		t.Errorf("id must be a non-empty string, got %v", item["id"])
	}

	// closed is false for dry-run.
	closed, ok := item["closed"].(bool)
	if !ok || closed {
		t.Errorf("closed must be false for dry-run, got %v", item["closed"])
	}

	// No is_deleted field.
	if _, exists := item["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in close-eligible JSON items")
	}
}

func TestE2E_EpicCloseEligibleJSON_NoEligible_EmptyResults(t *testing.T) {
	// Given — a database with no eligible epics (just a standalone task).
	dir := initDB(t, "EN")
	createTask(t, dir, "No eligible epic", "ce-agent")

	// When — close-eligible with JSON output.
	stdout, stderr, code := runNP(t, dir, "epic", "close-eligible",
		"--author", "ce-agent",
		"--json",
	)

	// Then — results is empty.
	if code != 0 {
		t.Fatalf("epic close-eligible --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)

	results, ok := result["results"].([]any)
	if !ok {
		t.Fatalf("results must be an array, got %v", result["results"])
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d items", len(results))
	}
}

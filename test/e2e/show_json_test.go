//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

// utcMillisecondZ matches a UTC timestamp with exactly millisecond precision
// and a literal Z suffix — e.g., "2026-03-24T02:41:40.000Z".
var utcMillisecondZ = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

func TestE2E_ShowJSON_TaskConformsToJSONStandards(t *testing.T) {
	// Given — a fresh database with a task issue.
	dir := initDB(t, "SJ")
	taskID := createTask(t, dir, "Show JSON audit task", "show-json-agent")

	// When — show the issue as JSON.
	stdout, stderr, code := runNP(t, dir, "show", taskID, "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("show --json failed (exit %d): %s", code, stderr)
	}

	// Parse into ordered structure to inspect raw keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	// AC: All keys are snake_case (no uppercase letters).
	snakeCase := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range raw {
		if !snakeCase.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	// Parse into typed map for value checks.
	result := parseJSON(t, stdout)

	// AC: ID is a non-empty string.
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Errorf("id must be a non-empty string, got %v", result["id"])
	}

	// AC: Role is a human-readable string.
	role, ok := result["role"].(string)
	if !ok || role != "task" {
		t.Errorf("role must be %q, got %v", "task", result["role"])
	}

	// AC: State is a human-readable string.
	state, ok := result["state"].(string)
	if !ok {
		t.Errorf("state must be a string, got %v", result["state"])
	}
	validStates := map[string]bool{"open": true, "claimed": true, "closed": true, "deferred": true}
	if !validStates[state] {
		t.Errorf("state %q is not a valid human-readable state", state)
	}

	// AC: created_at is UTC millisecond with Z suffix.
	createdAt, ok := result["created_at"].(string)
	if !ok || createdAt == "" {
		t.Fatalf("created_at must be a non-empty string, got %v", result["created_at"])
	}
	if !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format (expected like 2026-03-24T02:41:40.000Z)", createdAt)
	}

	// AC: No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in show JSON output")
	}
}

func TestE2E_ShowJSON_ClaimedTaskIncludesClaimTimestamp(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "SC")
	taskID := createTask(t, dir, "Claimed show JSON", "claim-agent")
	_ = claimIssue(t, dir, taskID, "claim-agent")

	// When — show the claimed issue as JSON.
	stdout, stderr, code := runNP(t, dir, "show", taskID, "--json")

	// Then — claim_stale_at is a UTC millisecond Z timestamp.
	if code != 0 {
		t.Fatalf("show --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	staleAt, ok := result["claim_stale_at"].(string)
	if !ok || staleAt == "" {
		t.Fatalf("claim_stale_at must be a non-empty string for a claimed issue, got %v", result["claim_stale_at"])
	}
	if !utcMillisecondZ.MatchString(staleAt) {
		t.Errorf("claim_stale_at %q does not match UTC millisecond Z format", staleAt)
	}
}

func TestE2E_ShowJSON_EpicConformsToJSONStandards(t *testing.T) {
	// Given — an epic with a child task.
	dir := initDB(t, "SE")
	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Epic for show JSON audit",
		"--author", "epic-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicResult := parseJSON(t, stdout)
	epicID := epicResult["id"].(string)

	// Add a child task.
	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Child task",
		"--author", "epic-agent",
		"--parent", epicID,
	)
	if code != 0 {
		t.Fatalf("precondition: create child failed (exit %d): %s", code, stderr)
	}

	// When — show the epic as JSON.
	stdout, stderr, code = runNP(t, dir, "show", epicID, "--json")

	// Then — role is "epic" and child_count is present.
	if code != 0 {
		t.Fatalf("show --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	role, ok := result["role"].(string)
	if !ok || role != "epic" {
		t.Errorf("role must be %q, got %v", "epic", result["role"])
	}

	childCount, ok := result["child_count"].(float64)
	if !ok || childCount < 1 {
		t.Errorf("expected child_count >= 1 for epic with children, got %v", result["child_count"])
	}

	// created_at must conform to timestamp standard.
	createdAt, ok := result["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}
}

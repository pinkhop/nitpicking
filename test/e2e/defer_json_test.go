//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_IssueDeferJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "DJ")
	taskID := createTask(t, dir, "Defer JSON audit", "defer-agent")
	claimID := claimIssue(t, dir, taskID, "defer-agent")

	// When — defer without --until, with JSON output.
	stdout, stderr, code := runNP(t, dir, "issue", "defer", taskID,
		"--claim", claimID,
		"--json",
	)

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("issue defer --json failed (exit %d): %s", code, stderr)
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

	// AC: issue_id is a non-empty string.
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", result["issue_id"])
	}

	// AC: action is "defer".
	action, ok := result["action"].(string)
	if !ok || action != "defer" {
		t.Errorf("action must be %q, got %v", "defer", result["action"])
	}

	// AC: "until" key should not be present when --until is omitted.
	if _, exists := raw["until"]; exists {
		t.Errorf("until field must not be present when --until is omitted")
	}

	// AC: No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in defer JSON output")
	}
}

func TestE2E_IssueDeferJSON_WithUntilIncludesDate(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "DU")
	taskID := createTask(t, dir, "Defer with until", "defer-agent")
	claimID := claimIssue(t, dir, taskID, "defer-agent")

	// When — defer with --until flag.
	stdout, stderr, code := runNP(t, dir, "issue", "defer", taskID,
		"--claim", claimID,
		"--until", "2026-04-01",
		"--json",
	)

	// Then — the JSON body includes the until field.
	if code != 0 {
		t.Fatalf("issue defer --until --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)

	until, ok := result["until"].(string)
	if !ok || until != "2026-04-01" {
		t.Errorf("until must be %q, got %v", "2026-04-01", result["until"])
	}

	action, ok := result["action"].(string)
	if !ok || action != "defer" {
		t.Errorf("action must be %q, got %v", "defer", result["action"])
	}
}

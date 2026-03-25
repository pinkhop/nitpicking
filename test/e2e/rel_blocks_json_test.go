//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_RelBlocksListJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — two tasks with a blocking relationship.
	dir := initDB(t, "RB")
	taskA := createTask(t, dir, "Blocker task", "rel-agent")
	taskB := createTask(t, dir, "Blocked task", "rel-agent")
	_, stderr, code := runNP(t, dir, "rel", "add", taskB, "blocked_by", taskA, "--author", "rel-agent")
	if code != 0 {
		t.Fatalf("precondition: rel add failed (exit %d): %s", code, stderr)
	}

	// When — list blocking relationships with JSON output.
	stdout, stderr, code := runNP(t, dir, "rel", "blocks", "list", taskB, "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("rel blocks list --json failed (exit %d): %s", code, stderr)
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
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", result["issue_id"])
	}

	// relationships is an array.
	rels, ok := result["relationships"].([]any)
	if !ok || len(rels) == 0 {
		t.Fatalf("expected at least 1 relationship, got %v", result["relationships"])
	}

	rel, ok := rels[0].(map[string]any)
	if !ok {
		t.Fatalf("relationship is not an object: %v", rels[0])
	}

	// All rel keys are snake_case.
	for key := range rel {
		if !snakeCase.MatchString(key) {
			t.Errorf("relationship key %q is not snake_case", key)
		}
	}

	// source_id and target_id are non-empty strings.
	if sid, ok := rel["source_id"].(string); !ok || sid == "" {
		t.Errorf("source_id must be a non-empty string, got %v", rel["source_id"])
	}
	if tid, ok := rel["target_id"].(string); !ok || tid == "" {
		t.Errorf("target_id must be a non-empty string, got %v", rel["target_id"])
	}

	// type is a string.
	if tp, ok := rel["type"].(string); !ok || tp == "" {
		t.Errorf("type must be a non-empty string, got %v", rel["type"])
	}

	// No is_deleted field.
	if _, exists := rel["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in relationship JSON")
	}
}

func TestE2E_RelBlocksUnblockJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — two tasks with a blocking relationship.
	dir := initDB(t, "RU")
	taskA := createTask(t, dir, "Blocker to unblock", "rel-agent")
	taskB := createTask(t, dir, "Blocked to unblock", "rel-agent")
	_, stderr, code := runNP(t, dir, "rel", "add", taskB, "blocked_by", taskA, "--author", "rel-agent")
	if code != 0 {
		t.Fatalf("precondition: rel add failed (exit %d): %s", code, stderr)
	}

	// When — unblock with JSON output.
	stdout, stderr, code := runNP(t, dir, "rel", "blocks", "unblock", taskA, taskB,
		"--author", "rel-agent", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("rel blocks unblock --json failed (exit %d): %s", code, stderr)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	// AC: All keys are snake_case or single lowercase letters.
	for key := range raw {
		if key != "a" && key != "b" && key != "action" {
			t.Errorf("unexpected key %q in unblock JSON output", key)
		}
	}

	result := parseJSON(t, stdout)

	// a and b are non-empty strings.
	if a, ok := result["a"].(string); !ok || a == "" {
		t.Errorf("a must be a non-empty string, got %v", result["a"])
	}
	if b, ok := result["b"].(string); !ok || b == "" {
		t.Errorf("b must be a non-empty string, got %v", result["b"])
	}

	// action is "unblocked".
	if action, ok := result["action"].(string); !ok || action != "unblocked" {
		t.Errorf("action must be %q, got %v", "unblocked", result["action"])
	}
}

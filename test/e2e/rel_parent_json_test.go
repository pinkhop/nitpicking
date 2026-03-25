//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_RelParentChildrenJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — an epic with a child task.
	dir := initDB(t, "PC")
	epicOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Parent children audit",
		"--author", "parent-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicID := parseJSON(t, epicOut)["id"].(string)

	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Child for children list",
		"--author", "parent-agent",
		"--parent", epicID,
	)
	if code != 0 {
		t.Fatalf("precondition: create child failed (exit %d): %s", code, stderr)
	}

	// When — list children with JSON output.
	stdout, stderr, code := runNP(t, dir, "rel", "parent", "children", epicID, "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("rel parent children --json failed (exit %d): %s", code, stderr)
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
		t.Fatalf("expected at least 1 child, got %v", result["items"])
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not an object: %v", items[0])
	}

	// All item keys are snake_case.
	for key := range item {
		if !snakeCase.MatchString(key) {
			t.Errorf("item key %q is not snake_case", key)
		}
	}

	// ID is a non-empty string (not an empty object).
	id, ok := item["id"].(string)
	if !ok || id == "" {
		t.Errorf("id must be a non-empty string, got %v (%T)", item["id"], item["id"])
	}

	// Role and state are human-readable strings, not integers.
	role, ok := item["role"].(string)
	if !ok {
		t.Errorf("role must be a string, got %v (%T)", item["role"], item["role"])
	}
	if role != "task" && role != "epic" {
		t.Errorf("role %q is not a valid human-readable role", role)
	}

	if _, ok := item["state"].(string); !ok {
		t.Errorf("state must be a string, got %v (%T)", item["state"], item["state"])
	}

	// created_at is UTC millisecond with Z suffix.
	createdAt, ok := item["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}

	// No is_deleted field.
	if _, exists := item["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in children JSON items")
	}

	// No IsDeleted (PascalCase) field.
	if _, exists := item["IsDeleted"]; exists {
		t.Errorf("IsDeleted (PascalCase) field must not be present in children JSON items")
	}
}

func TestE2E_RelParentTreeJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — an epic with a child task.
	dir := initDB(t, "PT")
	epicOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Parent tree audit",
		"--author", "tree-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicID := parseJSON(t, epicOut)["id"].(string)

	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Descendant for tree",
		"--author", "tree-agent",
		"--parent", epicID,
	)
	if code != 0 {
		t.Fatalf("precondition: create child failed (exit %d): %s", code, stderr)
	}

	// When — show tree with JSON output.
	stdout, stderr, code := runNP(t, dir, "rel", "parent", "tree", epicID, "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("rel parent tree --json failed (exit %d): %s", code, stderr)
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
		t.Fatalf("expected at least 1 descendant, got %v", result["items"])
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not an object: %v", items[0])
	}

	// ID is a non-empty string (not an empty object).
	id, ok := item["id"].(string)
	if !ok || id == "" {
		t.Errorf("id must be a non-empty string, got %v (%T)", item["id"], item["id"])
	}

	// Role is a human-readable string.
	role, ok := item["role"].(string)
	if !ok || (role != "task" && role != "epic") {
		t.Errorf("role must be a string (task/epic), got %v (%T)", item["role"], item["role"])
	}

	// State is a human-readable string.
	if _, ok := item["state"].(string); !ok {
		t.Errorf("state must be a string, got %v (%T)", item["state"], item["state"])
	}

	// created_at is UTC millisecond with Z suffix.
	createdAt, ok := item["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}

	// No is_deleted field.
	if _, exists := item["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in tree JSON items")
	}
}

func TestE2E_RelParentDetachJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — an epic with a child task.
	dir := initDB(t, "PD")
	epicOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Detach audit epic",
		"--author", "detach-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicID := parseJSON(t, epicOut)["id"].(string)

	childOut, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Child to detach",
		"--author", "detach-agent",
		"--parent", epicID,
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create child failed (exit %d): %s", code, stderr)
	}
	childID := parseJSON(t, childOut)["id"].(string)

	// When — detach with JSON output.
	stdout, stderr, code := runNP(t, dir, "rel", "parent", "detach", epicID, childID,
		"--author", "detach-agent", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("rel parent detach --json failed (exit %d): %s", code, stderr)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	result := parseJSON(t, stdout)

	// child and parent are non-empty strings.
	if c, ok := result["child"].(string); !ok || c == "" {
		t.Errorf("child must be a non-empty string, got %v", result["child"])
	}
	if p, ok := result["parent"].(string); !ok || p == "" {
		t.Errorf("parent must be a non-empty string, got %v", result["parent"])
	}

	// action is "detached".
	if action, ok := result["action"].(string); !ok || action != "detached" {
		t.Errorf("action must be %q, got %v", "detached", result["action"])
	}
}

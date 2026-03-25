//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

func TestE2E_RelListJSON_Shape(t *testing.T) {
	// Given — a task with multiple relationship types.
	dir := initDB(t, "RL")
	author := "rellist-audit"
	taskA := createTask(t, dir, "Task A", author)
	taskB := createTask(t, dir, "Task B", author)
	taskC := createTask(t, dir, "Task C", author)

	// Add blocked_by and refs relationships.
	runNP(t, dir, "rel", "add", taskA, "blocked_by", taskB, "--author", author, "--json")
	runNP(t, dir, "rel", "add", taskA, "refs", taskC, "--author", author, "--json")

	// When — list all relationships for task A.
	stdout, stderr, code := runNP(t, dir, "rel", "list", taskA, "--json")

	// Then
	if code != 0 {
		t.Fatalf("rel list --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("top-level key %q is not snake_case", key)
		}
	}

	// issue_id is a non-empty string.
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID != taskA {
		t.Errorf("expected issue_id %q, got %v", taskA, result["issue_id"])
	}

	// relationships is an array.
	rels, ok := result["relationships"].([]any)
	if !ok {
		t.Fatalf("expected relationships array, got %T", result["relationships"])
	}
	if len(rels) != 2 {
		t.Errorf("expected 2 relationships, got %d", len(rels))
	}

	// Validate each relationship item.
	for i, raw := range rels {
		rel, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("relationships[%d]: expected object, got %T", i, raw)
		}
		for key := range rel {
			if !snakeCaseRE.MatchString(key) {
				t.Errorf("relationships[%d]: key %q is not snake_case", i, key)
			}
		}
		if _, ok := rel["source_id"].(string); !ok {
			t.Errorf("relationships[%d]: source_id should be a string", i)
		}
		if _, ok := rel["target_id"].(string); !ok {
			t.Errorf("relationships[%d]: target_id should be a string", i)
		}
		if _, ok := rel["type"].(string); !ok {
			t.Errorf("relationships[%d]: type should be a string", i)
		}
	}

	// No is_deleted.
	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}

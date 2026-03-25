//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

func TestE2E_RelRefsListJSON_Shape(t *testing.T) {
	// Given — two tasks with a refs relationship.
	dir := initDB(t, "RR")
	author := "refslist-audit"
	taskA := createTask(t, dir, "Task A", author)
	taskB := createTask(t, dir, "Task B", author)
	_, stderr, code := runNP(t, dir, "rel", "add", taskA, "refs", taskB,
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: rel add refs failed (exit %d): %s", code, stderr)
	}

	// When — list refs for task A.
	stdout, stderr, code := runNP(t, dir, "rel", "refs", "list", taskA, "--json")

	// Then
	if code != 0 {
		t.Fatalf("rel refs list --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	issueID, ok := result["issue_id"].(string)
	if !ok || issueID != taskA {
		t.Errorf("expected issue_id %q, got %v", taskA, result["issue_id"])
	}

	rels, ok := result["relationships"].([]any)
	if !ok {
		t.Fatalf("expected relationships array, got %T", result["relationships"])
	}
	if len(rels) != 1 {
		t.Errorf("expected 1 relationship, got %d", len(rels))
	}

	if len(rels) > 0 {
		rel := rels[0].(map[string]any)
		for key := range rel {
			if !snakeCaseRE.MatchString(key) {
				t.Errorf("relationship key %q is not snake_case", key)
			}
		}
		if _, ok := rel["source_id"].(string); !ok {
			t.Errorf("source_id should be a string, got %T", rel["source_id"])
		}
		if _, ok := rel["target_id"].(string); !ok {
			t.Errorf("target_id should be a string, got %T", rel["target_id"])
		}
		if rel["type"] != "refs" {
			t.Errorf("expected type 'refs', got %v", rel["type"])
		}
	}

	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}

func TestE2E_RelRefsUnrefJSON_Shape(t *testing.T) {
	// Given — two tasks with a refs relationship.
	dir := initDB(t, "RR")
	author := "unref-audit"
	taskA := createTask(t, dir, "Task A", author)
	taskB := createTask(t, dir, "Task B", author)
	_, stderr, code := runNP(t, dir, "rel", "add", taskA, "refs", taskB,
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: rel add refs failed (exit %d): %s", code, stderr)
	}

	// When — unref.
	stdout, stderr, code := runNP(t, dir, "rel", "refs", "unref", taskA, taskB,
		"--author", author, "--json")

	// Then
	if code != 0 {
		t.Fatalf("rel refs unref --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	if result["action"] != "unrefed" {
		t.Errorf("expected action 'unrefed', got %v", result["action"])
	}

	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}

//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

// assertRelAddShape validates that all keys are snake_case strings and no
// PascalCase or is_deleted keys leak.
func assertRelAddShape(t *testing.T, result map[string]any) {
	t.Helper()

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	// action is a string.
	action, ok := result["action"].(string)
	if !ok || action == "" {
		t.Errorf("action should be a non-empty string, got %v (%T)", result["action"], result["action"])
	}

	// No is_deleted.
	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}

func TestE2E_RelAddJSON_BlockedBy_Shape(t *testing.T) {
	// Given — two tasks.
	dir := initDB(t, "RA")
	author := "rel-audit"
	taskA := createTask(t, dir, "Blocker task", author)
	taskB := createTask(t, dir, "Blocked task", author)

	// When — add blocked_by relationship with --json.
	stdout, stderr, code := runNP(t, dir, "rel", "add", taskB, "blocked_by", taskA,
		"--author", author, "--json")

	// Then
	if code != 0 {
		t.Fatalf("rel add blocked_by --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertRelAddShape(t, result)

	if result["source"].(string) != taskB {
		t.Errorf("expected source %q, got %v", taskB, result["source"])
	}
	if result["target"].(string) != taskA {
		t.Errorf("expected target %q, got %v", taskA, result["target"])
	}
	if result["type"] != "blocked_by" {
		t.Errorf("expected type 'blocked_by', got %v", result["type"])
	}
}

func TestE2E_RelAddJSON_Blocks_Shape(t *testing.T) {
	dir := initDB(t, "RA")
	author := "rel-audit"
	taskA := createTask(t, dir, "Task A", author)
	taskB := createTask(t, dir, "Task B", author)

	stdout, stderr, code := runNP(t, dir, "rel", "add", taskA, "blocks", taskB,
		"--author", author, "--json")

	if code != 0 {
		t.Fatalf("rel add blocks --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertRelAddShape(t, result)
	if result["type"] != "blocks" {
		t.Errorf("expected type 'blocks', got %v", result["type"])
	}
}

func TestE2E_RelAddJSON_Refs_Shape(t *testing.T) {
	dir := initDB(t, "RA")
	author := "rel-audit"
	taskA := createTask(t, dir, "Task A", author)
	taskB := createTask(t, dir, "Task B", author)

	stdout, stderr, code := runNP(t, dir, "rel", "add", taskA, "refs", taskB,
		"--author", author, "--json")

	if code != 0 {
		t.Fatalf("rel add refs --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertRelAddShape(t, result)
	if result["type"] != "refs" {
		t.Errorf("expected type 'refs', got %v", result["type"])
	}
}

func TestE2E_RelAddJSON_ParentOf_Shape(t *testing.T) {
	dir := initDB(t, "RA")
	author := "rel-audit"
	epicStdout, _, _ := runNP(t, dir, "create",
		"--role", "epic", "--title", "Parent epic", "--author", author, "--json")
	epicID := parseJSON(t, epicStdout)["id"].(string)
	taskID := createTask(t, dir, "Child task", author)
	claimID := claimIssue(t, dir, taskID, author)

	stdout, stderr, code := runNP(t, dir, "rel", "add", epicID, "parent_of", taskID,
		"--claim", claimID, "--author", author, "--json")

	if code != 0 {
		t.Fatalf("rel add parent_of --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertRelAddShape(t, result)

	// parent_of returns child/parent/action instead of source/type/target.
	child, ok := result["child"].(string)
	if !ok || child != taskID {
		t.Errorf("expected child %q, got %v", taskID, result["child"])
	}
	parent, ok := result["parent"].(string)
	if !ok || parent != epicID {
		t.Errorf("expected parent %q, got %v", epicID, result["parent"])
	}
}

func TestE2E_RelAddJSON_ChildOf_Shape(t *testing.T) {
	dir := initDB(t, "RA")
	author := "rel-audit"
	epicStdout, _, _ := runNP(t, dir, "create",
		"--role", "epic", "--title", "Parent epic", "--author", author, "--json")
	epicID := parseJSON(t, epicStdout)["id"].(string)
	taskID := createTask(t, dir, "Child task", author)
	claimID := claimIssue(t, dir, taskID, author)

	stdout, stderr, code := runNP(t, dir, "rel", "add", taskID, "child_of", epicID,
		"--claim", claimID, "--author", author, "--json")

	if code != 0 {
		t.Fatalf("rel add child_of --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertRelAddShape(t, result)

	child, ok := result["child"].(string)
	if !ok || child != taskID {
		t.Errorf("expected child %q, got %v", taskID, result["child"])
	}
	parent, ok := result["parent"].(string)
	if !ok || parent != epicID {
		t.Errorf("expected parent %q, got %v", epicID, result["parent"])
	}
}

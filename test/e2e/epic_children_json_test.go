//go:build e2e

package e2e_test

import (
	"testing"
)

func TestE2E_EpicChildrenJSON_Shape(t *testing.T) {
	// Given — an epic with two children (task and nested epic).
	dir := initDB(t, "EC")
	author := "children-audit"
	epicStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Parent epic", "--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create epic failed")
	}
	epicID := parseJSON(t, epicStdout)["id"].(string)

	createTaskWithParent(t, dir, "Child task A", author, epicID)
	createTaskWithParent(t, dir, "Child task B", author, epicID)

	// When — epic children with --json.
	stdout, stderr, code := runNP(t, dir, "epic", "children", epicID, "--json")

	// Then — exit 0 and correct shape (reuses assertListOutputShape from
	// list_json_test.go since the structure is identical).
	if code != 0 {
		t.Fatalf("epic children --json failed (exit %d): %s", code, stderr)
	}
	assertListOutputShape(t, stdout)

	result := parseJSON(t, stdout)
	items := result["items"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 children, got %d", len(items))
	}
}

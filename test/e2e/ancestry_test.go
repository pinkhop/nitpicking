//go:build e2e

package e2e_test

import "testing"

func TestE2E_List_DescendantsOf_ReturnsAllChildren(t *testing.T) {
	// Given — an epic with nested children: epic → task A, epic → sub-epic → task B.
	dir := initDB(t, "ANC")
	author := "ancestry-agent"
	epicID := createEpic(t, dir, "Root epic", author)
	createTaskWithParent(t, dir, "Child task A", author, epicID)
	subEpicStdout, _, _ := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Sub-epic",
		"--author", author,
		"--parent", epicID,
		"--json",
	)
	subEpicID := parseJSON(t, subEpicStdout)["id"].(string)
	createTaskWithParent(t, dir, "Grandchild task B", author, subEpicID)

	// When — list descendants of the root epic.
	stdout, stderr, code := runNP(t, dir, "list", "--descendants-of", epicID, "--json")

	// Then — all descendants (child, sub-epic, grandchild) are returned.
	if code != 0 {
		t.Fatalf("list --descendants-of failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	totalCount, ok := result["total_count"].(float64)
	if !ok || totalCount != 3 {
		t.Errorf("expected 3 descendants, got %v", result["total_count"])
	}
}

func TestE2E_List_AncestorsOf_ReturnsParentChain(t *testing.T) {
	// Given — a three-level hierarchy: root epic → sub-epic → task.
	dir := initDB(t, "ANC")
	author := "ancestry-agent"
	rootID := createEpic(t, dir, "Root epic", author)
	subStdout, _, _ := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Sub-epic",
		"--author", author,
		"--parent", rootID,
		"--json",
	)
	subID := parseJSON(t, subStdout)["id"].(string)
	leafID := createTaskWithParent(t, dir, "Leaf task", author, subID)

	// When — list ancestors of the leaf task.
	stdout, stderr, code := runNP(t, dir, "list", "--ancestors-of", leafID, "--json")

	// Then — both the sub-epic and root epic are returned.
	if code != 0 {
		t.Fatalf("list --ancestors-of failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	totalCount, ok := result["total_count"].(float64)
	if !ok || totalCount != 2 {
		t.Errorf("expected 2 ancestors (sub-epic + root), got %v", result["total_count"])
	}
}

func TestE2E_List_DescendantsOf_ComposesWithReady(t *testing.T) {
	// Given — an epic with a closed child and an open child.
	dir := initDB(t, "ANC")
	author := "ancestry-agent"
	epicID := createEpic(t, dir, "Filter epic", author)
	createTaskWithParent(t, dir, "Open child", author, epicID)
	closedID := createTaskWithParent(t, dir, "Will be closed", author, epicID)
	closeIssue(t, dir, closedID, author)

	// When — list ready descendants of the epic.
	stdout, stderr, code := runNP(t, dir, "list",
		"--descendants-of", epicID,
		"--ready",
		"--json",
	)

	// Then — only the open child appears (closed child is not ready).
	if code != 0 {
		t.Fatalf("list --descendants-of --ready failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	totalCount, ok := result["total_count"].(float64)
	if !ok || totalCount != 1 {
		t.Errorf("expected 1 ready descendant, got %v", result["total_count"])
	}
}

//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

func TestE2E_RelParentTree_TextOutput_IncludesRootAndIndentation(t *testing.T) {
	// Given — an epic with a child task.
	dir := initDB(t, "TT")
	epicOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Tree root epic",
		"--author", "tree-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicID := parseJSON(t, epicOut)["id"].(string)

	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Direct child",
		"--author", "tree-agent",
		"--parent", epicID,
	)
	if code != 0 {
		t.Fatalf("precondition: create child failed (exit %d): %s", code, stderr)
	}

	// When — show tree as text.
	stdout, stderr, code := runNP(t, dir, "rel", "parent", "tree", epicID)

	// Then — root issue is included and descendants are indented.
	if code != 0 {
		t.Fatalf("rel parent tree failed (exit %d): %s", code, stderr)
	}

	// AC: Root issue ID appears in output.
	if !strings.Contains(stdout, epicID) {
		t.Errorf("expected root issue ID %s in output, got:\n%s", epicID, stdout)
	}

	// AC: Tree uses indentation characters (├── or └──).
	if !strings.Contains(stdout, "└──") && !strings.Contains(stdout, "├──") {
		t.Errorf("expected tree indentation characters in output, got:\n%s", stdout)
	}
}

func TestE2E_RelParentTree_TextOutput_NestedHierarchy(t *testing.T) {
	// Given — a 3-level hierarchy: epic -> child epic -> grandchild task.
	dir := initDB(t, "TN")
	epicOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Grandparent epic",
		"--author", "tree-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create epic failed (exit %d): %s", code, stderr)
	}
	epicID := parseJSON(t, epicOut)["id"].(string)

	childOut, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Child epic",
		"--author", "tree-agent",
		"--parent", epicID,
		"--json",
	)
	if code != 0 {
		t.Fatalf("precondition: create child epic failed (exit %d): %s", code, stderr)
	}
	childEpicID := parseJSON(t, childOut)["id"].(string)

	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Grandchild task",
		"--author", "tree-agent",
		"--parent", childEpicID,
	)
	if code != 0 {
		t.Fatalf("precondition: create grandchild failed (exit %d): %s", code, stderr)
	}

	// When — show tree as text.
	stdout, stderr, code := runNP(t, dir, "rel", "parent", "tree", epicID)

	// Then — all three levels appear with increasing indentation.
	if code != 0 {
		t.Fatalf("rel parent tree failed (exit %d): %s", code, stderr)
	}

	// Root, child, and grandchild IDs all appear.
	if !strings.Contains(stdout, epicID) {
		t.Errorf("expected root ID %s in output", epicID)
	}
	if !strings.Contains(stdout, childEpicID) {
		t.Errorf("expected child epic ID %s in output", childEpicID)
	}
	if !strings.Contains(stdout, "Grandchild task") {
		t.Errorf("expected grandchild title in output, got:\n%s", stdout)
	}

	// Nested indentation: grandchild should have deeper tree prefix.
	lines := strings.Split(stdout, "\n")
	var grandchildLine string
	for _, line := range lines {
		if strings.Contains(line, "Grandchild task") {
			grandchildLine = line
			break
		}
	}
	if grandchildLine == "" {
		t.Fatalf("grandchild line not found in output:\n%s", stdout)
	}
	// Grandchild should have more leading whitespace or deeper tree chars than child.
	if !strings.Contains(grandchildLine, "    ") {
		t.Errorf("expected deeper indentation for grandchild, got: %q", grandchildLine)
	}
}

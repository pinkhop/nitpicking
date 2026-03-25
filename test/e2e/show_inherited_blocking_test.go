//go:build e2e

package e2e_test

import (
	"testing"
)

func TestE2E_ShowJSON_InheritedBlocking_SurfacedWhenAncestorBlocked(t *testing.T) {
	// Given — an epic blocked by another task, with a child task.
	dir := initDB(t, "IB")
	author := "inherited-blocking"

	blockerID := createTask(t, dir, "Blocker task", author)

	epicStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Blocked epic",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create epic failed")
	}
	epicID := parseJSON(t, epicStdout)["id"].(string)

	// Block the epic.
	_, stderr, code := runNP(t, dir, "rel", "add", epicID, "blocked_by", blockerID,
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: rel add blocked_by failed (exit %d): %s", code, stderr)
	}

	// Create a child task under the blocked epic.
	childID := createTaskWithParent(t, dir, "Child of blocked epic", author, epicID)

	// When — show the child task.
	stdout, stderr, code := runNP(t, dir, "show", childID, "--json")

	// Then — inherited_blocking is present.
	if code != 0 {
		t.Fatalf("show --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	// The child should not be ready (ancestor is blocked).
	if result["is_ready"] == true {
		t.Error("expected is_ready false when ancestor is blocked")
	}

	// inherited_blocking should be present.
	ib, ok := result["inherited_blocking"].(map[string]any)
	if !ok {
		t.Fatalf("expected inherited_blocking object, got %T (%v)", result["inherited_blocking"], result["inherited_blocking"])
	}

	ancestorID, ok := ib["ancestor_id"].(string)
	if !ok || ancestorID != epicID {
		t.Errorf("expected ancestor_id %q, got %v", epicID, ib["ancestor_id"])
	}

	blockerIDs, ok := ib["blocker_ids"].([]any)
	if !ok || len(blockerIDs) == 0 {
		t.Fatalf("expected non-empty blocker_ids array, got %v", ib["blocker_ids"])
	}
	if blockerIDs[0].(string) != blockerID {
		t.Errorf("expected blocker_ids[0] %q, got %v", blockerID, blockerIDs[0])
	}
}

func TestE2E_ShowJSON_NoInheritedBlocking_WhenAncestorNotBlocked(t *testing.T) {
	// Given — an epic without any blockers, with a child task.
	dir := initDB(t, "IB")
	author := "no-blocking"

	epicStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Normal epic",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create epic failed")
	}
	epicID := parseJSON(t, epicStdout)["id"].(string)

	childID := createTaskWithParent(t, dir, "Normal child", author, epicID)

	// When — show the child task.
	stdout, stderr, code := runNP(t, dir, "show", childID, "--json")

	// Then — inherited_blocking should NOT be present.
	if code != 0 {
		t.Fatalf("show --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	if _, found := result["inherited_blocking"]; found {
		t.Error("expected no inherited_blocking when ancestor is not blocked")
	}
}

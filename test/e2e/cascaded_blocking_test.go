//go:build e2e

package e2e_test

import (
	"testing"
)

func TestE2E_CascadedBlocking_TaskUnderBlockedEpicNotReady(t *testing.T) {
	// Given — Epic A blocks Epic B; a task is created under Epic B.
	dir := initDB(t, "CB")
	author := "cascade-agent"

	epicAStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic A (blocker)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic A failed")
	}
	epicAID := parseJSON(t, epicAStdout)["id"].(string)

	epicBStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic B (blocked)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic B failed")
	}
	epicBID := parseJSON(t, epicBStdout)["id"].(string)

	_, stderr, code := runNP(t, dir, "rel", "add", epicBID, "blocked_by", epicAID,
		"--author", author)
	if code != 0 {
		t.Fatalf("precondition: rel add failed (exit %d): %s", code, stderr)
	}

	taskID := createTaskWithParent(t, dir, "Task under blocked epic", author, epicBID)

	// When — show the task.
	shown := showIssue(t, dir, taskID)

	// Then — the task is not ready.
	if shown["is_ready"] == true {
		t.Error("expected task under blocked epic to not be ready")
	}
}

func TestE2E_CascadedBlocking_TaskBecomesReadyWhenBlockerClosed(t *testing.T) {
	// Given — Epic A blocks Epic B; a task is created under Epic B.
	dir := initDB(t, "CR")
	author := "cascade-agent"

	epicAStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic A (to close)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic A failed")
	}
	epicAID := parseJSON(t, epicAStdout)["id"].(string)

	epicBStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic B (blocked then unblocked)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic B failed")
	}
	epicBID := parseJSON(t, epicBStdout)["id"].(string)

	_, stderr, code := runNP(t, dir, "rel", "add", epicBID, "blocked_by", epicAID,
		"--author", author)
	if code != 0 {
		t.Fatalf("precondition: rel add failed (exit %d): %s", code, stderr)
	}

	taskID := createTaskWithParent(t, dir, "Task awaiting unblock", author, epicBID)

	// Verify task is initially not ready.
	before := showIssue(t, dir, taskID)
	if before["is_ready"] == true {
		t.Fatal("precondition: task should not be ready before blocker is closed")
	}

	// When — close Epic A (the blocker) by adding a child, closing it,
	// then closing the epic via close-eligible.
	childOfA := createTaskWithParent(t, dir, "Child of A", author, epicAID)
	claimID := claimIssue(t, dir, childOfA, author)
	_, stderr, code = runNP(t, dir, "done", "--claim", claimID,
		"--author", author, "--reason", "done")
	if code != 0 {
		t.Fatalf("precondition: close child of A failed (exit %d): %s", code, stderr)
	}
	_, stderr, code = runNP(t, dir, "epic", "close-eligible", "--author", author)
	if code != 0 {
		t.Fatalf("precondition: close-eligible failed (exit %d): %s", code, stderr)
	}

	// Then — the task under B is now ready.
	after := showIssue(t, dir, taskID)
	if after["is_ready"] != true {
		t.Error("expected task to become ready after blocker epic is closed")
	}

	// inherited_blocking should no longer be present.
	if _, found := after["inherited_blocking"]; found {
		t.Error("expected no inherited_blocking after blocker is closed")
	}
}

func TestE2E_CascadedBlocking_BlockedListIncludesAncestorBlockedTasks(t *testing.T) {
	// Given — Epic A blocks Epic B; a task is created under Epic B.
	dir := initDB(t, "BL")
	author := "cascade-agent"

	epicAStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Blocking epic",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic A failed")
	}
	epicAID := parseJSON(t, epicAStdout)["id"].(string)

	epicBStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Blocked epic",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic B failed")
	}
	epicBID := parseJSON(t, epicBStdout)["id"].(string)

	_, stderr, code := runNP(t, dir, "rel", "add", epicBID, "blocked_by", epicAID,
		"--author", author)
	if code != 0 {
		t.Fatalf("precondition: rel add failed (exit %d): %s", code, stderr)
	}

	taskID := createTaskWithParent(t, dir, "Ancestor-blocked task", author, epicBID)

	// When — list blocked issues.
	stdout, stderr, code := runNP(t, dir, "blocked", "--json")

	// Then — the task appears in the blocked list.
	if code != 0 {
		t.Fatalf("blocked --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)
	items, ok := result["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %v", result["items"])
	}

	var foundTask bool
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["id"] == taskID {
			foundTask = true
			if item["display_status"] != "blocked" {
				t.Errorf("expected display_status 'blocked', got %v", item["display_status"])
			}
			break
		}
	}
	if !foundTask {
		t.Errorf("expected task %s in blocked list, but it was not found", taskID)
	}
}

func TestE2E_CascadedBlocking_ShowSurfacesInheritedBlockingReason(t *testing.T) {
	// Given — Epic A blocks Epic B; a task is created under Epic B.
	dir := initDB(t, "SB")
	author := "cascade-agent"

	epicAStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic A (blocker for show)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic A failed")
	}
	epicAID := parseJSON(t, epicAStdout)["id"].(string)

	epicBStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic B (blocked for show)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic B failed")
	}
	epicBID := parseJSON(t, epicBStdout)["id"].(string)

	_, stderr, code := runNP(t, dir, "rel", "add", epicBID, "blocked_by", epicAID,
		"--author", author)
	if code != 0 {
		t.Fatalf("precondition: rel add failed (exit %d): %s", code, stderr)
	}

	taskID := createTaskWithParent(t, dir, "Task for show audit", author, epicBID)

	// When — show the task as JSON.
	shown := showIssue(t, dir, taskID)

	// Then — inherited_blocking surfaces the blocking reason.
	ib, ok := shown["inherited_blocking"].(map[string]any)
	if !ok {
		t.Fatalf("expected inherited_blocking, got %v", shown["inherited_blocking"])
	}

	if ib["ancestor_id"] != epicBID {
		t.Errorf("expected ancestor_id %q, got %v", epicBID, ib["ancestor_id"])
	}

	blockerIDs, ok := ib["blocker_ids"].([]any)
	if !ok || len(blockerIDs) == 0 {
		t.Fatalf("expected non-empty blocker_ids, got %v", ib["blocker_ids"])
	}
	if blockerIDs[0] != epicAID {
		t.Errorf("expected blocker_ids[0] %q, got %v", epicAID, blockerIDs[0])
	}
}

func TestE2E_CascadedBlocking_NestedCascading_GrandchildNotReady(t *testing.T) {
	// Given — Epic A blocks Epic B; Epic C is a child of B; a task is under C.
	dir := initDB(t, "NC")
	author := "cascade-agent"

	epicAStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic A (root blocker)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic A failed")
	}
	epicAID := parseJSON(t, epicAStdout)["id"].(string)

	epicBStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic B (blocked)",
		"--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic B failed")
	}
	epicBID := parseJSON(t, epicBStdout)["id"].(string)

	_, stderr, code := runNP(t, dir, "rel", "add", epicBID, "blocked_by", epicAID,
		"--author", author)
	if code != 0 {
		t.Fatalf("precondition: rel add failed (exit %d): %s", code, stderr)
	}

	// Epic C is a child of B.
	epicCStdout, stderr, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Epic C (child of B)",
		"--author", author, "--parent", epicBID, "--json")
	if code != 0 {
		t.Fatalf("precondition: create Epic C failed (exit %d): %s", code, stderr)
	}
	epicCID := parseJSON(t, epicCStdout)["id"].(string)

	// Task under C.
	taskID := createTaskWithParent(t, dir, "Grandchild task", author, epicCID)

	// When — show the task.
	shown := showIssue(t, dir, taskID)

	// Then — the task is not ready (cascaded from A → B → C → task).
	if shown["is_ready"] == true {
		t.Error("expected grandchild task under nested blocked epic to not be ready")
	}

	// inherited_blocking should reference Epic B (the directly blocked ancestor).
	ib, ok := shown["inherited_blocking"].(map[string]any)
	if !ok {
		t.Fatalf("expected inherited_blocking, got %v", shown["inherited_blocking"])
	}

	if ib["ancestor_id"] != epicBID {
		t.Errorf("expected ancestor_id %q (Epic B, the blocked one), got %v", epicBID, ib["ancestor_id"])
	}
}

//go:build e2e

package e2e_test

import "testing"

func TestE2E_EpicWithChildren_NotReady(t *testing.T) {
	// Given — an epic with child tasks (already decomposed).
	dir := initDB(t, "RDY")
	author := "readiness-agent"
	epicID, _ := seedEpicWithTasks(t, dir, "Decomposed epic", author,
		"Child task A", "Child task B",
	)

	// When — check the epic's readiness via show.
	epic := showTicket(t, dir, epicID)

	// Then — the epic should NOT be ready because it has children.
	if epic["is_ready"] == true {
		t.Errorf("epic with children should not be ready, got is_ready=true")
	}
}

func TestE2E_EpicWithChildren_ExcludedFromNextAndList(t *testing.T) {
	// Given — an epic with a child task and a standalone open task.
	dir := initDB(t, "RDY")
	author := "readiness-agent"
	epicID, _ := seedEpicWithTasks(t, dir, "Has children", author, "Child A")
	createTask(t, dir, "Standalone task", author)

	// When — list ready tickets.
	stdout, stderr, code := runNP(t, dir, "list", "--ready", "--json")
	if code != 0 {
		t.Fatalf("list --ready failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	// Then — no epic with children should appear in the ready list.
	items, ok := result["items"].([]any)
	if !ok {
		t.Fatal("expected items array")
	}

	for _, item := range items {
		m := item.(map[string]any)
		if m["role"] == "epic" {
			t.Errorf("epic with children should not appear in --ready list: %v", m["title"])
		}
	}

	// When — "claim ready" should claim a task, not the epic.
	nextStdout, stderr, code := runNP(t, dir, "claim", "ready", "--author", author, "--json")
	if code != 0 {
		t.Fatalf("claim ready failed (exit %d): %s", code, stderr)
	}
	nextResult := parseJSON(t, nextStdout)
	if nextResult["ticket_id"] == epicID {
		t.Errorf("next should not claim epic with children %s", epicID)
	}
}

func TestE2E_ChildlessEpic_IsReady(t *testing.T) {
	// Given — an epic with no children (needs decomposition).
	dir := initDB(t, "RDY")
	author := "readiness-agent"
	epicID := createEpic(t, dir, "Needs decomposition", author)

	// When — check the epic's readiness.
	epic := showTicket(t, dir, epicID)

	// Then — the childless epic should be ready.
	if epic["is_ready"] != true {
		t.Errorf("childless active epic should be ready, got is_ready=%v", epic["is_ready"])
	}
}

//go:build e2e

// Package e2e_test contains end-to-end tests that exercise the np binary
// through realistic multi-step workflows. Each test simulates a complete
// usage pattern — from database initialization through ticket lifecycle
// to final state verification.
package e2e_test

import (
	"encoding/json"
	"testing"
)

// parseJSON unmarshals JSON stdout into a generic map, failing the test if
// the output is not valid JSON.
func parseJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout)
	}
	return result
}

// initDB creates a temporary directory, initializes an np database with the
// given prefix, and returns the directory path. The test fails if init does
// not succeed.
func initDB(t *testing.T, prefix string) string {
	t.Helper()

	dir := t.TempDir()
	_, stderr, code := runNP(t, dir, "init", prefix)
	if code != 0 {
		t.Fatalf("init failed (exit %d): %s", code, stderr)
	}
	return dir
}

// createTask creates a task ticket and returns its ID. The test fails if
// creation does not succeed.
func createTask(t *testing.T, dir, title, author string) string {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", title,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("create task %q failed (exit %d): %s", title, code, stderr)
	}
	result := parseJSON(t, stdout)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("create task %q: missing id in response", title)
	}
	return id
}

// claimTicket claims a ticket and returns the claim ID. The test fails if
// claiming does not succeed.
func claimTicket(t *testing.T, dir, ticketID, author string) string {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "claim", ticketID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("claim %s failed (exit %d): %s", ticketID, code, stderr)
	}
	result := parseJSON(t, stdout)
	claimID, ok := result["claim_id"].(string)
	if !ok || claimID == "" {
		t.Fatalf("claim %s: missing claim_id in response", ticketID)
	}
	return claimID
}

// showTicket returns the full JSON representation of a ticket. The test
// fails if the show command does not succeed.
func showTicket(t *testing.T, dir, ticketID string) map[string]any {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "show", ticketID, "--json")
	if code != 0 {
		t.Fatalf("show %s failed (exit %d): %s", ticketID, code, stderr)
	}
	return parseJSON(t, stdout)
}

func TestE2E_TaskLifecycle_CreateClaimUpdateClose(t *testing.T) {
	// Given — a fresh database with a task ticket.
	dir := initDB(t, "WF")
	author := "workflow-agent"
	taskID := createTask(t, dir, "Implement feature X", author)

	// When — claim the task, update its fields, add a note, and close it.
	claimID := claimTicket(t, dir, taskID, author)

	_, stderr, code := runNP(t, dir, "update", taskID,
		"--claim-id", claimID,
		"--title", "Implement feature X (revised)",
		"--description", "Updated after investigation",
		"--priority", "P1",
		"--json",
	)
	if code != 0 {
		t.Fatalf("update failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "note", "add",
		"--ticket", taskID,
		"--body", "Root cause found in auth module",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("note add failed (exit %d): %s", code, stderr)
	}

	closeStdout, stderr, code := runNP(t, dir, "close", taskID,
		"--claim-id", claimID,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close failed (exit %d): %s", code, stderr)
	}

	// Then — the task should be closed with updated fields.
	closeResult := parseJSON(t, closeStdout)
	if closeResult["action"] != "close" {
		t.Errorf("expected action 'close', got %v", closeResult["action"])
	}

	ticket := showTicket(t, dir, taskID)
	if ticket["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", ticket["state"])
	}
	if ticket["title"] != "Implement feature X (revised)" {
		t.Errorf("expected revised title, got %v", ticket["title"])
	}
	if ticket["priority"] != "P1" {
		t.Errorf("expected priority P1, got %v", ticket["priority"])
	}
	if ticket["description"] != "Updated after investigation" {
		t.Errorf("expected updated description, got %v", ticket["description"])
	}
}

func TestE2E_TaskLifecycle_ClaimReleaseReclaimClose(t *testing.T) {
	// Given — a task that has been created and claimed once.
	dir := initDB(t, "WF")
	agent1 := "agent-alpha"
	agent2 := "agent-beta"
	taskID := createTask(t, dir, "Refactor logging", agent1)

	// When — agent1 claims and releases, then agent2 claims and closes.
	claim1 := claimTicket(t, dir, taskID, agent1)

	_, stderr, code := runNP(t, dir, "release", taskID,
		"--claim-id", claim1,
		"--json",
	)
	if code != 0 {
		t.Fatalf("release failed (exit %d): %s", code, stderr)
	}

	claim2 := claimTicket(t, dir, taskID, agent2)

	_, stderr, code = runNP(t, dir, "close", taskID,
		"--claim-id", claim2,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close failed (exit %d): %s", code, stderr)
	}

	// Then — the task is closed and the history shows both agents' actions.
	ticket := showTicket(t, dir, taskID)
	if ticket["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", ticket["state"])
	}

	histStdout, stderr, code := runNP(t, dir, "history", taskID, "--json")
	if code != 0 {
		t.Fatalf("history failed (exit %d): %s", code, stderr)
	}
	histResult := parseJSON(t, histStdout)
	totalCount, ok := histResult["total_count"].(float64)
	if !ok || totalCount < 4 {
		t.Errorf("expected at least 4 history entries (create, claim, release, claim, close), got %v", histResult["total_count"])
	}
}

func TestE2E_TaskLifecycle_DeferAndWait(t *testing.T) {
	// Given — two tasks, one to be deferred and one to be marked as waiting.
	dir := initDB(t, "WF")
	author := "lifecycle-agent"
	deferID := createTask(t, dir, "Low priority polish", author)
	waitID := createTask(t, dir, "Blocked on external API", author)

	// When — defer one task and mark the other as waiting.
	deferClaim := claimTicket(t, dir, deferID, author)
	_, stderr, code := runNP(t, dir, "defer", deferID,
		"--claim-id", deferClaim,
		"--json",
	)
	if code != 0 {
		t.Fatalf("defer failed (exit %d): %s", code, stderr)
	}

	waitClaim := claimTicket(t, dir, waitID, author)
	_, stderr, code = runNP(t, dir, "wait", waitID,
		"--claim-id", waitClaim,
		"--json",
	)
	if code != 0 {
		t.Fatalf("wait failed (exit %d): %s", code, stderr)
	}

	// Then — each task reflects the correct terminal/non-terminal state.
	deferTicket := showTicket(t, dir, deferID)
	if deferTicket["state"] != "deferred" {
		t.Errorf("expected deferred state, got %v", deferTicket["state"])
	}

	waitTicket := showTicket(t, dir, waitID)
	if waitTicket["state"] != "waiting" {
		t.Errorf("expected waiting state, got %v", waitTicket["state"])
	}
}

func TestE2E_EpicWithChildren_DerivedCompletion(t *testing.T) {
	// Given — an epic with two child tasks.
	dir := initDB(t, "WF")
	author := "epic-agent"

	epicStdout, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Authentication overhaul",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("create epic failed (exit %d): %s", code, stderr)
	}
	epicID := parseJSON(t, epicStdout)["id"].(string)

	child1 := createTaskWithParent(t, dir, "Add OAuth provider", author, epicID)
	child2 := createTaskWithParent(t, dir, "Write integration tests", author, epicID)

	// When — close the first child; the epic should not yet be complete.
	claim1 := claimTicket(t, dir, child1, author)
	_, stderr, code = runNP(t, dir, "close", child1,
		"--claim-id", claim1,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close child1 failed (exit %d): %s", code, stderr)
	}

	epicAfterOne := showTicket(t, dir, epicID)
	if epicAfterOne["is_complete"] == true {
		t.Error("epic should not be complete with one open child")
	}

	// Close the second child — the epic should now be complete.
	claim2 := claimTicket(t, dir, child2, author)
	_, stderr, code = runNP(t, dir, "close", child2,
		"--claim-id", claim2,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close child2 failed (exit %d): %s", code, stderr)
	}

	// Then — the epic is derived-complete.
	epicAfterAll := showTicket(t, dir, epicID)
	if epicAfterAll["is_complete"] != true {
		t.Errorf("epic should be complete after all children closed, got is_complete=%v", epicAfterAll["is_complete"])
	}
}

// createTaskWithParent creates a task under a parent epic and returns its ID.
func createTaskWithParent(t *testing.T, dir, title, author, parentID string) string {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", title,
		"--author", author,
		"--parent", parentID,
		"--json",
	)
	if code != 0 {
		t.Fatalf("create task %q (parent %s) failed (exit %d): %s", title, parentID, code, stderr)
	}
	result := parseJSON(t, stdout)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("create task %q: missing id in response", title)
	}
	return id
}

func TestE2E_AtomicEdit(t *testing.T) {
	// Given — a task that needs a quick one-shot edit without manual claiming.
	dir := initDB(t, "WF")
	author := "edit-agent"
	taskID := createTask(t, dir, "Original title", author)

	// When — use the edit command for an atomic claim-update-release.
	_, stderr, code := runNP(t, dir, "edit", taskID,
		"--author", author,
		"--title", "Revised via edit",
		"--description", "Quick fix applied",
		"--json",
	)
	if code != 0 {
		t.Fatalf("edit failed (exit %d): %s", code, stderr)
	}

	// Then — the ticket is updated but not claimed (edit releases automatically).
	ticket := showTicket(t, dir, taskID)
	if ticket["title"] != "Revised via edit" {
		t.Errorf("expected 'Revised via edit', got %v", ticket["title"])
	}
	if ticket["description"] != "Quick fix applied" {
		t.Errorf("expected 'Quick fix applied', got %v", ticket["description"])
	}
	if ticket["state"] != "open" {
		t.Errorf("expected state 'open' after edit (auto-release), got %v", ticket["state"])
	}
}

func TestE2E_NotesOnClosedTicket(t *testing.T) {
	// Given — a task that has been closed.
	dir := initDB(t, "WF")
	author := "notes-agent"
	taskID := createTask(t, dir, "Closed task", author)
	claimID := claimTicket(t, dir, taskID, author)

	_, stderr, code := runNP(t, dir, "close", taskID,
		"--claim-id", claimID,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close failed (exit %d): %s", code, stderr)
	}

	// When — add notes to the closed ticket (notes don't require claiming).
	_, stderr, code = runNP(t, dir, "note", "add",
		"--ticket", taskID,
		"--body", "Post-mortem: root cause was a race condition",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("note add on closed ticket failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "note", "add",
		"--ticket", taskID,
		"--body", "Follow-up ticket created for monitoring",
		"--author", "other-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("second note add failed (exit %d): %s", code, stderr)
	}

	// Then — both notes are present on the closed ticket.
	noteStdout, stderr, code := runNP(t, dir, "note", "list", "--ticket", taskID, "--json")
	if code != 0 {
		t.Fatalf("note list failed (exit %d): %s", code, stderr)
	}
	noteResult := parseJSON(t, noteStdout)
	noteCount, ok := noteResult["total_count"].(float64)
	if !ok || noteCount != 2 {
		t.Errorf("expected 2 notes on closed ticket, got %v", noteResult["total_count"])
	}
}

func TestE2E_RelationshipsAndSearch(t *testing.T) {
	// Given — two tasks where one blocks the other, plus a third with a
	// citation relationship.
	dir := initDB(t, "WF")
	author := "relate-agent"
	blockerID := createTask(t, dir, "Fix database migration", author)
	blockedID := createTask(t, dir, "Deploy new schema", author)
	referenceID := createTask(t, dir, "Document migration steps", author)

	// When — establish relationships.
	_, stderr, code := runNP(t, dir, "relate", "add", blockedID,
		"blocked_by", blockerID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("relate blocked_by failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "relate", "add", referenceID,
		"cites", blockerID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("relate cites failed (exit %d): %s", code, stderr)
	}

	// Then — the blocked ticket shows the relationship and is not ready.
	blockedTicket := showTicket(t, dir, blockedID)
	if blockedTicket["is_ready"] == true {
		t.Error("blocked ticket should not be ready while blocker is open")
	}

	rels, ok := blockedTicket["relationships"].([]any)
	if !ok || len(rels) == 0 {
		t.Fatalf("expected relationships on blocked ticket, got %v", blockedTicket["relationships"])
	}

	// Search should find the migration-related tickets.
	searchStdout, stderr, code := runNP(t, dir, "search", "migration", "--json")
	if code != 0 {
		t.Fatalf("search failed (exit %d): %s", code, stderr)
	}
	searchResult := parseJSON(t, searchStdout)
	searchCount, ok := searchResult["total_count"].(float64)
	if !ok || searchCount < 2 {
		t.Errorf("expected at least 2 search results for 'migration', got %v", searchResult["total_count"])
	}
}

func TestE2E_CreateClaimWithFlag(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "WF")
	author := "flag-agent"

	// When — create a task with the --claim flag for atomic create-and-claim.
	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Urgent hotfix",
		"--author", author,
		"--claim",
		"--json",
	)
	if code != 0 {
		t.Fatalf("create --claim failed (exit %d): %s", code, stderr)
	}

	// Then — the response includes a claim_id and the ticket is claimed.
	result := parseJSON(t, stdout)
	taskID, ok := result["id"].(string)
	if !ok || taskID == "" {
		t.Fatal("missing id in create --claim response")
	}
	claimID, ok := result["claim_id"].(string)
	if !ok || claimID == "" {
		t.Fatal("expected claim_id in create --claim response")
	}

	ticket := showTicket(t, dir, taskID)
	if ticket["state"] != "claimed" {
		t.Errorf("expected state 'claimed', got %v", ticket["state"])
	}

	// Clean up — close the ticket so the claim is released.
	_, stderr, code = runNP(t, dir, "close", taskID,
		"--claim-id", claimID,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close failed (exit %d): %s", code, stderr)
	}
}

func TestE2E_HistoryAuditTrail(t *testing.T) {
	// Given — a task that goes through several lifecycle steps.
	dir := initDB(t, "WF")
	author := "audit-agent"
	taskID := createTask(t, dir, "Audit trail test", author)

	// When — claim, update, release, re-claim, close.
	claim1 := claimTicket(t, dir, taskID, author)

	_, _, _ = runNP(t, dir, "update", taskID,
		"--claim-id", claim1,
		"--title", "Audit trail test (updated)",
		"--json",
	)
	_, _, _ = runNP(t, dir, "release", taskID,
		"--claim-id", claim1,
		"--json",
	)

	claim2 := claimTicket(t, dir, taskID, author)
	_, _, _ = runNP(t, dir, "close", taskID,
		"--claim-id", claim2,
		"--json",
	)

	histStdout, stderr, code := runNP(t, dir, "history", taskID, "--json")
	if code != 0 {
		t.Fatalf("history failed (exit %d): %s", code, stderr)
	}

	// Then — the history should contain entries for every mutation.
	histResult := parseJSON(t, histStdout)
	entries, ok := histResult["entries"].([]any)
	if !ok {
		t.Fatalf("expected entries array, got %T", histResult["entries"])
	}

	// Expect at least: created, claimed, updated, released, claimed, state_changed (close).
	if len(entries) < 6 {
		t.Errorf("expected at least 6 history entries, got %d", len(entries))
	}

	// Verify the first entry is a creation event.
	firstEntry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map for first entry, got %T", entries[0])
	}
	if firstEntry["event_type"] != "created" {
		t.Errorf("expected first entry event_type 'created', got %v", firstEntry["event_type"])
	}
}

func TestE2E_NextClaimsHighestPriorityReady(t *testing.T) {
	// Given — three tasks with different priorities.
	dir := initDB(t, "WF")
	author := "priority-agent"

	createTaskWithPriority(t, dir, "Low priority", author, "P3")
	highID := createTaskWithPriority(t, dir, "High priority", author, "P0")
	createTaskWithPriority(t, dir, "Medium priority", author, "P2")

	// When — next claims the highest-priority ready ticket.
	stdout, stderr, code := runNP(t, dir, "next",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("next failed (exit %d): %s", code, stderr)
	}

	// Then — the claimed ticket should be the P0 one.
	result := parseJSON(t, stdout)
	if result["ticket_id"] != highID {
		t.Errorf("expected next to claim P0 ticket %s, got %v", highID, result["ticket_id"])
	}
}

// createTaskWithPriority creates a task with the specified priority and
// returns its ID.
func createTaskWithPriority(t *testing.T, dir, title, author, priority string) string {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", title,
		"--author", author,
		"--priority", priority,
		"--json",
	)
	if code != 0 {
		t.Fatalf("create task %q with priority %s failed (exit %d): %s", title, priority, code, stderr)
	}
	result := parseJSON(t, stdout)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("create task %q: missing id in response", title)
	}
	return id
}

func TestE2E_FacetsAndFiltering(t *testing.T) {
	// Given — tasks with facets.
	dir := initDB(t, "WF")
	author := "facet-agent"

	// Create tasks using edit to set facets (create supports --facet).
	_, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Fix login bug",
		"--author", author,
		"--facet", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with facet failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Add dashboard feature",
		"--author", author,
		"--facet", "kind:feature",
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with facet failed (exit %d): %s", code, stderr)
	}

	// When — list with facet filter.
	stdout, stderr, code := runNP(t, dir, "list",
		"--ready",
		"--facet", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("list --facet failed (exit %d): %s", code, stderr)
	}

	// Then — only the fix ticket should appear.
	result := parseJSON(t, stdout)
	totalCount, ok := result["total_count"].(float64)
	if !ok || totalCount != 1 {
		t.Errorf("expected 1 ticket with facet kind:fix, got %v", result["total_count"])
	}

	items, ok := result["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatal("expected items array with one entry")
	}
	firstItem := items[0].(map[string]any)
	if firstItem["title"] != "Fix login bug" {
		t.Errorf("expected 'Fix login bug', got %v", firstItem["title"])
	}
}

func TestE2E_DoctorDiagnostics(t *testing.T) {
	// Given — a healthy database with a couple of tickets.
	dir := initDB(t, "WF")
	author := "doctor-agent"
	createTask(t, dir, "Healthy task A", author)
	createTask(t, dir, "Healthy task B", author)

	// When — run doctor diagnostics.
	_, stderr, code := runNP(t, dir, "doctor", "--json")

	// Then — doctor should succeed on a healthy database.
	if code != 0 {
		t.Errorf("doctor failed on healthy database (exit %d): %s", code, stderr)
	}
}

func TestE2E_DeleteAndGC(t *testing.T) {
	// Given — a claimed task that will be deleted.
	dir := initDB(t, "WF")
	author := "gc-agent"
	taskID := createTask(t, dir, "Ephemeral task", author)
	claimID := claimTicket(t, dir, taskID, author)

	// When — soft-delete the task, then garbage-collect.
	_, stderr, code := runNP(t, dir, "delete", taskID,
		"--claim-id", claimID,
		"--confirm",
		"--json",
	)
	if code != 0 {
		t.Fatalf("delete failed (exit %d): %s", code, stderr)
	}

	// The ticket should no longer appear in list.
	listStdout, _, listCode := runNP(t, dir, "list", "--json")
	if listCode != 0 {
		t.Fatalf("list failed after delete (exit %d)", listCode)
	}
	listResult := parseJSON(t, listStdout)
	totalCount, _ := listResult["total_count"].(float64)
	if totalCount != 0 {
		t.Errorf("expected 0 tickets after delete, got %v", totalCount)
	}

	// GC should succeed.
	_, stderr, code = runNP(t, dir, "gc", "--confirm", "--json")
	if code != 0 {
		t.Errorf("gc failed (exit %d): %s", code, stderr)
	}
}

func TestE2E_BlockedByRelationship_ReadinessGating(t *testing.T) {
	// Given — task B is blocked by task A.
	dir := initDB(t, "WF")
	author := "block-agent"
	blockerID := createTask(t, dir, "Prerequisite work", author)
	blockedID := createTask(t, dir, "Dependent work", author)

	_, stderr, code := runNP(t, dir, "relate", "add", blockedID,
		"blocked_by", blockerID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("relate add failed (exit %d): %s", code, stderr)
	}

	// When — check readiness before and after closing the blocker.
	blockedBefore := showTicket(t, dir, blockedID)

	blockerClaim := claimTicket(t, dir, blockerID, author)
	_, stderr, code = runNP(t, dir, "close", blockerID,
		"--claim-id", blockerClaim,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close blocker failed (exit %d): %s", code, stderr)
	}

	blockedAfter := showTicket(t, dir, blockedID)

	// Then — the blocked ticket becomes ready only after its blocker closes.
	if blockedBefore["is_ready"] == true {
		t.Error("blocked ticket should not be ready while blocker is open")
	}
	if blockedAfter["is_ready"] != true {
		t.Error("blocked ticket should be ready after blocker is closed")
	}
}

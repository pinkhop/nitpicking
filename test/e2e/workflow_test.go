//go:build e2e

// Package e2e_test contains end-to-end tests that exercise the np binary
// through realistic multi-step workflows. Each test simulates a complete
// usage pattern — from database initialization through issue lifecycle
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

// createTask creates a task issue and returns its ID. The test fails if
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

// claimIssue claims an issue and returns the claim ID. The test fails if
// claiming does not succeed.
func claimIssue(t *testing.T, dir, issueID, author string) string {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "claim", "id", issueID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("claim %s failed (exit %d): %s", issueID, code, stderr)
	}
	result := parseJSON(t, stdout)
	claimID, ok := result["claim_id"].(string)
	if !ok || claimID == "" {
		t.Fatalf("claim %s: missing claim_id in response", issueID)
	}
	return claimID
}

// showIssue returns the full JSON representation of an issue. The test
// fails if the show command does not succeed.
func showIssue(t *testing.T, dir, issueID string) map[string]any {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "show", issueID, "--json")
	if code != 0 {
		t.Fatalf("show %s failed (exit %d): %s", issueID, code, stderr)
	}
	return parseJSON(t, stdout)
}

func TestE2E_TaskLifecycle_CreateClaimUpdateClose(t *testing.T) {
	// Given — a fresh database with a task issue.
	dir := initDB(t, "WF")
	author := "workflow-agent"
	taskID := createTask(t, dir, "Implement feature X", author)

	// When — claim the task, update its fields, add a comment, and close it.
	claimID := claimIssue(t, dir, taskID, author)

	_, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--title", "Implement feature X (revised)",
		"--description", "Updated after investigation",
		"--priority", "P1",
		"--json",
	)
	if code != 0 {
		t.Fatalf("update failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "comment", "add",
		"--issue", taskID,
		"--body", "Root cause found in auth module",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("comment add failed (exit %d): %s", code, stderr)
	}

	closeStdout, stderr, code := runNP(t, dir, "issue", "close", taskID,
		"--claim", claimID,
		"--author", author,
		"--reason", "test close",
		"--json",
	)
	if code != 0 {
		t.Fatalf("close failed (exit %d): %s", code, stderr)
	}

	// Then — the task should be closed with updated fields.
	closeResult := parseJSON(t, closeStdout)
	if closeResult["action"] != "done" {
		t.Errorf("expected action 'done', got %v", closeResult["action"])
	}

	issue := showIssue(t, dir, taskID)
	if issue["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", issue["state"])
	}
	if issue["title"] != "Implement feature X (revised)" {
		t.Errorf("expected revised title, got %v", issue["title"])
	}
	if issue["priority"] != "P1" {
		t.Errorf("expected priority P1, got %v", issue["priority"])
	}
	if issue["description"] != "Updated after investigation" {
		t.Errorf("expected updated description, got %v", issue["description"])
	}
}

func TestE2E_TaskLifecycle_ClaimReleaseReclaimClose(t *testing.T) {
	// Given — a task that has been created and claimed once.
	dir := initDB(t, "WF")
	agent1 := "agent-alpha"
	agent2 := "agent-beta"
	taskID := createTask(t, dir, "Refactor logging", agent1)

	// When — agent1 claims and releases, then agent2 claims and closes.
	claim1 := claimIssue(t, dir, taskID, agent1)

	_, stderr, code := runNP(t, dir, "issue", "release", taskID,
		"--claim", claim1,
		"--json",
	)
	if code != 0 {
		t.Fatalf("release failed (exit %d): %s", code, stderr)
	}

	claim2 := claimIssue(t, dir, taskID, agent2)

	_, stderr, code = runNP(t, dir, "issue", "close", taskID,
		"--claim", claim2,
		"--author", agent2,
		"--reason", "test close",
		"--json",
	)
	if code != 0 {
		t.Fatalf("close failed (exit %d): %s", code, stderr)
	}

	// Then — the task is closed and the history shows both agents' actions.
	issue := showIssue(t, dir, taskID)
	if issue["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", issue["state"])
	}

	histStdout, stderr, code := runNP(t, dir, "issue", "history", taskID, "--json")
	if code != 0 {
		t.Fatalf("history failed (exit %d): %s", code, stderr)
	}
	histResult := parseJSON(t, histStdout)
	entries, ok := histResult["entries"].([]any)
	if !ok || len(entries) < 4 {
		t.Errorf("expected at least 4 history entries (create, claim, release, claim, close), got %d", len(entries))
	}
}

func TestE2E_TaskLifecycle_Defer(t *testing.T) {
	// Given — a task to be deferred.
	dir := initDB(t, "WF")
	author := "lifecycle-agent"
	deferID := createTask(t, dir, "Low priority polish", author)

	// When — defer the task.
	deferClaim := claimIssue(t, dir, deferID, author)
	_, stderr, code := runNP(t, dir, "issue", "defer", deferID,
		"--claim", deferClaim,
		"--json",
	)
	if code != 0 {
		t.Fatalf("defer failed (exit %d): %s", code, stderr)
	}

	// Then — the task is in deferred state.
	deferIssue := showIssue(t, dir, deferID)
	if deferIssue["state"] != "deferred" {
		t.Errorf("expected deferred state, got %v", deferIssue["state"])
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

	// When — close both children, then close the epic.
	claim1 := claimIssue(t, dir, child1, author)
	_, stderr, code = runNP(t, dir, "issue", "close", child1,
		"--claim", claim1, "--author", author, "--reason", "test close", "--json")
	if code != 0 {
		t.Fatalf("close child1 failed (exit %d): %s", code, stderr)
	}

	// Closing epic with one open child should fail.
	epicClaim := claimIssue(t, dir, epicID, author)
	_, stderr, code = runNP(t, dir, "issue", "close", epicID,
		"--claim", epicClaim, "--author", author, "--reason", "test close", "--json")
	if code == 0 {
		t.Error("expected closing epic with open child to fail")
	}

	// Release the epic claim so we can reclaim later.
	_, stderr, code = runNP(t, dir, "issue", "release", epicID,
		"--claim", epicClaim, "--json")
	if code != 0 {
		t.Fatalf("release epic failed (exit %d): %s", code, stderr)
	}

	// Close the second child.
	claim2 := claimIssue(t, dir, child2, author)
	_, stderr, code = runNP(t, dir, "issue", "close", child2,
		"--claim", claim2, "--author", author, "--reason", "test close", "--json")
	if code != 0 {
		t.Fatalf("close child2 failed (exit %d): %s", code, stderr)
	}

	// Then — epic can now be closed since all children are closed.
	epicClaim = claimIssue(t, dir, epicID, author)
	_, stderr, code = runNP(t, dir, "issue", "close", epicID,
		"--claim", epicClaim, "--author", author, "--reason", "test close", "--json")
	if code != 0 {
		t.Fatalf("close epic failed (exit %d): %s", code, stderr)
	}

	epicAfterClose := showIssue(t, dir, epicID)
	if epicAfterClose["state"] != "closed" {
		t.Errorf("expected epic state 'closed', got %v", epicAfterClose["state"])
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
	_, stderr, code := runNP(t, dir, "issue", "edit", taskID,
		"--author", author,
		"--title", "Revised via edit",
		"--description", "Quick fix applied",
		"--json",
	)
	if code != 0 {
		t.Fatalf("edit failed (exit %d): %s", code, stderr)
	}

	// Then — the issue is updated but not claimed (edit releases automatically).
	issue := showIssue(t, dir, taskID)
	if issue["title"] != "Revised via edit" {
		t.Errorf("expected 'Revised via edit', got %v", issue["title"])
	}
	if issue["description"] != "Quick fix applied" {
		t.Errorf("expected 'Quick fix applied', got %v", issue["description"])
	}
	if issue["state"] != "open" {
		t.Errorf("expected state 'open' after edit (auto-release), got %v", issue["state"])
	}
}

func TestE2E_CommentsOnClosedIssue(t *testing.T) {
	// Given — a task that has been closed.
	dir := initDB(t, "WF")
	author := "comments-agent"
	taskID := createTask(t, dir, "Closed task", author)
	claimID := claimIssue(t, dir, taskID, author)

	_, stderr, code := runNP(t, dir, "issue", "close", taskID,
		"--claim", claimID,
		"--author", author,
		"--reason", "test close",
		"--json",
	)
	if code != 0 {
		t.Fatalf("close failed (exit %d): %s", code, stderr)
	}

	// When — add comments to the closed issue (comments don't require claiming).
	_, stderr, code = runNP(t, dir, "comment", "add",
		"--issue", taskID,
		"--body", "Post-mortem: root cause was a race condition",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("comment add on closed issue failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "comment", "add",
		"--issue", taskID,
		"--body", "Follow-up issue created for monitoring",
		"--author", "other-agent",
		"--json",
	)
	if code != 0 {
		t.Fatalf("second comment add failed (exit %d): %s", code, stderr)
	}

	// Then — both comments are present on the closed issue.
	commentStdout, stderr, code := runNP(t, dir, "comment", "list", "--issue", taskID, "--json")
	if code != 0 {
		t.Fatalf("comment list failed (exit %d): %s", code, stderr)
	}
	commentResult := parseJSON(t, commentStdout)
	comments, ok := commentResult["comments"].([]any)
	if !ok || len(comments) != 3 {
		t.Errorf("expected 3 comments on closed issue (1 close reason + 2 post-close), got %d", len(comments))
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
	_, stderr, code := runNP(t, dir, "rel", "add", blockedID,
		"blocked_by", blockerID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("relate blocked_by failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "rel", "add", referenceID,
		"refs", blockerID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("relate refs failed (exit %d): %s", code, stderr)
	}

	// Then — the blocked issue shows the relationship and is not ready.
	blockedIssue := showIssue(t, dir, blockedID)
	if blockedIssue["is_ready"] == true {
		t.Error("blocked issue should not be ready while blocker is open")
	}

	rels, ok := blockedIssue["relationships"].([]any)
	if !ok || len(rels) == 0 {
		t.Fatalf("expected relationships on blocked issue, got %v", blockedIssue["relationships"])
	}

	// Search should find the migration-related issues.
	searchStdout, stderr, code := runNP(t, dir, "search", "migration", "--json")
	if code != 0 {
		t.Fatalf("search failed (exit %d): %s", code, stderr)
	}
	searchResult := parseJSON(t, searchStdout)
	searchItems, ok := searchResult["items"].([]any)
	if !ok || len(searchItems) < 2 {
		t.Errorf("expected at least 2 search results for 'migration', got %d", len(searchItems))
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

	// Then — the response includes a claim_id and the issue is claimed.
	result := parseJSON(t, stdout)
	taskID, ok := result["id"].(string)
	if !ok || taskID == "" {
		t.Fatal("missing id in create --claim response")
	}
	claimID, ok := result["claim_id"].(string)
	if !ok || claimID == "" {
		t.Fatal("expected claim_id in create --claim response")
	}

	issue := showIssue(t, dir, taskID)
	if issue["state"] != "claimed" {
		t.Errorf("expected state 'claimed', got %v", issue["state"])
	}

	// Clean up — close the issue so the claim is released.
	_, stderr, code = runNP(t, dir, "issue", "close", taskID,
		"--claim", claimID,
		"--author", author,
		"--reason", "test close",
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
	claim1 := claimIssue(t, dir, taskID, author)

	_, _, _ = runNP(t, dir, "issue", "update", taskID,
		"--claim", claim1,
		"--title", "Audit trail test (updated)",
		"--json",
	)
	_, _, _ = runNP(t, dir, "issue", "release", taskID,
		"--claim", claim1,
		"--json",
	)

	claim2 := claimIssue(t, dir, taskID, author)
	_, _, _ = runNP(t, dir, "issue", "close", taskID,
		"--claim", claim2,
		"--author", author,
		"--reason", "test close",
		"--json",
	)

	histStdout, stderr, code := runNP(t, dir, "issue", "history", taskID, "--json")
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

	// When — "claim ready" claims the highest-priority ready issue.
	stdout, stderr, code := runNP(t, dir, "claim", "ready",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("claim ready failed (exit %d): %s", code, stderr)
	}

	// Then — the claimed issue should be the P0 one.
	result := parseJSON(t, stdout)
	if result["issue_id"] != highID {
		t.Errorf("expected next to claim P0 issue %s, got %v", highID, result["issue_id"])
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

func TestE2E_DimensionsAndFiltering(t *testing.T) {
	// Given — tasks with dimensions.
	dir := initDB(t, "WF")
	author := "dimension-agent"

	// Create tasks using edit to set dimensions (create supports --dimension).
	_, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Fix login bug",
		"--author", author,
		"--dimension", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with dimension failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "create",
		"--role", "task",
		"--title", "Add dashboard feature",
		"--author", author,
		"--dimension", "kind:feature",
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with dimension failed (exit %d): %s", code, stderr)
	}

	// When — list with dimension filter.
	stdout, stderr, code := runNP(t, dir, "list",
		"--ready",
		"--dimension", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("list --dimension failed (exit %d): %s", code, stderr)
	}

	// Then — only the fix issue should appear.
	result := parseJSON(t, stdout)
	items, ok := result["items"].([]any)
	if !ok || len(items) != 1 {
		t.Errorf("expected 1 issue with dimension kind:fix, got %d", len(items))
	}

	items, ok = result["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatal("expected items array with one entry")
	}
	firstItem := items[0].(map[string]any)
	if firstItem["title"] != "Fix login bug" {
		t.Errorf("expected 'Fix login bug', got %v", firstItem["title"])
	}
}

func TestE2E_DoctorDiagnostics(t *testing.T) {
	// Given — a healthy database with a couple of issues.
	dir := initDB(t, "WF")
	author := "doctor-agent"
	createTask(t, dir, "Healthy task A", author)
	createTask(t, dir, "Healthy task B", author)

	// When — run doctor diagnostics.
	_, stderr, code := runNP(t, dir, "admin", "doctor", "--json")

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
	claimID := claimIssue(t, dir, taskID, author)

	// When — soft-delete the task, then garbage-collect.
	_, stderr, code := runNP(t, dir, "issue", "delete", taskID,
		"--claim", claimID,
		"--confirm",
		"--json",
	)
	if code != 0 {
		t.Fatalf("delete failed (exit %d): %s", code, stderr)
	}

	// The issue should no longer appear in list.
	listStdout, _, listCode := runNP(t, dir, "list", "--json")
	if listCode != 0 {
		t.Fatalf("list failed after delete (exit %d)", listCode)
	}
	listResult := parseJSON(t, listStdout)
	listItems, _ := listResult["items"].([]any)
	if len(listItems) != 0 {
		t.Errorf("expected 0 issues after delete, got %d", len(listItems))
	}

	// GC should succeed.
	_, stderr, code = runNP(t, dir, "admin", "gc", "--confirm", "--json")
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

	_, stderr, code := runNP(t, dir, "rel", "add", blockedID,
		"blocked_by", blockerID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("relate add failed (exit %d): %s", code, stderr)
	}

	// When — check readiness before and after closing the blocker.
	blockedBefore := showIssue(t, dir, blockedID)

	blockerClaim := claimIssue(t, dir, blockerID, author)
	_, stderr, code = runNP(t, dir, "issue", "close", blockerID,
		"--claim", blockerClaim,
		"--author", author,
		"--reason", "test close",
		"--json",
	)
	if code != 0 {
		t.Fatalf("close blocker failed (exit %d): %s", code, stderr)
	}

	blockedAfter := showIssue(t, dir, blockedID)

	// Then — the blocked issue becomes ready only after its blocker closes.
	if blockedBefore["is_ready"] == true {
		t.Error("blocked issue should not be ready while blocker is open")
	}
	if blockedAfter["is_ready"] != true {
		t.Error("blocked issue should be ready after blocker is closed")
	}
}

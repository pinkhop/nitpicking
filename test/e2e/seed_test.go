//go:build e2e

package e2e_test

import "testing"

// --- State-seeding helpers ---
//
// These helpers build on the low-level primitives (createTask, claimIssue,
// etc.) to seed the database into specific states in a single call. They
// eliminate multi-step setup boilerplate so that tests can focus on the
// behaviour under test.

// seedClaimedTask creates a task and claims it in one step. Returns the
// issue ID and the claim ID. The test fails if any step does not succeed.
func seedClaimedTask(t *testing.T, dir, title, author string) (issueID, claimID string) {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", title,
		"--author", author,
		"--claim",
		"--json",
	)
	if code != 0 {
		t.Fatalf("seed claimed task %q failed (exit %d): %s", title, code, stderr)
	}
	result := parseJSON(t, stdout)

	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("seed claimed task %q: missing id", title)
	}
	cid, ok := result["claim_id"].(string)
	if !ok || cid == "" {
		t.Fatalf("seed claimed task %q: missing claim_id", title)
	}
	return id, cid
}

// closeIssue claims and closes an issue in one workflow. The test fails if
// any step does not succeed.
func closeIssue(t *testing.T, dir, issueID, author string) {
	t.Helper()

	claimID := claimIssue(t, dir, issueID, author)
	_, stderr, code := runNP(t, dir, "issue", "close", issueID,
		"--claim", claimID,
		"--author", author,
		"--reason", "test close",
		"--json",
	)
	if code != 0 {
		t.Fatalf("close %s failed (exit %d): %s", issueID, code, stderr)
	}
}

// seedClosedTask creates a task and immediately closes it. Returns the
// issue ID. The test fails if any step does not succeed.
func seedClosedTask(t *testing.T, dir, title, author string) string {
	t.Helper()

	taskID := createTask(t, dir, title, author)
	closeIssue(t, dir, taskID, author)
	return taskID
}

// deferIssue claims and defers an issue. The test fails if any step does
// not succeed.
func deferIssue(t *testing.T, dir, issueID, author string) {
	t.Helper()

	claimID := claimIssue(t, dir, issueID, author)
	_, stderr, code := runNP(t, dir, "issue", "defer", issueID,
		"--claim", claimID,
		"--json",
	)
	if code != 0 {
		t.Fatalf("defer %s failed (exit %d): %s", issueID, code, stderr)
	}
}

// seedDeferredTask creates a task and defers it. Returns the issue ID.
func seedDeferredTask(t *testing.T, dir, title, author string) string {
	t.Helper()

	taskID := createTask(t, dir, title, author)
	deferIssue(t, dir, taskID, author)
	return taskID
}

// addRelationship adds a relationship between two issues. The test fails
// if the command does not succeed.
func addRelationship(t *testing.T, dir, sourceID, relType, targetID, author string) {
	t.Helper()

	_, stderr, code := runNP(t, dir, "rel", "add", sourceID,
		relType, targetID,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("relate add %s %s %s failed (exit %d): %s",
			sourceID, relType, targetID, code, stderr)
	}
}

// seedBlockedPair creates two tasks where the second is blocked by the
// first. Returns (blockerID, blockedID).
func seedBlockedPair(t *testing.T, dir, blockerTitle, blockedTitle, author string) (blockerID, blockedID string) {
	t.Helper()

	bkr := createTask(t, dir, blockerTitle, author)
	bkd := createTask(t, dir, blockedTitle, author)
	addRelationship(t, dir, bkd, "blocked_by", bkr, author)
	return bkr, bkd
}

// createEpic creates an epic and returns its ID. The test fails if creation
// does not succeed.
func createEpic(t *testing.T, dir, title, author string) string {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", title,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("create epic %q failed (exit %d): %s", title, code, stderr)
	}
	result := parseJSON(t, stdout)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("create epic %q: missing id", title)
	}
	return id
}

// seedEpicWithTasks creates an epic and a set of child tasks underneath it.
// Returns the epic ID and the child task IDs in the same order as the
// provided titles.
func seedEpicWithTasks(t *testing.T, dir, epicTitle, author string, taskTitles ...string) (epicID string, taskIDs []string) {
	t.Helper()

	epicID = createEpic(t, dir, epicTitle, author)
	taskIDs = make([]string, 0, len(taskTitles))
	for _, title := range taskTitles {
		taskIDs = append(taskIDs, createTaskWithParent(t, dir, title, author, epicID))
	}
	return epicID, taskIDs
}

// seedClaimedTaskWithThreshold creates a task and claims it with a custom
// stale threshold. Returns the issue ID and claim ID. Useful for
// claim-stealing tests where a short threshold is needed.
func seedClaimedTaskWithThreshold(t *testing.T, dir, title, author, threshold string) (issueID, claimID string) {
	t.Helper()

	taskID := createTask(t, dir, title, author)

	stdout, stderr, code := runNP(t, dir, "claim", "id", taskID,
		"--author", author,
		"--stale-threshold", threshold,
		"--json",
	)
	if code != 0 {
		t.Fatalf("claim %s with threshold %s failed (exit %d): %s",
			taskID, threshold, code, stderr)
	}
	result := parseJSON(t, stdout)
	cid, ok := result["claim_id"].(string)
	if !ok || cid == "" {
		t.Fatalf("claim %s: missing claim_id", taskID)
	}
	return taskID, cid
}

// addComment adds a comment to an issue. The test fails if the command does not
// succeed.
func addComment(t *testing.T, dir, issueID, body, author string) {
	t.Helper()

	_, stderr, code := runNP(t, dir, "comment", "add",
		"--issue", issueID,
		"--body", body,
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("comment add on %s failed (exit %d): %s", issueID, code, stderr)
	}
}

// --- Verification tests for seeding helpers ---
//
// These tests confirm each helper produces the expected database state.
// They serve as regression tests for the test infrastructure itself.

func TestE2E_Seed_ClaimedTask(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "SEED")
	author := "seed-agent"

	// When — seed a claimed task.
	issueID, claimID := seedClaimedTask(t, dir, "Claimed task", author)

	// Then — the issue is in claimed state with the correct claim.
	issue := showIssue(t, dir, issueID)
	if issue["state"] != "claimed" {
		t.Errorf("expected state 'claimed', got %v", issue["state"])
	}
	if issue["claim_id"] != claimID {
		t.Errorf("expected claim_id %q, got %v", claimID, issue["claim_id"])
	}
}

func TestE2E_Seed_ClosedTask(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "SEED")

	// When — seed a closed task.
	issueID := seedClosedTask(t, dir, "Closed task", "seed-agent")

	// Then — the issue is in closed state.
	issue := showIssue(t, dir, issueID)
	if issue["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", issue["state"])
	}
}

func TestE2E_Seed_DeferredTask(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "SEED")

	// When — seed a deferred task.
	issueID := seedDeferredTask(t, dir, "Deferred task", "seed-agent")

	// Then — the issue is in deferred state.
	issue := showIssue(t, dir, issueID)
	if issue["state"] != "deferred" {
		t.Errorf("expected state 'deferred', got %v", issue["state"])
	}
}

func TestE2E_Seed_BlockedPair(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "SEED")
	author := "seed-agent"

	// When — seed a blocked pair.
	blockerID, blockedID := seedBlockedPair(t, dir, "Blocker", "Blocked", author)

	// Then — the blocked issue is not ready and has a blocked_by relationship.
	blockedIssue := showIssue(t, dir, blockedID)
	if blockedIssue["is_ready"] == true {
		t.Error("blocked issue should not be ready")
	}

	rels, ok := blockedIssue["relationships"].([]any)
	if !ok || len(rels) == 0 {
		t.Fatal("expected at least one relationship on blocked issue")
	}

	foundBlocker := false
	for _, r := range rels {
		rel, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if rel["type"] == "blocked_by" && rel["target_id"] == blockerID {
			foundBlocker = true
		}
	}
	if !foundBlocker {
		t.Errorf("expected blocked_by relationship to %s", blockerID)
	}
}

func TestE2E_Seed_EpicWithTasks(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "SEED")
	author := "seed-agent"

	// When — seed an epic with child tasks.
	epicID, taskIDs := seedEpicWithTasks(t, dir, "Parent epic", author,
		"Child A", "Child B", "Child C",
	)

	// Then — the epic exists and has the correct number of children.
	epic := showIssue(t, dir, epicID)
	if epic["role"] != "epic" {
		t.Errorf("expected role 'epic', got %v", epic["role"])
	}

	if len(taskIDs) != 3 {
		t.Fatalf("expected 3 task IDs, got %d", len(taskIDs))
	}

	// Each child should reference the epic as its parent.
	for _, childID := range taskIDs {
		child := showIssue(t, dir, childID)
		if child["parent_id"] != epicID {
			t.Errorf("child %s: expected parent_id %s, got %v", childID, epicID, child["parent_id"])
		}
	}
}

//go:build e2e

package e2e_test

import "testing"

func TestE2E_Update_AcceptanceCriteriaFlag_RoundTrips(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	taskID, claimID := seedClaimedTask(t, dir, "AC test", author)

	// When — update with --acceptance-criteria.
	_, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--acceptance-criteria", "All unit tests pass",
		"--json",
	)

	// Then — the acceptance criteria round-trips through show.
	if code != 0 {
		t.Fatalf("update failed (exit %d): %s", code, stderr)
	}
	issue := showIssue(t, dir, taskID)
	if issue["acceptance_criteria"] != "All unit tests pass" {
		t.Errorf("expected acceptance_criteria to round-trip, got %v", issue["acceptance_criteria"])
	}
}

func TestE2E_Update_ParentFlag_ReparentsIssue(t *testing.T) {
	// Given — an epic and a standalone task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	epicID := createEpic(t, dir, "Target epic", author)
	taskID, claimID := seedClaimedTask(t, dir, "Orphan task", author)

	// When — update the task's parent to the epic.
	_, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--parent", epicID,
		"--json",
	)

	// Then — the task is now a child of the epic.
	if code != 0 {
		t.Fatalf("update --parent failed (exit %d): %s", code, stderr)
	}
	issue := showIssue(t, dir, taskID)
	if issue["parent_id"] != epicID {
		t.Errorf("expected parent_id %s, got %v", epicID, issue["parent_id"])
	}
}

func TestE2E_Update_DimensionSetAndRemove(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	taskID, claimID := seedClaimedTask(t, dir, "Dimension test", author)

	// When — set two dimensions.
	_, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--dimension", "kind:fix",
		"--dimension", "area:auth",
		"--json",
	)
	if code != 0 {
		t.Fatalf("dimension-set failed (exit %d): %s", code, stderr)
	}

	// Then — list with dimension filter finds the issue.
	listStdout, _, listCode := runNP(t, dir, "list", "--dimension", "kind:fix", "--json")
	if listCode != 0 {
		t.Fatal("list --dimension kind:fix failed")
	}
	listResult := parseJSON(t, listStdout)
	listItems, _ := listResult["items"].([]any)
	if len(listItems) != 1 {
		t.Errorf("expected 1 issue with dimension kind:fix, got %d", len(listItems))
	}

	listStdout, _, listCode = runNP(t, dir, "list", "--dimension", "area:auth", "--json")
	if listCode != 0 {
		t.Fatal("list --dimension area:auth failed")
	}
	listResult = parseJSON(t, listStdout)
	listItems, _ = listResult["items"].([]any)
	if len(listItems) != 1 {
		t.Errorf("expected 1 issue with dimension area:auth, got %d", len(listItems))
	}

	// When — remove one dimension.
	_, stderr, code = runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--dimension-remove", "area",
		"--json",
	)
	if code != 0 {
		t.Fatalf("dimension-remove failed (exit %d): %s", code, stderr)
	}

	// Then — the removed dimension no longer matches.
	listStdout, _, listCode = runNP(t, dir, "list", "--dimension", "area:auth", "--json")
	if listCode != 0 {
		t.Fatal("list --dimension area:auth after remove failed")
	}
	listResult = parseJSON(t, listStdout)
	listItems, _ = listResult["items"].([]any)
	if len(listItems) != 0 {
		t.Errorf("expected 0 issues with dimension area:auth after removal, got %d", len(listItems))
	}

	// The remaining dimension is still present.
	listStdout, _, listCode = runNP(t, dir, "list", "--dimension", "kind:fix", "--json")
	if listCode != 0 {
		t.Fatal("list --dimension kind:fix after remove failed")
	}
	listResult = parseJSON(t, listStdout)
	listItems, _ = listResult["items"].([]any)
	if len(listItems) != 1 {
		t.Errorf("expected 1 issue still matching kind:fix, got %d", len(listItems))
	}
}

func TestE2E_Update_NoteFlag_AddsNote(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	taskID, claimID := seedClaimedTask(t, dir, "Comment test", author)

	// When — update with --comment to add a comment inline.
	_, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--comment", "Progress update: halfway done",
		"--json",
	)

	// Then — the comment exists on the issue.
	if code != 0 {
		t.Fatalf("update --comment failed (exit %d): %s", code, stderr)
	}
	commentStdout, stderr, code := runNP(t, dir, "comment", "list", "--issue", taskID, "--json")
	if code != 0 {
		t.Fatalf("comment list failed (exit %d): %s", code, stderr)
	}
	commentResult := parseJSON(t, commentStdout)
	commentItems, ok := commentResult["comments"].([]any)
	if !ok || len(commentItems) < 1 {
		t.Errorf("expected at least 1 comment after update --comment, got %d", len(commentItems))
	}
}

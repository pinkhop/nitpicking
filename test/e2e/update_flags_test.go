//go:build e2e

package e2e_test

import "testing"

func TestE2E_Update_AcceptanceCriteriaFlag_RoundTrips(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	taskID, claimID := seedClaimedTask(t, dir, "AC test", author)

	// When — update with --acceptance-criteria.
	_, stderr, code := runNP(t, dir, "update", taskID,
		"--claim-id", claimID,
		"--acceptance-criteria", "All unit tests pass",
		"--json",
	)

	// Then — the acceptance criteria round-trips through show.
	if code != 0 {
		t.Fatalf("update failed (exit %d): %s", code, stderr)
	}
	ticket := showTicket(t, dir, taskID)
	if ticket["acceptance_criteria"] != "All unit tests pass" {
		t.Errorf("expected acceptance_criteria to round-trip, got %v", ticket["acceptance_criteria"])
	}
}

func TestE2E_Update_ParentFlag_ReparentsTicket(t *testing.T) {
	// Given — an epic and a standalone task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	epicID := createEpic(t, dir, "Target epic", author)
	taskID, claimID := seedClaimedTask(t, dir, "Orphan task", author)

	// When — update the task's parent to the epic.
	_, stderr, code := runNP(t, dir, "update", taskID,
		"--claim-id", claimID,
		"--parent", epicID,
		"--json",
	)

	// Then — the task is now a child of the epic.
	if code != 0 {
		t.Fatalf("update --parent failed (exit %d): %s", code, stderr)
	}
	ticket := showTicket(t, dir, taskID)
	if ticket["parent_id"] != epicID {
		t.Errorf("expected parent_id %s, got %v", epicID, ticket["parent_id"])
	}
}

func TestE2E_Update_FacetSetAndRemove(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	taskID, claimID := seedClaimedTask(t, dir, "Facet test", author)

	// When — set two facets.
	_, stderr, code := runNP(t, dir, "update", taskID,
		"--claim-id", claimID,
		"--facet-set", "kind:fix",
		"--facet-set", "area:auth",
		"--json",
	)
	if code != 0 {
		t.Fatalf("facet-set failed (exit %d): %s", code, stderr)
	}

	// Then — list with facet filter finds the ticket.
	listStdout, _, listCode := runNP(t, dir, "list", "--facet", "kind:fix", "--json")
	if listCode != 0 {
		t.Fatal("list --facet kind:fix failed")
	}
	listResult := parseJSON(t, listStdout)
	listCount, _ := listResult["total_count"].(float64)
	if listCount != 1 {
		t.Errorf("expected 1 ticket with facet kind:fix, got %v", listCount)
	}

	listStdout, _, listCode = runNP(t, dir, "list", "--facet", "area:auth", "--json")
	if listCode != 0 {
		t.Fatal("list --facet area:auth failed")
	}
	listResult = parseJSON(t, listStdout)
	listCount, _ = listResult["total_count"].(float64)
	if listCount != 1 {
		t.Errorf("expected 1 ticket with facet area:auth, got %v", listCount)
	}

	// When — remove one facet.
	_, stderr, code = runNP(t, dir, "update", taskID,
		"--claim-id", claimID,
		"--facet-remove", "area",
		"--json",
	)
	if code != 0 {
		t.Fatalf("facet-remove failed (exit %d): %s", code, stderr)
	}

	// Then — the removed facet no longer matches.
	listStdout, _, listCode = runNP(t, dir, "list", "--facet", "area:auth", "--json")
	if listCode != 0 {
		t.Fatal("list --facet area:auth after remove failed")
	}
	listResult = parseJSON(t, listStdout)
	listCount, _ = listResult["total_count"].(float64)
	if listCount != 0 {
		t.Errorf("expected 0 tickets with facet area:auth after removal, got %v", listCount)
	}

	// The remaining facet is still present.
	listStdout, _, listCode = runNP(t, dir, "list", "--facet", "kind:fix", "--json")
	if listCode != 0 {
		t.Fatal("list --facet kind:fix after remove failed")
	}
	listResult = parseJSON(t, listStdout)
	listCount, _ = listResult["total_count"].(float64)
	if listCount != 1 {
		t.Errorf("expected 1 ticket still matching kind:fix, got %v", listCount)
	}
}

func TestE2E_Update_NoteFlag_AddsNote(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "UFLAG")
	author := "flag-agent"
	taskID, claimID := seedClaimedTask(t, dir, "Note test", author)

	// When — update with --note to add a note inline.
	_, stderr, code := runNP(t, dir, "update", taskID,
		"--claim-id", claimID,
		"--note", "Progress update: halfway done",
		"--json",
	)

	// Then — the note exists on the ticket.
	if code != 0 {
		t.Fatalf("update --note failed (exit %d): %s", code, stderr)
	}
	noteStdout, stderr, code := runNP(t, dir, "note", "list", "--ticket", taskID, "--json")
	if code != 0 {
		t.Fatalf("note list failed (exit %d): %s", code, stderr)
	}
	noteResult := parseJSON(t, noteStdout)
	noteCount, ok := noteResult["total_count"].(float64)
	if !ok || noteCount < 1 {
		t.Errorf("expected at least 1 note after update --note, got %v", noteResult["total_count"])
	}
}

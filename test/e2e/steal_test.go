//go:build e2e

package e2e_test

import (
	"testing"
	"time"
)

func TestE2E_ClaimStealing_StealStaleClaimByID(t *testing.T) {
	// Given — agent A claims an issue with a very short stale threshold,
	// and the threshold has elapsed.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	issueID, claimA := seedClaimedTaskWithThreshold(t, dir, "Stealable task", agentA, "1s")
	time.Sleep(2 * time.Second)

	// When — agent B steals the claim.
	stdout, stderr, code := runNP(t, dir, "claim", "id", issueID,
		"--author", agentB,
		"--steal",
		"--json",
	)

	// Then — the steal succeeds and returns a new claim ID.
	if code != 0 {
		t.Fatalf("steal failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	claimB, ok := result["claim_id"].(string)
	if !ok || claimB == "" {
		t.Fatal("expected claim_id in steal response")
	}
	if result["stolen"] != true {
		t.Error("expected stolen=true in response")
	}
	if claimB == claimA {
		t.Error("new claim ID should differ from the original")
	}

	// The issue should now be claimed by agent B.
	issue := showIssue(t, dir, issueID)
	if issue["state"] != "claimed" {
		t.Errorf("expected state 'claimed', got %v", issue["state"])
	}
	if issue["claim_author"] != agentB {
		t.Errorf("expected claim_author %q, got %v", agentB, issue["claim_author"])
	}
}

func TestE2E_ClaimStealing_OriginalClaimInvalidatedAfterSteal(t *testing.T) {
	// Given — agent A's claim has been stolen by agent B.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	issueID, claimA := seedClaimedTaskWithThreshold(t, dir, "Will be stolen", agentA, "1s")
	time.Sleep(2 * time.Second)

	_, stderr, code := runNP(t, dir, "claim", "id", issueID,
		"--author", agentB,
		"--steal",
		"--json",
	)
	if code != 0 {
		t.Fatalf("steal precondition failed (exit %d): %s", code, stderr)
	}

	// When — agent A attempts to use the original claim to update the issue.
	_, _, code = runNP(t, dir, "issue", "update", issueID,
		"--claim", claimA,
		"--title", "Agent A's update",
		"--json",
	)

	// Then — the update fails because the old claim no longer exists.
	// Exit code 2 (not found) — the invalidated claim ID is gone.
	if code == 0 {
		t.Error("expected non-zero exit code when using invalidated claim, but got 0")
	}
}

func TestE2E_ClaimStealing_CannotStealActiveClaim(t *testing.T) {
	// Given — agent A has a fresh (non-stale) claim.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	issueID, _ := seedClaimedTask(t, dir, "Actively claimed", agentA)

	// When — agent B attempts to steal the active claim.
	_, _, code := runNP(t, dir, "claim", "id", issueID,
		"--author", agentB,
		"--steal",
		"--json",
	)

	// Then — the steal fails with a claim conflict (exit code 3).
	if code != 3 {
		t.Errorf("expected exit code 3 (claim conflict for active claim), got %d", code)
	}
}

func TestE2E_ClaimStealing_ClaimReadyStealIfNeeded(t *testing.T) {
	// Given — the only issue in the database is stale-claimed by agent A,
	// so there are no unclaimed ready issues.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	issueID, _ := seedClaimedTaskWithThreshold(t, dir, "Only issue", agentA, "1s")
	time.Sleep(2 * time.Second)

	// When — agent B uses "claim ready" with --steal-if-needed.
	stdout, stderr, code := runNP(t, dir, "claim", "ready",
		"--author", agentB,
		"--steal-if-needed",
		"--json",
	)

	// Then — claim ready steals the stale claim.
	if code != 0 {
		t.Fatalf("claim ready --steal-if-needed failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	if result["issue_id"] != issueID {
		t.Errorf("expected issue_id %s, got %v", issueID, result["issue_id"])
	}
	if result["stolen"] != true {
		t.Error("expected stolen=true in claim ready --steal-if-needed response")
	}
}

func TestE2E_ClaimStealing_StolenClaimAllowsFullLifecycle(t *testing.T) {
	// Given — agent B has stolen a stale claim from agent A.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	issueID, _ := seedClaimedTaskWithThreshold(t, dir, "Lifecycle after steal", agentA, "1s")
	time.Sleep(2 * time.Second)

	stdout, stderr, code := runNP(t, dir, "claim", "id", issueID,
		"--author", agentB,
		"--steal",
		"--json",
	)
	if code != 0 {
		t.Fatalf("steal precondition failed (exit %d): %s", code, stderr)
	}
	claimB := parseJSON(t, stdout)["claim_id"].(string)

	// When — agent B updates the issue and closes it using the stolen claim.
	_, stderr, code = runNP(t, dir, "issue", "update", issueID,
		"--claim", claimB,
		"--title", "Fixed by agent B",
		"--json",
	)
	if code != 0 {
		t.Fatalf("update after steal failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "issue", "close", issueID,
		"--claim", claimB,
		"--author", agentB,
		"--reason", "test close",
		"--json",
	)
	if code != 0 {
		t.Fatalf("close after steal failed (exit %d): %s", code, stderr)
	}

	// Then — the issue is closed with agent B's changes.
	issue := showIssue(t, dir, issueID)
	if issue["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", issue["state"])
	}
	if issue["title"] != "Fixed by agent B" {
		t.Errorf("expected title 'Fixed by agent B', got %v", issue["title"])
	}
}

//go:build e2e

package e2e_test

import (
	"testing"
	"time"
)

func TestE2E_ClaimStealing_StealStaleClaimByID(t *testing.T) {
	// Given — agent A claims a ticket with a very short stale threshold,
	// and the threshold has elapsed.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	ticketID, claimA := seedClaimedTaskWithThreshold(t, dir, "Stealable task", agentA, "1s")
	time.Sleep(2 * time.Second)

	// When — agent B steals the claim.
	stdout, stderr, code := runNP(t, dir, "claim", "id", ticketID,
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

	// The ticket should now be claimed by agent B.
	ticket := showTicket(t, dir, ticketID)
	if ticket["state"] != "claimed" {
		t.Errorf("expected state 'claimed', got %v", ticket["state"])
	}
	if ticket["claim_author"] != agentB {
		t.Errorf("expected claim_author %q, got %v", agentB, ticket["claim_author"])
	}
}

func TestE2E_ClaimStealing_OriginalClaimInvalidatedAfterSteal(t *testing.T) {
	// Given — agent A's claim has been stolen by agent B.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	ticketID, claimA := seedClaimedTaskWithThreshold(t, dir, "Will be stolen", agentA, "1s")
	time.Sleep(2 * time.Second)

	_, stderr, code := runNP(t, dir, "claim", "id", ticketID,
		"--author", agentB,
		"--steal",
		"--json",
	)
	if code != 0 {
		t.Fatalf("steal precondition failed (exit %d): %s", code, stderr)
	}

	// When — agent A attempts to use the original claim to update the ticket.
	_, _, code = runNP(t, dir, "update", ticketID,
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

	ticketID, _ := seedClaimedTask(t, dir, "Actively claimed", agentA)

	// When — agent B attempts to steal the active claim.
	_, _, code := runNP(t, dir, "claim", "id", ticketID,
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
	// Given — the only ticket in the database is stale-claimed by agent A,
	// so there are no unclaimed ready tickets.
	dir := initDB(t, "STEAL")
	agentA := "agent-alpha"
	agentB := "agent-beta"

	ticketID, _ := seedClaimedTaskWithThreshold(t, dir, "Only ticket", agentA, "1s")
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
	if result["ticket_id"] != ticketID {
		t.Errorf("expected ticket_id %s, got %v", ticketID, result["ticket_id"])
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

	ticketID, _ := seedClaimedTaskWithThreshold(t, dir, "Lifecycle after steal", agentA, "1s")
	time.Sleep(2 * time.Second)

	stdout, stderr, code := runNP(t, dir, "claim", "id", ticketID,
		"--author", agentB,
		"--steal",
		"--json",
	)
	if code != 0 {
		t.Fatalf("steal precondition failed (exit %d): %s", code, stderr)
	}
	claimB := parseJSON(t, stdout)["claim_id"].(string)

	// When — agent B updates the ticket and closes it using the stolen claim.
	_, stderr, code = runNP(t, dir, "update", ticketID,
		"--claim", claimB,
		"--title", "Fixed by agent B",
		"--json",
	)
	if code != 0 {
		t.Fatalf("update after steal failed (exit %d): %s", code, stderr)
	}

	_, stderr, code = runNP(t, dir, "state", "close", ticketID,
		"--claim", claimB,
		"--json",
	)
	if code != 0 {
		t.Fatalf("close after steal failed (exit %d): %s", code, stderr)
	}

	// Then — the ticket is closed with agent B's changes.
	ticket := showTicket(t, dir, ticketID)
	if ticket["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", ticket["state"])
	}
	if ticket["title"] != "Fixed by agent B" {
		t.Errorf("expected title 'Fixed by agent B', got %v", ticket["title"])
	}
}

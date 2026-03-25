//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
	"time"
)

func TestE2E_ClaimIdJSON_Normal_Shape(t *testing.T) {
	// Given — an unclaimed task.
	dir := initDB(t, "CI")
	author := "claim-audit"
	taskID := createTask(t, dir, "Task for claim audit", author)

	// When — claim by ID with --json.
	stdout, stderr, code := runNP(t, dir, "claim", "id", taskID,
		"--author", author, "--json")

	// Then
	if code != 0 {
		t.Fatalf("claim id --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	issueID, ok := result["issue_id"].(string)
	if !ok || issueID != taskID {
		t.Errorf("expected issue_id %q, got %v", taskID, result["issue_id"])
	}

	claimID, ok := result["claim_id"].(string)
	if !ok || claimID == "" {
		t.Errorf("claim_id should be a non-empty string, got %v", result["claim_id"])
	}

	stolen, ok := result["stolen"].(bool)
	if !ok || stolen {
		t.Errorf("expected stolen false, got %v", result["stolen"])
	}

	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}

	// No PascalCase leaks.
	for _, banned := range []string{"IssueID", "ClaimID", "Stolen"} {
		if _, found := result[banned]; found {
			t.Errorf("PascalCase key %q leaked", banned)
		}
	}
}

func TestE2E_ClaimIdJSON_Steal_Shape(t *testing.T) {
	// Given — a task with a stale claim (1 second threshold).
	dir := initDB(t, "CI")
	author := "steal-audit"
	taskID := createTask(t, dir, "Task for steal audit", author)
	runNP(t, dir, "claim", "id", taskID,
		"--author", "original-claimer",
		"--stale-threshold", "1s",
		"--json")

	// Wait for claim to go stale.
	time.Sleep(2 * time.Second)

	// When — steal the claim with --json.
	stdout, stderr, code := runNP(t, dir, "claim", "id", taskID,
		"--author", author, "--steal", "--json")

	// Then
	if code != 0 {
		t.Fatalf("claim id --steal --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	stolen, ok := result["stolen"].(bool)
	if !ok || !stolen {
		t.Errorf("expected stolen true, got %v", result["stolen"])
	}
}

//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

func TestE2E_IssueReleaseJSON_Shape(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "RJ")
	author := "release-audit"
	taskID := createTask(t, dir, "Task for release audit", author)
	claimID := claimIssue(t, dir, taskID, author)

	// When — release the claim with --json.
	stdout, stderr, code := runNP(t, dir, "issue", "release", taskID,
		"--claim", claimID,
		"--json",
	)

	// Then — exit 0 and correct JSON shape.
	if code != 0 {
		t.Fatalf("issue release --json failed with exit code %d: %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	// AC1: All keys are snake_case.
	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	// AC2: issue_id is a non-empty string.
	issueID, ok := result["issue_id"].(string)
	if !ok || issueID == "" {
		t.Errorf("issue_id should be a non-empty string, got %v (%T)", result["issue_id"], result["issue_id"])
	}
	if issueID != taskID {
		t.Errorf("expected issue_id %q, got %q", taskID, issueID)
	}

	// action is "release".
	action, ok := result["action"].(string)
	if !ok || action != "release" {
		t.Errorf("expected action 'release', got %v (%T)", result["action"], result["action"])
	}

	// AC5: No is_deleted field.
	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}

	// No PascalCase leaks.
	for _, banned := range []string{"IssueID", "Action", "ClaimID"} {
		if _, found := result[banned]; found {
			t.Errorf("PascalCase key %q leaked into JSON output", banned)
		}
	}

	// Verify the issue is back to open state after release.
	issue := showIssue(t, dir, taskID)
	if issue["state"] != "open" {
		t.Errorf("expected state 'open' after release, got %v", issue["state"])
	}
}

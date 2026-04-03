package issuecmd_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- Release Tests ---

func TestRelease_ClaimedTask_TransitionsToOpen(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Task to release")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Release(t.Context(), issuecmd.ReleaseInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		WriteTo: &buf,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, showErr := svc.ShowIssue(t.Context(), issueID.String())
	if showErr != nil {
		t.Fatalf("show issue failed: %v", showErr)
	}
	if shown.State != domain.StateOpen {
		t.Errorf("state: got %v, want %v", shown.State, domain.StateOpen)
	}
}

func TestRelease_InvalidClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a task with an incorrect claim ID.
	svc := setupService(t)
	issueID := createTask(t, svc, "Release with wrong claim")
	_ = claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Release(t.Context(), issuecmd.ReleaseInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: "bogus-claim-id",
		WriteTo: &buf,
	})

	// Then
	if err == nil {
		t.Fatal("expected error for invalid claim ID")
	}
}

func TestRelease_JSON_ReturnsStructuredOutput(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with JSON output.
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON release task")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Release(t.Context(), issuecmd.ReleaseInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		JSON:    true,
		WriteTo: &buf,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]string
	if jsonErr := json.Unmarshal(buf.Bytes(), &out); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", jsonErr, buf.String())
	}
	if out["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", out["issue_id"], issueID.String())
	}
	if out["action"] != "release" {
		t.Errorf("action: got %q, want %q", out["action"], "release")
	}
}

func TestRelease_TextOutput_ContainsIssueID(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with text output.
	svc := setupService(t)
	issueID := createTask(t, svc, "Text release task")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Release(t.Context(), issuecmd.ReleaseInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		JSON:    false,
		WriteTo: &buf,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte(issueID.String())) {
		t.Errorf("text output should contain issue ID %q, got: %s", issueID, output)
	}
}

func TestRelease_UnclaimedTask_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a task that is not claimed.
	svc := setupService(t)
	issueID := createTask(t, svc, "Unclaimed task")

	var buf bytes.Buffer

	// When: trying to release with a fabricated claim ID.
	err := issuecmd.Release(t.Context(), issuecmd.ReleaseInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: "nonexistent-claim",
		WriteTo: &buf,
	})

	// Then
	if err == nil {
		t.Fatal("expected error when releasing with a nonexistent claim")
	}
}

package issuecmd_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Defer Tests ---

func TestDefer_ClaimedTask_TransitionsToDeferred(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Task to defer")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
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
	if shown.State != domain.StateDeferred {
		t.Errorf("state: got %v, want %v", shown.State, domain.StateDeferred)
	}
}

func TestDefer_InvalidClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a task with an incorrect claim ID.
	svc := setupService(t)
	issueID := createTask(t, svc, "Defer with wrong claim")
	_ = claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
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

func TestDefer_JSON_ReturnsStructuredOutput(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with JSON output requested.
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON defer task")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
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
	if out["action"] != "defer" {
		t.Errorf("action: got %q, want %q", out["action"], "defer")
	}
}

func TestDefer_TextOutput_ContainsIssueID(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with text output.
	svc := setupService(t)
	issueID := createTask(t, svc, "Text defer task")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
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

// claimIssue claims an issue and returns the claim ID.
func claimIssue(t *testing.T, svc driving.Service, issueID domain.ID) string {
	t.Helper()
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	return claimOut.ClaimID
}

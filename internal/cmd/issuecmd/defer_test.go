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

func TestDefer_WithUntil_SetsDeferUntilLabel(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with an --until date.
	svc := setupService(t)
	issueID := createTask(t, svc, "Deferred with date")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		Until:   "2026-04-01",
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

	val, ok := shown.Labels["defer-until"]
	if !ok {
		t.Fatal("expected defer-until label to be present")
	}
	if val != "2026-04-01" {
		t.Errorf("defer-until: got %q, want %q", val, "2026-04-01")
	}
}

func TestDefer_WithUntil_SetsLabelBeforeTransition(t *testing.T) {
	t.Parallel()

	// Given: a claimed task. The label must be set while the claim is active,
	// before the transition invalidates it.
	svc := setupService(t)
	issueID := createTask(t, svc, "Label ordering test")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		Until:   "2026-06-15",
		WriteTo: &buf,
	})
	// Then: if the label were set after the transition, the update would fail
	// because the claim is invalidated. Success here proves correct ordering.
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

	if _, ok := shown.Labels["defer-until"]; !ok {
		t.Error("defer-until label should be present after successful defer with --until")
	}
}

func TestDefer_WithoutUntil_NoLabelSet(t *testing.T) {
	t.Parallel()

	// Given: a claimed task deferred without --until.
	svc := setupService(t)
	issueID := createTask(t, svc, "Defer without date")
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

	if _, ok := shown.Labels["defer-until"]; ok {
		t.Error("defer-until label should not be set when --until is omitted")
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

	// Given: a claimed task with JSON output.
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON defer task")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		Until:   "2026-05-01",
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
	if out["until"] != "2026-05-01" {
		t.Errorf("until: got %q, want %q", out["until"], "2026-05-01")
	}
}

func TestDefer_JSON_WithoutUntil_OmitsUntilField(t *testing.T) {
	t.Parallel()

	// Given: a claimed task deferred without --until, JSON output.
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON defer no until")
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

	var out map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &out); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", jsonErr, buf.String())
	}
	if _, hasUntil := out["until"]; hasUntil {
		t.Errorf("until field should be omitted when --until is not provided, got: %v", out["until"])
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

func TestDefer_TextOutputWithUntil_ContainsDate(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with --until and text output.
	svc := setupService(t)
	issueID := createTask(t, svc, "Text defer with date")
	claimID := claimIssue(t, svc, issueID)

	var buf bytes.Buffer

	// When
	err := issuecmd.Defer(t.Context(), issuecmd.DeferInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		Until:   "2026-07-01",
		WriteTo: &buf,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("2026-07-01")) {
		t.Errorf("text output should contain until date, got: %s", output)
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

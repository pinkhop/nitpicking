package done_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/done"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// claimAuthorName is the author name used when creating and claiming test
// issues. Tests verify that the done workflow derives this author from the
// claim record rather than accepting it as an explicit parameter.
const claimAuthorName = "test-agent"

// --- Helpers ---

func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

// createAndClaim creates a task and claims it, returning the issue ID and claim ID.
func createAndClaim(t *testing.T, svc driving.Service, title string) (domain.ID, string) {
	t.Helper()
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: out.Issue.ID().String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	return out.Issue.ID(), claimOut.ClaimID
}

// --- Tests ---

func TestRun_ClosesIssueAndAddsComment(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Test task")

	var buf bytes.Buffer
	input := done.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,

		Reason:  "Completed implementation and all tests pass.",
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := done.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.State != domain.StateClosed {
		t.Errorf("state: got %v, want %v", shown.State, domain.StateClosed)
	}

	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: issueID.String(),
	})
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}
	if comments.Comments[0].Body != "Completed implementation and all tests pass." {
		t.Errorf("comment body: got %q, want %q",
			comments.Comments[0].Body,
			"Completed implementation and all tests pass.")
	}
}

func TestRun_JSONOutput_ReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "JSON output task")

	var buf bytes.Buffer
	input := done.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,

		Reason:  "Done with this work.",
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := done.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
	if result["action"] != "close" {
		t.Errorf("action: got %q, want %q", result["action"], "close")
	}
}

func TestRun_EmptyReason_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "No reason task")

	var buf bytes.Buffer
	input := done.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,

		Reason:  "",
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := done.Run(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestRun_DerivesAuthorFromClaim(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Derive author task")

	var buf bytes.Buffer
	input := done.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		Reason:  "Closed — author derived from claim.",
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := done.Run(t.Context(), input)
	// Then — succeeds and the comment author matches the claim author.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	comments, listErr := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: issueID.String(),
	})
	if listErr != nil {
		t.Fatalf("list comments failed: %v", listErr)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}
	if comments.Comments[0].Author != mustAuthor(t, claimAuthorName) {
		t.Errorf("comment author: got %q, want %q",
			comments.Comments[0].Author, mustAuthor(t, claimAuthorName))
	}
}

func TestRun_InvalidClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, _ := createAndClaim(t, svc, "Wrong claim task")

	var buf bytes.Buffer
	input := done.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: "wrong-claim-id",

		Reason:  "Some reason.",
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := done.Run(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error for invalid claim ID")
	}
}

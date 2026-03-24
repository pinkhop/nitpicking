package done_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmd/done"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/fake"
)

// --- Helpers ---

func setupService(t *testing.T) service.Service {
	t.Helper()
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("precondition: invalid author: %v", err)
	}
	return a
}

// createAndClaim creates a task and claims it, returning the issue ID and claim ID.
func createAndClaim(t *testing.T, svc service.Service, title string) (issue.ID, string) {
	t.Helper()
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	out, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  title,
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, service.ClaimInput{
		IssueID: out.Issue.ID(),
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
		IssueID: issueID,
		ClaimID: claimID,
		Author:  mustAuthor(t, "test-agent"),
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

	shown, err := svc.ShowIssue(t.Context(), issueID)
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Issue.State() != issue.StateClosed {
		t.Errorf("state: got %q, want %q", shown.Issue.State(), issue.StateClosed)
	}

	comments, err := svc.ListComments(t.Context(), service.ListCommentsInput{
		IssueID: issueID,
	})
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}
	if comments.Comments[0].Body() != "Completed implementation and all tests pass." {
		t.Errorf("comment body: got %q, want %q",
			comments.Comments[0].Body(),
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
		IssueID: issueID,
		ClaimID: claimID,
		Author:  mustAuthor(t, "test-agent"),
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
	if result["action"] != "done" {
		t.Errorf("action: got %q, want %q", result["action"], "done")
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
		IssueID: issueID,
		ClaimID: claimID,
		Author:  mustAuthor(t, "test-agent"),
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

func TestRun_InvalidClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, _ := createAndClaim(t, svc, "Wrong claim task")

	var buf bytes.Buffer
	input := done.RunInput{
		Service: svc,
		IssueID: issueID,
		ClaimID: "wrong-claim-id",
		Author:  mustAuthor(t, "test-agent"),
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

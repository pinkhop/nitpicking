package cmdutil_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
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

// createAndClaim creates a task and claims it, returning the issue ID and
// claim ID.
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

func TestClaimIssueResolver_OnlyClaimProvided_ResolvesFromClaim(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "resolve from claim")
	resolver := cmdutil.NewClaimIssueResolver(svc, cmdutil.NewIDResolver(svc))

	// When
	resolvedID, err := resolver.Resolve(t.Context(), "", claimID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolvedID != issueID {
		t.Errorf("resolved ID: got %s, want %s", resolvedID, issueID)
	}
}

func TestClaimIssueResolver_OnlyIssueProvided_ResolvesDirectly(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, _ := createAndClaim(t, svc, "resolve from issue arg")
	resolver := cmdutil.NewClaimIssueResolver(svc, cmdutil.NewIDResolver(svc))

	// When
	resolvedID, err := resolver.Resolve(t.Context(), issueID.String(), "")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolvedID != issueID {
		t.Errorf("resolved ID: got %s, want %s", resolvedID, issueID)
	}
}

func TestClaimIssueResolver_BothProvidedAndAgree_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "both agree")
	resolver := cmdutil.NewClaimIssueResolver(svc, cmdutil.NewIDResolver(svc))

	// When
	resolvedID, err := resolver.Resolve(t.Context(), issueID.String(), claimID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolvedID != issueID {
		t.Errorf("resolved ID: got %s, want %s", resolvedID, issueID)
	}
}

func TestClaimIssueResolver_BothProvidedAndDisagree_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueA, _ := createAndClaim(t, svc, "issue A")
	_, claimB := createAndClaim(t, svc, "issue B")
	resolver := cmdutil.NewClaimIssueResolver(svc, cmdutil.NewIDResolver(svc))

	// When
	_, err := resolver.Resolve(t.Context(), issueA.String(), claimB)

	// Then
	if err == nil {
		t.Fatal("expected error when issue and claim disagree")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("error message: got %q, want substring %q", err.Error(), "does not match")
	}
}

func TestClaimIssueResolver_NeitherProvided_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	resolver := cmdutil.NewClaimIssueResolver(svc, cmdutil.NewIDResolver(svc))

	// When
	_, err := resolver.Resolve(t.Context(), "", "")

	// Then
	if err == nil {
		t.Fatal("expected error when neither issue nor claim provided")
	}
}

func TestClaimIssueResolver_InvalidClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	resolver := cmdutil.NewClaimIssueResolver(svc, cmdutil.NewIDResolver(svc))

	// When
	_, err := resolver.Resolve(t.Context(), "", "nonexistent-claim-id")

	// Then
	if err == nil {
		t.Fatal("expected error for nonexistent claim ID")
	}
}

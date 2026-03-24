package issuecmd_test

import (
	"bytes"
	"testing"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd"
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

// createTask creates a task and returns its ID.
func createTask(t *testing.T, svc service.Service, title string) issue.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}
	return out.Issue.ID()
}

// claimAndDefer claims an issue and defers it.
func claimAndDefer(t *testing.T, svc service.Service, issueID issue.ID) {
	t.Helper()
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	claimOut, err := svc.ClaimByID(ctx, service.ClaimInput{
		IssueID: issueID,
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: issueID,
		ClaimID: claimOut.ClaimID,
		Action:  service.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer failed: %v", err)
	}
}

// --- Reopen Tests ---

func TestReopen_DeferredIssue_TransitionsToOpen(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Deferred task")
	claimAndDefer(t, svc, issueID)

	var buf bytes.Buffer
	input := issuecmd.ReopenInput{
		Service: svc,
		IssueID: issueID,
		Author:  mustAuthor(t, "test-agent"),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := issuecmd.Reopen(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(t.Context(), issueID)
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Issue.State() != issue.StateOpen {
		t.Errorf("state: got %q, want %q", shown.Issue.State(), issue.StateOpen)
	}
}

func TestReopen_OpenIssue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: an issue that is already open (not deferred).
	svc := setupService(t)
	issueID := createTask(t, svc, "Open task")

	var buf bytes.Buffer
	input := issuecmd.ReopenInput{
		Service: svc,
		IssueID: issueID,
		Author:  mustAuthor(t, "test-agent"),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := issuecmd.Reopen(t.Context(), input)

	// Then: should error because open issues cannot be claimed for reopen.
	if err == nil {
		t.Fatal("expected error for reopening an already-open issue")
	}
}

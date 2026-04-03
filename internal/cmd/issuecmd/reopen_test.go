package issuecmd_test

import (
	"bytes"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

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

// createTask creates a task and returns its ID.
func createTask(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}
	return out.Issue.ID()
}

// claimAndDefer claims an issue and defers it.
func claimAndDefer(t *testing.T, svc driving.Service, issueID domain.ID) {
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

	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: issueID.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer failed: %v", err)
	}
}

// claimAndClose claims an issue and closes it.
func claimAndClose(t *testing.T, svc driving.Service, issueID domain.ID) {
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

	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: issueID.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close failed: %v", err)
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
		IssueID: issueID.String(),
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

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.State != domain.StateOpen {
		t.Errorf("state: got %v, want %v", shown.State, domain.StateOpen)
	}
}

func TestReopen_ClosedIssue_TransitionsToOpen(t *testing.T) {
	t.Parallel()

	// Given: a closed domain.
	svc := setupService(t)
	issueID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, issueID)

	var buf bytes.Buffer
	input := issuecmd.ReopenInput{
		Service: svc,
		IssueID: issueID.String(),
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

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.State != domain.StateOpen {
		t.Errorf("state: got %v, want %v", shown.State, domain.StateOpen)
	}
}

func TestReopen_OpenIssue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: an issue that is already open.
	svc := setupService(t)
	issueID := createTask(t, svc, "Open task")

	var buf bytes.Buffer
	input := issuecmd.ReopenInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := issuecmd.Reopen(t.Context(), input)

	// Then: should error because open issues cannot be reopened.
	if err == nil {
		t.Fatal("expected error for reopening an already-open issue")
	}
}

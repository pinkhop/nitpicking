package core_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func TestEpicProgress_AllChildrenClosed_ReturnsCompleted(t *testing.T) {
	t.Parallel()

	// Given — an epic with two closed children.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicID := createEpic(t, svc, "My epic", author)
	childA := createTask(t, svc, "Child A", epicID, author)
	childB := createTask(t, svc, "Child B", epicID, author)
	claimAndClose(t, svc, childA, author)
	claimAndClose(t, svc, childB, author)

	// When — requesting epic progress.
	out, err := svc.EpicProgress(t.Context(), driving.EpicProgressInput{
		EpicID: epicID.String(),
	})
	// Then — one epic, completed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(out.Items))
	}
	item := out.Items[0]
	if item.ID != epicID.String() {
		t.Errorf("id: got %s, want %s", item.ID, epicID)
	}
	if !item.Completed {
		t.Error("expected completed")
	}
	if item.Total != 2 {
		t.Errorf("total: got %d, want 2", item.Total)
	}
	if item.Closed != 2 {
		t.Errorf("closed: got %d, want 2", item.Closed)
	}
	if item.Percent != 100 {
		t.Errorf("percent: got %d, want 100", item.Percent)
	}
}

func TestEpicProgress_PartialChildren_ReturnsNotCompleted(t *testing.T) {
	t.Parallel()

	// Given — an epic with one open child and one closed child.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicID := createEpic(t, svc, "Partial epic", author)
	childA := createTask(t, svc, "Open child", epicID, author)
	_ = childA
	childB := createTask(t, svc, "Closed child", epicID, author)
	claimAndClose(t, svc, childB, author)

	// When — requesting epic progress.
	out, err := svc.EpicProgress(t.Context(), driving.EpicProgressInput{
		EpicID: epicID.String(),
	})
	// Then — not completed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(out.Items))
	}
	item := out.Items[0]
	if item.Completed {
		t.Error("expected not completed")
	}
	if item.Total != 2 {
		t.Errorf("total: got %d, want 2", item.Total)
	}
	if item.Closed != 1 {
		t.Errorf("closed: got %d, want 1", item.Closed)
	}
}

func TestEpicProgress_NoChildren_ReturnsZeroProgress(t *testing.T) {
	t.Parallel()

	// Given — an epic with no children.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicID := createEpic(t, svc, "Empty epic", author)

	// When — requesting epic progress.
	out, err := svc.EpicProgress(t.Context(), driving.EpicProgressInput{
		EpicID: epicID.String(),
	})
	// Then — zero progress, not completed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(out.Items))
	}
	if out.Items[0].Completed {
		t.Error("expected not completed")
	}
	if out.Items[0].Total != 0 {
		t.Errorf("total: got %d, want 0", out.Items[0].Total)
	}
}

func TestEpicProgress_AllOpenEpics_ReturnsMultiple(t *testing.T) {
	t.Parallel()

	// Given — two epics with different progress.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicA := createEpic(t, svc, "Epic A", author)
	epicB := createEpic(t, svc, "Epic B", author)
	childA := createTask(t, svc, "Child of A", epicA, author)
	claimAndClose(t, svc, childA, author)
	_ = createTask(t, svc, "Child of B", epicB, author)

	// When — requesting progress for all epics (no specific ID).
	out, err := svc.EpicProgress(t.Context(), driving.EpicProgressInput{})
	// Then — two items returned.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items: got %d, want 2", len(out.Items))
	}
}

func TestEpicProgress_SingleEpicNotFound_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a service with no matching epic.
	svc, _ := setupService(t)

	bogusID, err := domain.ParseID("NP-zzzzz")
	if err != nil {
		t.Fatalf("parse id: %v", err)
	}

	// When — requesting progress for a non-existent epic.
	_, epicErr := svc.EpicProgress(t.Context(), driving.EpicProgressInput{
		EpicID: bogusID.String(),
	})

	// Then — error returned.
	if epicErr == nil {
		t.Fatal("expected error for non-existent epic")
	}
}

func TestCloseCompletedEpics_ClosesCompletedEpics(t *testing.T) {
	t.Parallel()

	// Given — one completed epic and one incomplete epic.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	completedEpicID := createEpic(t, svc, "Completed epic", author)
	incompleteEpicID := createEpic(t, svc, "Incomplete epic", author)
	closedChild := createTask(t, svc, "Closed child", completedEpicID, author)
	claimAndClose(t, svc, closedChild, author)
	_ = createTask(t, svc, "Open child", incompleteEpicID, author)

	// When — closing completed epics.
	out, err := svc.CloseCompletedEpics(t.Context(), driving.CloseCompletedEpicsInput{
		Author: author,
	})
	// Then — one epic closed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ClosedCount != 1 {
		t.Errorf("closed count: got %d, want 1", out.ClosedCount)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results: got %d, want 1", len(out.Results))
	}
	if out.Results[0].ID != completedEpicID.String() {
		t.Errorf("id: got %s, want %s", out.Results[0].ID, completedEpicID)
	}
	if !out.Results[0].Closed {
		t.Error("expected closed=true")
	}

	// Verify the epic is actually closed.
	shown, showErr := svc.ShowIssue(t.Context(), completedEpicID.String())
	if showErr != nil {
		t.Fatalf("show: %v", showErr)
	}
	if shown.State != domain.StateClosed {
		t.Errorf("state: got %v, want closed", shown.State)
	}
}

func TestCloseCompletedEpics_DryRun_DoesNotClose(t *testing.T) {
	t.Parallel()

	// Given — one completed epic.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicID := createEpic(t, svc, "Completed epic", author)
	child := createTask(t, svc, "Child", epicID, author)
	claimAndClose(t, svc, child, author)

	// When — dry-running close-completed.
	out, err := svc.CloseCompletedEpics(t.Context(), driving.CloseCompletedEpicsInput{
		Author: author,
		DryRun: true,
	})
	// Then — one result, but not actually closed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results: got %d, want 1", len(out.Results))
	}
	if out.Results[0].Closed {
		t.Error("expected closed=false for dry run")
	}
	if out.ClosedCount != 0 {
		t.Errorf("closed count: got %d, want 0", out.ClosedCount)
	}

	// Verify the epic is still open.
	shown, showErr := svc.ShowIssue(t.Context(), epicID.String())
	if showErr != nil {
		t.Fatalf("show: %v", showErr)
	}
	if shown.State != domain.StateOpen {
		t.Errorf("state: got %v, want open", shown.State)
	}
}

func TestCloseCompletedEpics_NoCompletedEpics_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Given — only an incomplete epic.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicID := createEpic(t, svc, "Incomplete epic", author)
	_ = createTask(t, svc, "Open child", epicID, author)

	// When — closing completed epics.
	out, err := svc.CloseCompletedEpics(t.Context(), driving.CloseCompletedEpicsInput{
		Author: author,
	})
	// Then — empty result.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 0 {
		t.Errorf("results: got %d, want 0", len(out.Results))
	}
	if out.ClosedCount != 0 {
		t.Errorf("closed count: got %d, want 0", out.ClosedCount)
	}
}

// createEpic creates an open epic and returns its ID.
func createEpic(t *testing.T, svc driving.Service, title string, author string) domain.ID {
	t.Helper()

	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: author,
	})
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}
	return out.Issue.ID()
}

// createTask creates a task under the given parent and returns its ID.
func createTask(t *testing.T, svc driving.Service, title string, parentID domain.ID, author string) domain.ID {
	t.Helper()

	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		ParentID: parentID.String(),
		Author:   author,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return out.Issue.ID()
}

// claimAndClose claims and closes an domain.
func claimAndClose(t *testing.T, svc driving.Service, id domain.ID, author string) {
	t.Helper()

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: id.String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: id.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
	}); err != nil {
		t.Fatalf("close: %v", err)
	}
}

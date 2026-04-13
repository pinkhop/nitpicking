//go:build boundary

package sqlite_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func setupBoundaryService(t *testing.T) driving.Service {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := core.New(store, store)
	ctx := t.Context()
	if err := svc.Init(ctx, "TEST"); err != nil {
		t.Fatalf("initializing database: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

func TestBoundary_FullIssueLifecycle(t *testing.T) {
	// Given
	svc := setupBoundaryService(t)
	ctx := t.Context()
	author := mustAuthor(t, "boundary-test")

	// When — create a task
	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Boundary test task",
		Author: author,
		Claim:  true,
	})
	// Then
	if err != nil {
		t.Fatalf("creating issue: %v", err)
	}
	if createOut.ClaimID == "" {
		t.Error("expected claim ID")
	}

	issueID := createOut.Issue.ID()

	// When — update the title
	newTitle := "Updated boundary task"
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: issueID.String(),
		ClaimID: createOut.ClaimID,
		Title:   &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("updating issue: %v", err)
	}

	// When — show the issue
	showOut, err := svc.ShowIssue(ctx, issueID.String())
	// Then
	if err != nil {
		t.Fatalf("showing issue: %v", err)
	}
	if showOut.Title != "Updated boundary task" {
		t.Errorf("expected updated title, got %q", showOut.Title)
	}

	// When — close the issue
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: issueID.String(),
		ClaimID: createOut.ClaimID,
		Action:  driving.ActionClose,
	})
	// Then
	if err != nil {
		t.Fatalf("closing issue: %v", err)
	}

	// Verify closed.
	showOut, _ = svc.ShowIssue(ctx, issueID.String())
	if showOut.State != domain.StateClosed {
		t.Errorf("expected closed, got %v", showOut.State)
	}
}

func TestBoundary_CommentOnClosedIssue(t *testing.T) {
	// Given
	svc := setupBoundaryService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task", Author: author, Claim: true,
	})
	_ = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: createOut.Issue.ID().String(), ClaimID: createOut.ClaimID, Action: driving.ActionClose,
	})

	// When — add comment to closed issue
	commentOut, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: createOut.Issue.ID().String(), Author: author, Body: "Post-close comment",
	})
	// Then — should succeed per spec
	if err != nil {
		t.Fatalf("expected success adding comment to closed issue: %v", err)
	}
	if commentOut.Comment.Body != "Post-close comment" {
		t.Errorf("expected comment body, got %q", commentOut.Comment.Body)
	}
}

func TestBoundary_ListAndPagination(t *testing.T) {
	// Given
	svc := setupBoundaryService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	for i := range 5 {
		_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
			Role: domain.RoleTask, Title: "Task " + string(rune('A'+i)), Author: author,
		})
		if err != nil {
			t.Fatalf("creating issue %d: %v", i, err)
		}
	}

	// When — list with limit 3
	out, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Limit: 3,
	})
	// Then
	if err != nil {
		t.Fatalf("listing issues: %v", err)
	}
	if !out.HasMore {
		t.Errorf("expected HasMore to be true with 5 issues and limit 3")
	}
	if len(out.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(out.Items))
	}
}

func TestBoundary_DeleteAndNotFound(t *testing.T) {
	// Given
	svc := setupBoundaryService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "To delete", Author: author, Claim: true,
	})

	// When — delete
	err := svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: createOut.Issue.ID().String(), ClaimID: createOut.ClaimID,
	})
	// Then
	if err != nil {
		t.Fatalf("deleting issue: %v", err)
	}

	// Verify not found.
	_, err = svc.ShowIssue(ctx, createOut.Issue.ID().String())
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestBoundary_ExtendStaleThreshold(t *testing.T) {
	// Given
	svc := setupBoundaryService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task", Author: author, Claim: true,
	})

	// When
	err := svc.ExtendStaleThreshold(ctx, createOut.Issue.ID().String(), createOut.ClaimID, 8*time.Hour)
	// Then
	if err != nil {
		t.Fatalf("extending threshold: %v", err)
	}
}

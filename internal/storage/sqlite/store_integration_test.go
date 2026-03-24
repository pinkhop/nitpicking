//go:build integration

package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/storage/sqlite"
)

func setupIntegrationService(t *testing.T) service.Service {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := service.New(store)
	ctx := context.Background()
	if err := svc.Init(ctx, "TEST"); err != nil {
		t.Fatalf("initializing database: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("creating author: %v", err)
	}
	return a
}

func TestIntegration_FullIssueLifecycle(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "integration-test")

	// When — create a task
	createOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Integration test task",
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
	newTitle := "Updated integration task"
	err = svc.UpdateIssue(ctx, service.UpdateIssueInput{
		IssueID: issueID,
		ClaimID: createOut.ClaimID,
		Title:   &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("updating issue: %v", err)
	}

	// When — show the issue
	showOut, err := svc.ShowIssue(ctx, issueID)
	// Then
	if err != nil {
		t.Fatalf("showing issue: %v", err)
	}
	if showOut.Issue.Title() != "Updated integration task" {
		t.Errorf("expected updated title, got %q", showOut.Issue.Title())
	}

	// When — close the issue
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: issueID,
		ClaimID: createOut.ClaimID,
		Action:  service.ActionClose,
	})
	// Then
	if err != nil {
		t.Fatalf("closing issue: %v", err)
	}

	// Verify closed.
	showOut, _ = svc.ShowIssue(ctx, issueID)
	if showOut.Issue.State() != issue.StateClosed {
		t.Errorf("expected closed, got %s", showOut.Issue.State())
	}
}

func TestIntegration_NoteOnClosedIssue(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task", Author: author, Claim: true,
	})
	_ = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: createOut.Issue.ID(), ClaimID: createOut.ClaimID, Action: service.ActionClose,
	})

	// When — add comment to closed issue
	commentOut, err := svc.AddComment(ctx, service.AddCommentInput{
		IssueID: createOut.Issue.ID(), Author: author, Body: "Post-close comment",
	})
	// Then — should succeed per spec
	if err != nil {
		t.Fatalf("expected success adding comment to closed issue: %v", err)
	}
	if commentOut.Comment.Body() != "Post-close comment" {
		t.Errorf("expected comment body, got %q", commentOut.Comment.Body())
	}
}

func TestIntegration_ListAndPagination(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	for i := range 5 {
		_, err := svc.CreateIssue(ctx, service.CreateIssueInput{
			Role: issue.RoleTask, Title: "Task " + string(rune('A'+i)), Author: author,
		})
		if err != nil {
			t.Fatalf("creating issue %d: %v", i, err)
		}
	}

	// When — list with limit 3
	out, err := svc.ListIssues(ctx, service.ListIssuesInput{
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

func TestIntegration_DeleteAndNotFound(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "To delete", Author: author, Claim: true,
	})

	// When — delete
	err := svc.DeleteIssue(ctx, service.DeleteInput{
		IssueID: createOut.Issue.ID(), ClaimID: createOut.ClaimID,
	})
	// Then
	if err != nil {
		t.Fatalf("deleting issue: %v", err)
	}

	// Verify not found.
	_, err = svc.ShowIssue(ctx, createOut.Issue.ID())
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_ExtendStaleThreshold(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task", Author: author, Claim: true,
	})

	// When
	err := svc.ExtendStaleThreshold(ctx, createOut.Issue.ID(), createOut.ClaimID, 8*time.Hour)
	// Then
	if err != nil {
		t.Fatalf("extending threshold: %v", err)
	}
}

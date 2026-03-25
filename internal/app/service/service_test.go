package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/fake"
)

func setupService(t *testing.T) (service.Service, *fake.Repository) {
	t.Helper()
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	ctx := context.Background()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	return svc, repo
}

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

// --- Init ---

func TestInit_ValidPrefix_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	// When
	err := svc.Init(context.Background(), "NP")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInit_InvalidPrefix_Fails(t *testing.T) {
	t.Parallel()

	// Given
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	// When
	err := svc.Init(context.Background(), "np")

	// Then
	if err == nil {
		t.Fatal("expected error for lowercase prefix")
	}
}

// --- AgentName ---

func TestAgentName_ReturnsNonEmpty(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	name, err := svc.AgentName(context.Background())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name == "" {
		t.Error("expected non-empty name")
	}
}

// --- CreateIssue ---

func TestCreateIssue_Task_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	// When
	output, err := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Fix login bug",
		Author: author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Issue.ID().IsZero() {
		t.Error("expected non-zero issue ID")
	}
	if output.Issue.Title() != "Fix login bug" {
		t.Errorf("expected title, got %q", output.Issue.Title())
	}
	if output.Issue.State() != issue.StateOpen {
		t.Errorf("expected open state, got %s", output.Issue.State())
	}
}

func TestCreateIssue_WithClaim_ReturnsClaimID(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	output, err := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: mustAuthor(t, "alice"),
		Claim:  true,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ClaimID == "" {
		t.Error("expected non-empty claim ID when created with claim")
	}
}

func TestCreateIssue_IdempotencyKey_ReturnsSameIssue(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	input := service.CreateIssueInput{
		Role:           issue.RoleTask,
		Title:          "Idempotent task",
		Author:         author,
		IdempotencyKey: "idem-1",
	}

	// When — create twice with same key
	out1, err1 := svc.CreateIssue(context.Background(), input)
	out2, err2 := svc.CreateIssue(context.Background(), input)

	// Then
	if err1 != nil {
		t.Fatalf("first create failed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second create failed: %v", err2)
	}
	if out1.Issue.ID() != out2.Issue.ID() {
		t.Errorf("expected same issue ID, got %s and %s", out1.Issue.ID(), out2.Issue.ID())
	}
}

func TestCreateIssue_InvalidTitle_Fails(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	_, err := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "---",
		Author: mustAuthor(t, "alice"),
	})

	// Then
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// --- ClaimByID ---

func TestClaimByID_UnclaimedIssue_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.ClaimByID(context.Background(), service.ClaimInput{
		IssueID: created.Issue.ID(),
		Author:  author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ClaimID == "" {
		t.Error("expected non-empty claim ID")
	}
}

func TestClaimByID_AlreadyClaimed_Fails(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	bob := mustAuthor(t, "bob")
	_, err := svc.ClaimByID(context.Background(), service.ClaimInput{
		IssueID: created.Issue.ID(),
		Author:  bob,
	})

	// Then
	if !errors.Is(err, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError, got %v", err)
	}
}

// --- TransitionState ---

func TestTransitionState_Close_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.TransitionState(context.Background(), service.TransitionInput{
		IssueID: created.Issue.ID(),
		ClaimID: created.ClaimID,
		Action:  service.ActionClose,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify issue is closed.
	show, _ := svc.ShowIssue(context.Background(), created.Issue.ID())
	if show.Issue.State() != issue.StateClosed {
		t.Errorf("expected closed, got %s", show.Issue.State())
	}
}

func TestTransitionState_Release_ReturnsToDefault(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.TransitionState(context.Background(), service.TransitionInput{
		IssueID: created.Issue.ID(),
		ClaimID: created.ClaimID,
		Action:  service.ActionRelease,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowIssue(context.Background(), created.Issue.ID())
	if show.Issue.State() != issue.StateOpen {
		t.Errorf("expected open after release, got %s", show.Issue.State())
	}
}

// --- UpdateIssue ---

func TestUpdateIssue_ChangesTitle(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Original",
		Author: author,
		Claim:  true,
	})

	// When
	newTitle := "Updated title"
	err := svc.UpdateIssue(context.Background(), service.UpdateIssueInput{
		IssueID: created.Issue.ID(),
		ClaimID: created.ClaimID,
		Title:   &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowIssue(context.Background(), created.Issue.ID())
	if show.Issue.Title() != "Updated title" {
		t.Errorf("expected Updated title, got %q", show.Issue.Title())
	}
}

// --- OneShotUpdate ---

func TestOneShotUpdate_ChangesAndReleases(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Original",
		Author: author,
	})

	// When
	newTitle := "Quick fix"
	err := svc.OneShotUpdate(context.Background(), service.OneShotUpdateInput{
		IssueID: created.Issue.ID(),
		Author:  author,
		Title:   &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowIssue(context.Background(), created.Issue.ID())
	if show.Issue.Title() != "Quick fix" {
		t.Errorf("expected Quick fix, got %q", show.Issue.Title())
	}
	if show.Issue.State() != issue.StateOpen {
		t.Errorf("expected open after one-shot, got %s", show.Issue.State())
	}
}

// --- AddComment ---

func TestAddComment_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.AddComment(context.Background(), service.AddCommentInput{
		IssueID: created.Issue.ID(),
		Author:  author,
		Body:    "This is a comment.",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Comment.Body() != "This is a comment." {
		t.Errorf("expected comment body, got %q", output.Comment.Body())
	}
}

func TestAddComment_DeletedIssue_Fails(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// Delete the issue.
	_ = svc.DeleteIssue(context.Background(), service.DeleteInput{
		IssueID: created.Issue.ID(),
		ClaimID: created.ClaimID,
	})

	// When
	_, err := svc.AddComment(context.Background(), service.AddCommentInput{
		IssueID: created.Issue.ID(),
		Author:  author,
		Body:    "Comment on deleted issue",
	})

	// Then
	if !errors.Is(err, domain.ErrDeletedIssue) {
		t.Errorf("expected ErrDeletedIssue, got %v", err)
	}
}

func TestAddComment_ClosedIssue_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	_ = svc.TransitionState(context.Background(), service.TransitionInput{
		IssueID: created.Issue.ID(),
		ClaimID: created.ClaimID,
		Action:  service.ActionClose,
	})

	// When — comments CAN be added to closed issues
	_, err := svc.AddComment(context.Background(), service.AddCommentInput{
		IssueID: created.Issue.ID(),
		Author:  author,
		Body:    "Post-mortem comment",
	})
	// Then
	if err != nil {
		t.Fatalf("expected success adding comment to closed issue, got: %v", err)
	}
}

// --- ShowIssue ---

func TestShowIssue_ReturnsRevisionAndAuthor(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	show, err := svc.ShowIssue(context.Background(), created.Issue.ID())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.Revision != 0 {
		t.Errorf("expected revision 0, got %d", show.Revision)
	}
	if !show.Author.Equal(author) {
		t.Errorf("expected author alice, got %s", show.Author)
	}
}

func TestShowIssue_IncludesCommentCount(t *testing.T) {
	t.Parallel()

	// Given: an issue with two comments.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task with comments", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.AddComment(t.Context(), service.AddCommentInput{
		IssueID: created.Issue.ID(), Author: author, Body: "Comment one",
	})
	if err != nil {
		t.Fatalf("precondition: add comment 1: %v", err)
	}
	_, err = svc.AddComment(t.Context(), service.AddCommentInput{
		IssueID: created.Issue.ID(), Author: author, Body: "Comment two",
	})
	if err != nil {
		t.Fatalf("precondition: add comment 2: %v", err)
	}

	// When
	show, err := svc.ShowIssue(t.Context(), created.Issue.ID())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.CommentCount != 2 {
		t.Errorf("expected CommentCount 2, got %d", show.CommentCount)
	}
}

// --- ListIssues ---

func TestListIssues_FilterByReady(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	// Create two tasks — one open (ready), one claimed (not ready).
	_, _ = svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Ready task",
		Author: author,
	})
	_, _ = svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Claimed task",
		Author: author,
		Claim:  true,
	})

	// When
	output, err := svc.ListIssues(context.Background(), service.ListIssuesInput{
		Filter: port.IssueFilter{Ready: true},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Errorf("expected 1 ready issue, got %d", len(output.Items))
	}
}

func TestBlockedByClosedBlocker_TaskBecomesReady(t *testing.T) {
	t.Parallel()

	// Given — a task blocked_by another task. Close the blocker.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "agent-a")

	blockerOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Blocker task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Blocked task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked task: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID(),
		service.RelationshipInput{Type: issue.RelBlockedBy, TargetID: blockerOut.Issue.ID()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// Close the blocker.
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: blockerOut.Issue.ID(), ClaimID: blockerOut.ClaimID,
		Action: service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close blocker: %v", err)
	}

	// When — list ready issues.
	listOut, err := svc.ListIssues(ctx, service.ListIssuesInput{
		Filter: port.IssueFilter{Ready: true},
	})
	// Then — the blocked task should appear as ready.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, item := range listOut.Items {
		if item.ID == blockedOut.Issue.ID() {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected blocked task %s to be ready after blocker closed", blockedOut.Issue.ID())
	}
}

func TestShowIssue_BlockedByClosedBlocker_IsReady(t *testing.T) {
	t.Parallel()

	// Given — a task blocked by a closed task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "agent-b")

	blockerOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Blocker", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID(),
		service.RelationshipInput{Type: issue.RelBlockedBy, TargetID: blockerOut.Issue.ID()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: blockerOut.Issue.ID(), ClaimID: blockerOut.ClaimID,
		Action: service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close blocker: %v", err)
	}

	// When — show the blocked task.
	showOut, err := svc.ShowIssue(ctx, blockedOut.Issue.ID())
	// Then — the blocked task should be ready.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !showOut.IsReady {
		t.Error("expected blocked task to be ready when blocker is closed")
	}
}

func TestListIssues_ExcludeClosed_HidesClosedIssues(t *testing.T) {
	t.Parallel()

	// Given: one open task and one closed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	_, err := svc.CreateIssue(t.Context(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Open task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closed, err := svc.CreateIssue(t.Context(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Closed task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed task: %v", err)
	}
	err = svc.TransitionState(t.Context(), service.TransitionInput{
		IssueID: closed.Issue.ID(),
		ClaimID: closed.ClaimID,
		Action:  service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When: listing with ExcludeClosed.
	output, err := svc.ListIssues(t.Context(), service.ListIssuesInput{
		Filter: port.IssueFilter{ExcludeClosed: true},
	})
	// Then: only the open task appears.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Errorf("expected 1 issue, got %d", len(output.Items))
	}
	if len(output.Items) == 1 && output.Items[0].Title != "Open task" {
		t.Errorf("expected Open task, got %q", output.Items[0].Title)
	}
}

func TestListIssues_ExcludeClosed_WithExplicitClosedState_ShowsClosed(t *testing.T) {
	t.Parallel()

	// Given: one open task and one closed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	_, err := svc.CreateIssue(t.Context(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Open task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closed, err := svc.CreateIssue(t.Context(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Closed task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed task: %v", err)
	}
	err = svc.TransitionState(t.Context(), service.TransitionInput{
		IssueID: closed.Issue.ID(),
		ClaimID: closed.ClaimID,
		Action:  service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When: ExcludeClosed is set but States explicitly requests closed — States
	// takes precedence because it represents an explicit user intent.
	output, err := svc.ListIssues(t.Context(), service.ListIssuesInput{
		Filter: port.IssueFilter{
			ExcludeClosed: true,
			States:        []issue.State{issue.StateClosed},
		},
	})
	// Then: only the closed task appears; ExcludeClosed is overridden.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Errorf("expected 1 issue, got %d", len(output.Items))
	}
	if len(output.Items) == 1 && output.Items[0].Title != "Closed task" {
		t.Errorf("expected Closed task, got %q", output.Items[0].Title)
	}
}

// --- GetGraphData ---

func TestGetGraphData_ReturnsNodesAndRelationships(t *testing.T) {
	t.Parallel()

	// Given: two tasks with a blocked_by relationship.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	a, err := svc.CreateIssue(t.Context(), service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}
	b, err := svc.CreateIssue(t.Context(), service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create B: %v", err)
	}
	err = svc.AddRelationship(t.Context(), a.Issue.ID(),
		service.RelationshipInput{Type: issue.RelBlockedBy, TargetID: b.Issue.ID()}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship: %v", err)
	}

	// When
	result, err := svc.GetGraphData(t.Context())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result.Nodes))
	}
	if len(result.Relationships) == 0 {
		t.Error("expected at least 1 relationship")
	}
}

// --- DeleteIssue ---

func TestDeleteIssue_TaskSucceeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.DeleteIssue(context.Background(), service.DeleteInput{
		IssueID: created.Issue.ID(),
		ClaimID: created.ClaimID,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Show should fail.
	_, err = svc.ShowIssue(context.Background(), created.Issue.ID())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for deleted issue, got %v", err)
	}
}

// --- ExtendStaleThreshold ---

func TestExtendStaleThreshold_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.ExtendStaleThreshold(context.Background(), created.Issue.ID(), created.ClaimID, 12*time.Hour)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ShowHistory ---

func TestShowHistory_ReturnsEntries(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(context.Background(), service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.ShowHistory(context.Background(), service.ListHistoryInput{
		IssueID: created.Issue.ID(),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Entries) < 1 {
		t.Error("expected at least 1 history entry (creation)")
	}
}

func TestCloseIssue_WithUnclosedChildren_Fails(t *testing.T) {
	t.Parallel()

	// Given — a parent with an open child.
	svc, _ := setupService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	parentOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Parent", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create parent: %v", err)
	}

	_, err = svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Child", Author: author, ParentID: parentOut.Issue.ID(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// When — try to close the parent.
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: parentOut.Issue.ID(),
		ClaimID: parentOut.ClaimID,
		Action:  service.ActionClose,
	})

	// Then — should fail because child is not closed.
	if err == nil {
		t.Fatal("expected error when closing issue with unclosed children")
	}
	if !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition, got %v", err)
	}
}

func TestCloseIssue_WithAllChildrenClosed_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a parent with a closed child.
	svc, _ := setupService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	parentOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Parent", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create parent: %v", err)
	}

	childOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Child", Author: author, Claim: true, ParentID: parentOut.Issue.ID(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// Close the child first.
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: childOut.Issue.ID(),
		ClaimID: childOut.ClaimID,
		Action:  service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child: %v", err)
	}

	// Claim the parent.
	claimOut, err := svc.ClaimByID(ctx, service.ClaimInput{
		IssueID: parentOut.Issue.ID(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim parent: %v", err)
	}

	// When — close the parent.
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: parentOut.Issue.ID(),
		ClaimID: claimOut.ClaimID,
		Action:  service.ActionClose,
	})
	// Then — should succeed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- IsBlocked in list output ---

func TestListIssues_BlockedIssue_HasIsBlockedTrue(t *testing.T) {
	t.Parallel()

	// Given — task A is blocked by open task B.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "blocked-test")

	blockerOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	blockedOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.AddRelationship(ctx, blockedOut.Issue.ID(), service.RelationshipInput{
		Type: issue.RelBlockedBy, TargetID: blockerOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — list all issues.
	result, err := svc.ListIssues(ctx, service.ListIssuesInput{
		Filter: port.IssueFilter{ExcludeClosed: true},
		Limit:  -1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then — the blocked issue should have IsBlocked true.
	for _, item := range result.Items {
		if item.ID == blockedOut.Issue.ID() {
			if !item.IsBlocked {
				t.Error("expected IsBlocked=true for blocked issue")
			}
		} else if item.ID == blockerOut.Issue.ID() {
			if item.IsBlocked {
				t.Error("expected IsBlocked=false for non-blocked issue")
			}
		}
	}
}

func TestListIssues_ResolvedBlocker_HasIsBlockedFalse(t *testing.T) {
	t.Parallel()

	// Given — task A was blocked by task B, but B is now closed.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "blocked-test")

	blockerOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Blocker", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	blockedOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Was blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.AddRelationship(ctx, blockedOut.Issue.ID(), service.RelationshipInput{
		Type: issue.RelBlockedBy, TargetID: blockerOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: blockerOut.Issue.ID(), ClaimID: blockerOut.ClaimID, Action: service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close blocker: %v", err)
	}

	// When — list non-closed issues.
	result, err := svc.ListIssues(ctx, service.ListIssuesInput{
		Filter: port.IssueFilter{ExcludeClosed: true},
		Limit:  -1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then — the previously-blocked issue should have IsBlocked false.
	for _, item := range result.Items {
		if item.ID == blockedOut.Issue.ID() && item.IsBlocked {
			t.Error("expected IsBlocked=false when blocker is closed")
		}
	}
}

// --- Symmetric refs relationships ---

func TestAddRelationship_Refs_AppearsInBothDirections(t *testing.T) {
	t.Parallel()

	// Given — two tasks exist.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "refs-test")

	aOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create B: %v", err)
	}

	// When — add A refs B.
	err = svc.AddRelationship(ctx, aOut.Issue.ID(), service.RelationshipInput{
		Type: issue.RelRefs, TargetID: bOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add refs: %v", err)
	}

	// Then — ShowIssue for B should include the refs relationship.
	showB, err := svc.ShowIssue(ctx, bOut.Issue.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundRefs := false
	for _, rel := range showB.Relationships {
		if rel.Type() == issue.RelRefs {
			foundRefs = true
			// From B's perspective, B is the source and A is the target.
			if rel.SourceID() != bOut.Issue.ID() || rel.TargetID() != aOut.Issue.ID() {
				t.Errorf("expected B→refs→A, got %s→refs→%s", rel.SourceID(), rel.TargetID())
			}
		}
	}
	if !foundRefs {
		t.Error("expected refs relationship visible from B's side")
	}
}

func TestAddRelationship_Refs_Idempotent_BothDirections(t *testing.T) {
	t.Parallel()

	// Given — A refs B already exists.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "refs-test")

	aOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	err = svc.AddRelationship(ctx, aOut.Issue.ID(), service.RelationshipInput{
		Type: issue.RelRefs, TargetID: bOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add refs: %v", err)
	}

	// When — add B refs A (reverse direction of same symmetric link).
	err = svc.AddRelationship(ctx, bOut.Issue.ID(), service.RelationshipInput{
		Type: issue.RelRefs, TargetID: aOut.Issue.ID(),
	}, author)
	// Then — should succeed (idempotent), and only one relationship should exist.
	if err != nil {
		t.Fatalf("unexpected error adding reverse refs: %v", err)
	}
	showA, err := svc.ShowIssue(ctx, aOut.Issue.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	refsCount := 0
	for _, rel := range showA.Relationships {
		if rel.Type() == issue.RelRefs {
			refsCount++
		}
	}
	if refsCount != 1 {
		t.Errorf("expected exactly 1 refs relationship from A's side, got %d", refsCount)
	}
}

func TestRemoveRelationship_Refs_DeletesEitherDirection(t *testing.T) {
	t.Parallel()

	// Given — A refs B exists.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "refs-test")

	aOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role: issue.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.AddRelationship(ctx, aOut.Issue.ID(), service.RelationshipInput{
		Type: issue.RelRefs, TargetID: bOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — remove B refs A (reverse of stored direction).
	err = svc.RemoveRelationship(ctx, bOut.Issue.ID(), service.RelationshipInput{
		Type: issue.RelRefs, TargetID: aOut.Issue.ID(),
	}, author)
	// Then — the relationship should be gone from both sides.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showA, err := svc.ShowIssue(ctx, aOut.Issue.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, rel := range showA.Relationships {
		if rel.Type() == issue.RelRefs {
			t.Error("expected refs relationship to be removed from A's side")
		}
	}
}

// --- Doctor: no-ready-issues analysis ---

func TestDoctor_NoIssues_NoBlockerFindings(t *testing.T) {
	t.Parallel()

	// Given — an empty database with no issues.
	svc, _ := setupService(t)
	ctx := t.Context()

	// When
	result, err := svc.Doctor(ctx)
	// Then — no blocker findings should appear.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range result.Findings {
		if strings.HasPrefix(f.Category, "blocker_") {
			t.Errorf("unexpected blocker finding: %q", f.Category)
		}
	}
}

func TestDoctor_ReadyIssuesExist_NoBlockerFindings(t *testing.T) {
	t.Parallel()

	// Given — a database with a ready task (not blocked).
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	_, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "A ready task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range result.Findings {
		if strings.HasPrefix(f.Category, "blocker_") {
			t.Errorf("unexpected blocker finding: %q", f.Category)
		}
	}
}

func TestDoctor_AllClosed_NoBlockerFindings(t *testing.T) {
	t.Parallel()

	// Given — a database where all issues are closed.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	out, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Closable task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: out.Issue.ID(),
		ClaimID: out.ClaimID,
		Action:  service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range result.Findings {
		if strings.HasPrefix(f.Category, "blocker_") {
			t.Errorf("unexpected blocker finding: %q", f.Category)
		}
	}
}

func TestDoctor_BlockedByClaimedTask_NoBlockerFindings(t *testing.T) {
	t.Parallel()

	// Given — task A is blocked by claimed task B. B is claimed (actively
	// being worked on), so the chain resolves.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	blockerOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Blocker task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Blocked task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID(), service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: blockerOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx)
	// Then — no blocker findings because the blocker is claimed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range result.Findings {
		if strings.HasPrefix(f.Category, "blocker_") {
			t.Errorf("unexpected blocker finding: %q — %s", f.Category, f.Message)
		}
	}
}

func TestDoctor_BlockedByCloseEligibleEpic_ReportsBlockerCloseEligible(t *testing.T) {
	t.Parallel()

	// Given — task A is blocked by epic B whose only child C is closed.
	// Epic B is close-eligible, so closing it would unblock task A.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	epicOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleEpic,
		Title:  "Close-eligible epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	childOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:     issue.RoleTask,
		Title:    "Epic child",
		Author:   author,
		ParentID: epicOut.Issue.ID(),
		Claim:    true,
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: childOut.Issue.ID(),
		ClaimID: childOut.ClaimID,
		Action:  service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Blocked by epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked task: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID(), service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: epicOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx)
	// Then — should report blocker_close_eligible.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "blocker_close_eligible")
	if finding == nil {
		t.Fatal("expected 'blocker_close_eligible' finding")
	}
	if !strings.Contains(finding.Suggestion, "epic close-eligible") {
		t.Errorf("expected suggestion to mention 'epic close-eligible', got %q",
			finding.Suggestion)
	}
	if !slices.Contains(finding.IssueIDs, epicOut.Issue.ID().String()) {
		t.Errorf("expected epic %s in IssueIDs, got %v",
			epicOut.Issue.ID(), finding.IssueIDs)
	}
}

func TestDoctor_DeferredIssueBlocking_ReportsBlockerDeferred(t *testing.T) {
	t.Parallel()

	// Given — task A is blocked by deferred task B.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	blockerOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Deferred blocker",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}
	err = svc.TransitionState(ctx, service.TransitionInput{
		IssueID: blockerOut.Issue.ID(),
		ClaimID: blockerOut.ClaimID,
		Action:  service.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Blocked by deferred",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID(), service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: blockerOut.Issue.ID(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx)
	// Then — should report blocker_deferred and suggest undefer.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "blocker_deferred")
	if finding == nil {
		t.Fatal("expected 'blocker_deferred' finding")
	}
	if !strings.Contains(finding.Suggestion, "issue undefer") {
		t.Errorf("expected suggestion to mention 'issue undefer', got %q",
			finding.Suggestion)
	}
	if !slices.Contains(finding.IssueIDs, blockerOut.Issue.ID().String()) {
		t.Errorf("expected deferred blocker %s in IssueIDs, got %v",
			blockerOut.Issue.ID(), finding.IssueIDs)
	}
}

func TestDoctor_StaleClaim_ReportsStealable(t *testing.T) {
	t.Parallel()

	// Given — a task with a stale claim (last activity >2h ago with default
	// threshold).
	svc, repo := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	out, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Stale claim task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// Insert a claim with old last_activity directly into the repo.
	staleClaim := claim.ReconstructClaim(
		"stale-claim-id-1234",
		out.Issue.ID(),
		author,
		claim.DefaultStaleThreshold,
		time.Now().Add(-3*time.Hour), // 3h ago — past 2h threshold
	)
	if err := repo.CreateClaim(ctx, staleClaim); err != nil {
		t.Fatalf("precondition: create stale claim: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx)
	// Then — stale_claim finding should say "stealable".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "stale_claim")
	if finding == nil {
		t.Fatal("expected 'stale_claim' finding")
	}
	if !strings.Contains(finding.Message, "stealable") {
		t.Errorf("expected message to contain 'stealable', got %q", finding.Message)
	}
	if finding.Suggestion == "" {
		t.Error("expected suggestion for stealing the claim")
	}
}

func TestDoctor_LongClaim_ReportsLongHeld(t *testing.T) {
	t.Parallel()

	// Given — a task with a claim held for >2× default threshold (>4h) but
	// with a custom 24h threshold so it's not yet stale.
	svc, repo := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	out, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:   issue.RoleTask,
		Title:  "Long claim task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// Insert a claim with old last_activity but long threshold.
	longClaim := claim.ReconstructClaim(
		"long-claim-id-5678",
		out.Issue.ID(),
		author,
		24*time.Hour,                 // custom 24h threshold
		time.Now().Add(-5*time.Hour), // 5h ago — >2× default 2h but <24h threshold
	)
	if err := repo.CreateClaim(ctx, longClaim); err != nil {
		t.Fatalf("precondition: create long claim: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx)
	// Then — long_claim finding should appear.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "long_claim")
	if finding == nil {
		t.Fatal("expected 'long_claim' finding")
	}
	if !strings.Contains(finding.Message, "not yet stealable") {
		t.Errorf("expected message to contain 'not yet stealable', got %q", finding.Message)
	}
}

// findingByCategory returns the first finding with the given category, or nil.
func findingByCategory(findings []service.DoctorFinding, category string) *service.DoctorFinding {
	for i := range findings {
		if findings[i].Category == category {
			return &findings[i]
		}
	}
	return nil
}

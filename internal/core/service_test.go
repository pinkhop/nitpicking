package core_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func setupService(t *testing.T) (driving.Service, *memory.Repository) {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	ctx := context.Background()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	return svc, repo
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

func mustLabel(_ *testing.T, key, value string) driving.LabelInput {
	return driving.LabelInput{Key: key, Value: value}
}

// --- Init ---

func TestInit_ValidPrefix_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	// When
	err := svc.Init(t.Context(), "NP")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInit_InvalidPrefix_Fails(t *testing.T) {
	t.Parallel()

	// Given
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	// When
	err := svc.Init(t.Context(), "np")

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
	name, err := svc.AgentName(t.Context())
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
	output, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
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
	if output.Issue.State() != domain.StateOpen {
		t.Errorf("expected open state, got %s", output.Issue.State())
	}
}

func TestCreateIssue_WithClaim_ReturnsClaimID(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	output, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
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
	input := driving.CreateIssueInput{
		Role:           domain.RoleTask,
		Title:          "Idempotent task",
		Author:         author,
		IdempotencyKey: "idem-1",
	}

	// When — create twice with same key
	out1, err1 := svc.CreateIssue(t.Context(), input)
	out2, err2 := svc.CreateIssue(t.Context(), input)

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
	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
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
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: created.Issue.ID().String(),
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
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	bob := mustAuthor(t, "bob")
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: created.Issue.ID().String(),
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
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Action:  driving.ActionClose,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify issue is closed.
	show, _ := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if show.State != domain.StateClosed {
		t.Errorf("expected closed, got %v", show.State)
	}
}

func TestTransitionState_Release_ReturnsToDefault(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Action:  driving.ActionRelease,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if show.State != domain.StateOpen {
		t.Errorf("expected open after release, got %v", show.State)
	}
}

// --- CloseWithReason ---

func TestCloseWithReason_ClosesIssueAndAddsComment(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task to close with reason",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.CloseWithReason(t.Context(), driving.CloseWithReasonInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Reason:  "Implementation complete.",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, showErr := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if showErr != nil {
		t.Fatalf("show issue failed: %v", showErr)
	}
	if show.State != domain.StateClosed {
		t.Errorf("state: got %v, want %v", show.State, domain.StateClosed)
	}

	comments, listErr := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: created.Issue.ID().String(),
	})
	if listErr != nil {
		t.Fatalf("list comments failed: %v", listErr)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}
	if comments.Comments[0].Body != "Implementation complete." {
		t.Errorf("comment body: got %q, want %q",
			comments.Comments[0].Body, "Implementation complete.")
	}
	if comments.Comments[0].Author != author {
		t.Errorf("comment author: got %q, want %q",
			comments.Comments[0].Author, author)
	}
}

func TestCloseWithReason_EmptyReason_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "No reason task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.CloseWithReason(t.Context(), driving.CloseWithReasonInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Reason:  "",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestCloseWithReason_InvalidClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Invalid claim task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.CloseWithReason(t.Context(), driving.CloseWithReasonInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: "bogus-claim-id",
		Reason:  "Some reason.",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for invalid claim ID")
	}
}

func TestCloseWithReason_RecordsCommentAndStateHistory(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "History task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.CloseWithReason(t.Context(), driving.CloseWithReasonInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Reason:  "All done.",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hist, histErr := svc.ShowHistory(t.Context(), driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
	})
	if histErr != nil {
		t.Fatalf("show history failed: %v", histErr)
	}

	// Expect at least a comment-added and a state-changed event from CloseWithReason
	// (in addition to earlier create/claim events).
	var hasCommentEvent, hasStateEvent bool
	for _, entry := range hist.Entries {
		if entry.EventType == history.EventCommentAdded.String() {
			hasCommentEvent = true
		}
		if entry.EventType == history.EventStateChanged.String() {
			for _, change := range entry.Changes {
				if change.Field == "state" && change.After == domain.StateClosed.String() {
					hasStateEvent = true
				}
			}
		}
	}
	if !hasCommentEvent {
		t.Error("missing EventCommentAdded history entry")
	}
	if !hasStateEvent {
		t.Error("missing EventStateChanged history entry with closed state")
	}
}

func TestCloseWithReason_UnclosedChildren_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — an epic with an open child task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	epic, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Epic with open child",
		Author: author,
		Claim:  true,
	})
	_, childErr := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Open child",
		Author:   author,
		ParentID: epic.Issue.ID().String(),
	})
	if childErr != nil {
		t.Fatalf("precondition: create child failed: %v", childErr)
	}

	// When
	err := svc.CloseWithReason(t.Context(), driving.CloseWithReasonInput{
		IssueID: epic.Issue.ID().String(),
		ClaimID: epic.ClaimID,
		Reason:  "Attempting to close epic.",
	})

	// Then — should fail because child is not closed.
	if err == nil {
		t.Fatal("expected error for unclosed children")
	}
	if !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition, got: %v", err)
	}
}

func TestCloseWithReason_Atomic_NeitherCommentNorCloseOnFailure(t *testing.T) {
	t.Parallel()

	// Given — an epic with an open child (which will cause the close to fail).
	// This verifies that the comment is NOT persisted when the close fails,
	// proving atomicity.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	epic, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Epic for atomicity test",
		Author: author,
		Claim:  true,
	})
	_, childErr := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Open child",
		Author:   author,
		ParentID: epic.Issue.ID().String(),
	})
	if childErr != nil {
		t.Fatalf("precondition: create child failed: %v", childErr)
	}

	// When — attempt close (will fail due to unclosed child).
	_ = svc.CloseWithReason(t.Context(), driving.CloseWithReasonInput{
		IssueID: epic.Issue.ID().String(),
		ClaimID: epic.ClaimID,
		Reason:  "This should not persist.",
	})

	// Then — the comment must not have been persisted.
	comments, listErr := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: epic.Issue.ID().String(),
	})
	if listErr != nil {
		t.Fatalf("list comments failed: %v", listErr)
	}
	if len(comments.Comments) != 0 {
		t.Errorf("expected 0 comments after failed close, got %d", len(comments.Comments))
	}
}

// --- UpdateIssue ---

func TestUpdateIssue_ChangesTitle(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Original",
		Author: author,
		Claim:  true,
	})

	// When
	newTitle := "Updated title"
	err := svc.UpdateIssue(t.Context(), driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Title:   &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if show.Title != "Updated title" {
		t.Errorf("expected Updated title, got %q", show.Title)
	}
}

func TestUpdateIssue_ChangesDescription(t *testing.T) {
	t.Parallel()

	// Given — a claimed task with no description.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "update-agent")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Desc test", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — update only the description.
	desc := "New description"
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
		Description: &desc,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, _ := svc.ShowIssue(ctx, created.Issue.ID().String())
	if show.Description != "New description" {
		t.Errorf("description: got %q, want %q", show.Description, "New description")
	}
	// Title should be unchanged.
	if show.Title != "Desc test" {
		t.Errorf("title should be unchanged, got %q", show.Title)
	}
}

func TestUpdateIssue_ChangesPriority(t *testing.T) {
	t.Parallel()

	// Given — a claimed task with default priority.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "update-agent")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Priority test", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — update only priority to P0.
	p0 := domain.P0
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
		Priority: &p0,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, _ := svc.ShowIssue(ctx, created.Issue.ID().String())
	if show.Priority != domain.P0 {
		t.Errorf("priority: got %v, want %v", show.Priority, domain.P0)
	}
}

func TestUpdateIssue_SetsLabels(t *testing.T) {
	t.Parallel()

	// Given — a claimed task with no labels.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "update-agent")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Label set test", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	kindBug := driving.LabelInput{Key: "kind", Value: "bug"}

	// When — set a label.
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
		LabelSet: []driving.LabelInput{kindBug},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, _ := svc.ShowIssue(ctx, created.Issue.ID().String())
	val, exists := show.Labels["kind"]
	if !exists || val != "bug" {
		t.Errorf("expected label kind:bug, got exists=%v val=%q", exists, val)
	}
}

func TestUpdateIssue_RemovesLabels(t *testing.T) {
	t.Parallel()

	// Given — a claimed task with a label.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "update-agent")

	kindBug := driving.LabelInput{Key: "kind", Value: "bug"}
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Label remove test", Author: author,
		Claim: true, Labels: []driving.LabelInput{kindBug},
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — remove the label.
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
		LabelRemove: []string{"kind"},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, _ := svc.ShowIssue(ctx, created.Issue.ID().String())
	if len(show.Labels) != 0 {
		t.Errorf("expected 0 labels after removal, got %d", len(show.Labels))
	}
}

func TestUpdateIssue_SetLabel_RecordsLabelAddedHistory(t *testing.T) {
	t.Parallel()

	// Given — a claimed task with no labels.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "label-agent")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Label history test", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	kindBug := driving.LabelInput{Key: "kind", Value: "bug"}

	// When — set a label.
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID:  created.Issue.ID().String(),
		ClaimID:  created.ClaimID,
		LabelSet: []driving.LabelInput{kindBug},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("unexpected error fetching history: %v", err)
	}

	var found bool
	for _, e := range histOut.Entries {
		if e.EventType == history.EventLabelAdded.String() {
			found = true
			changes := e.Changes
			var hasLabel bool
			for _, c := range changes {
				if c.Field == "label" && c.After == "kind:bug" {
					hasLabel = true
				}
			}
			if !hasLabel {
				t.Errorf("expected label field change kind:bug, got %v", changes)
			}
		}
	}
	if !found {
		t.Error("expected label_added history entry, none found")
	}
}

func TestUpdateIssue_RemoveLabel_RecordsLabelRemovedHistory(t *testing.T) {
	t.Parallel()

	// Given — a claimed task with a label.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "label-agent")
	kindBug := driving.LabelInput{Key: "kind", Value: "bug"}
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Label remove history", Author: author,
		Claim: true, Labels: []driving.LabelInput{kindBug},
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — remove the label.
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID:     created.Issue.ID().String(),
		ClaimID:     created.ClaimID,
		LabelRemove: []string{"kind"},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("unexpected error fetching history: %v", err)
	}

	var found bool
	for _, e := range histOut.Entries {
		if e.EventType == history.EventLabelRemoved.String() {
			found = true
			changes := e.Changes
			var hasLabel bool
			for _, c := range changes {
				if c.Field == "label" && c.Before == "kind:bug" {
					hasLabel = true
				}
			}
			if !hasLabel {
				t.Errorf("expected label field change kind:bug in Before, got %v", changes)
			}
		}
	}
	if !found {
		t.Error("expected label_removed history entry, none found")
	}
}

func TestUpdateIssue_MultipleFieldsSimultaneously(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "update-agent")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Multi-update", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — update title, description, and priority in one call.
	newTitle := "Updated multi"
	newDesc := "Updated description"
	p1 := domain.P1
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
		Title: &newTitle, Description: &newDesc, Priority: &p1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, _ := svc.ShowIssue(ctx, created.Issue.ID().String())
	if show.Title != "Updated multi" {
		t.Errorf("title: got %q, want %q", show.Title, "Updated multi")
	}
	if show.Description != "Updated description" {
		t.Errorf("description: got %q, want %q", show.Description, "Updated description")
	}
	if show.Priority != domain.P1 {
		t.Errorf("priority: got %v, want %v", show.Priority, domain.P1)
	}
}

func TestUpdateIssue_NoChanges_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "update-agent")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "No-op update", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — update with no fields set.
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
	})
	// Then — succeeds without error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, _ := svc.ShowIssue(ctx, created.Issue.ID().String())
	if show.Title != "No-op update" {
		t.Errorf("title should be unchanged, got %q", show.Title)
	}
}

// --- OneShotUpdate ---

func TestOneShotUpdate_ChangesAndReleases(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Original",
		Author: author,
	})

	// When
	newTitle := "Quick fix"
	err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
		Title:   &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if show.Title != "Quick fix" {
		t.Errorf("expected Quick fix, got %q", show.Title)
	}
	if show.State != domain.StateOpen {
		t.Errorf("expected open after one-shot, got %v", show.State)
	}
}

// --- AddComment ---

func TestAddComment_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
		Body:    "This is a comment.",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Comment.Body != "This is a comment." {
		t.Errorf("expected comment body, got %q", output.Comment.Body)
	}
}

func TestAddComment_DeletedIssue_Fails(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// Delete the domain.
	_ = svc.DeleteIssue(t.Context(), driving.DeleteInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
	})

	// When
	_, err := svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: created.Issue.ID().String(),
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
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	_ = svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Action:  driving.ActionClose,
	})

	// When — comments CAN be added to closed issues
	_, err := svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
		Body:    "Post-mortem comment",
	})
	// Then
	if err != nil {
		t.Fatalf("expected success adding comment to closed issue, got: %v", err)
	}
}

func TestAddComment_RecordsCommentAddedHistory(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	_, err = svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
		Body:    "My observation",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	histOut, err := svc.ShowHistory(t.Context(), driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("unexpected error fetching history: %v", err)
	}

	var found bool
	for _, e := range histOut.Entries {
		if e.EventType == history.EventCommentAdded.String() {
			found = true
			changes := e.Changes
			if len(changes) < 1 {
				t.Fatal("expected at least one field change on comment_added event")
			}
			var hasBody bool
			for _, c := range changes {
				if c.Field == "body" && c.After == "My observation" {
					hasBody = true
				}
			}
			if !hasBody {
				t.Errorf("expected field change for body, got %v", changes)
			}
		}
	}
	if !found {
		t.Error("expected comment_added history entry, none found")
	}
}

// --- ShowIssue ---

func TestShowIssue_PopulatesFlatFields(t *testing.T) {
	t.Parallel()

	// Given — a task with description, acceptance criteria, labels, and a parent.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	epic, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Parent Epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:               domain.RoleTask,
		Title:              "Flat fields task",
		Description:        "A description",
		AcceptanceCriteria: "AC one",
		Priority:           domain.P1,
		ParentID:           epic.Issue.ID().String(),
		Labels:             []driving.LabelInput{mustLabel(t, "kind", "bug")},
		Author:             author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	show, err := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.ID != created.Issue.ID().String() {
		t.Errorf("ID: got %q, want %q", show.ID, created.Issue.ID().String())
	}
	if show.Role != domain.RoleTask {
		t.Errorf("Role: got %q, want %q", show.Role, domain.RoleTask)
	}
	if show.Title != "Flat fields task" {
		t.Errorf("Title: got %q, want %q", show.Title, "Flat fields task")
	}
	if show.Description != "A description" {
		t.Errorf("Description: got %q, want %q", show.Description, "A description")
	}
	if show.AcceptanceCriteria != "AC one" {
		t.Errorf("AcceptanceCriteria: got %q, want %q", show.AcceptanceCriteria, "AC one")
	}
	if show.Priority != domain.P1 {
		t.Errorf("Priority: got %q, want %q", show.Priority, domain.P1)
	}
	if show.State != domain.StateOpen {
		t.Errorf("State: got %v, want %v", show.State, domain.StateOpen)
	}
	if show.ParentID != epic.Issue.ID().String() {
		t.Errorf("ParentID: got %q, want %q", show.ParentID, epic.Issue.ID().String())
	}
	if show.Labels == nil {
		t.Fatal("Labels: expected non-nil map")
	}
	if show.Labels["kind"] != "bug" {
		t.Errorf("Labels[kind]: got %q, want %q", show.Labels["kind"], "bug")
	}
	if show.CreatedAt.IsZero() {
		t.Error("CreatedAt: expected non-zero time")
	}
}

func TestShowIssue_FlatFields_NoParentOrLabels(t *testing.T) {
	t.Parallel()

	// Given — a task with no parent and no labels.
	svc, _ := setupService(t)
	author := mustAuthor(t, "bob")

	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Orphan task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	show, err := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.ParentID != "" {
		t.Errorf("ParentID: got %q, want empty", show.ParentID)
	}
	if show.Labels == nil {
		t.Fatal("Labels: expected non-nil map even when empty")
	}
	if len(show.Labels) != 0 {
		t.Errorf("Labels: expected empty map, got %v", show.Labels)
	}
}

func TestShowIssue_ReturnsRevisionAndAuthor(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	show, err := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.Revision != 0 {
		t.Errorf("expected revision 0, got %d", show.Revision)
	}
	if show.Author != author {
		t.Errorf("expected author alice, got %s", show.Author)
	}
}

func TestShowIssue_IncludesCommentCount(t *testing.T) {
	t.Parallel()

	// Given: an issue with two comments.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task with comments", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: author, Body: "Comment one",
	})
	if err != nil {
		t.Fatalf("precondition: add comment 1: %v", err)
	}
	_, err = svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: author, Body: "Comment two",
	})
	if err != nil {
		t.Fatalf("precondition: add comment 2: %v", err)
	}

	// When
	show, err := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.CommentCount != 2 {
		t.Errorf("expected CommentCount 2, got %d", show.CommentCount)
	}
}

func TestShowIssue_ClaimedIssue_IncludesClaimedAt(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Claim timestamp task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: created.Issue.ID().String(), Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: claim: %v", err)
	}

	// When
	result, err := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ClaimedAt.IsZero() {
		t.Error("expected ClaimedAt to be non-zero for a claimed issue")
	}
}

func TestShowIssue_UnclaimedIssue_ClaimedAtIsZero(t *testing.T) {
	t.Parallel()

	// Given — an unclaimed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Unclaimed task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	result, err := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ClaimedAt.IsZero() {
		t.Errorf("expected ClaimedAt to be zero for unclaimed issue, got %v", result.ClaimedAt)
	}
}

func TestShowIssue_IncludesChildren(t *testing.T) {
	t.Parallel()

	// Given — an epic with two children.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	epic, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Parent epic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}
	_, err = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child A", Author: author, ParentID: epic.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child A: %v", err)
	}
	_, err = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child B", Author: author, ParentID: epic.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child B: %v", err)
	}

	// When
	result, err := svc.ShowIssue(t.Context(), epic.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ChildCount != 2 {
		t.Errorf("expected ChildCount 2, got %d", result.ChildCount)
	}
	if len(result.Children) != 2 {
		t.Errorf("expected 2 Children items, got %d", len(result.Children))
	}
}

func TestShowIssue_ChildrenIncludeSecondaryState(t *testing.T) {
	t.Parallel()

	// Given — an epic with two child tasks: one ready and one blocked.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	epic, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Parent epic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}
	readyChild, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Ready child", Author: author, ParentID: epic.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create ready child: %v", err)
	}
	blocker, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}
	blockedChild, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked child", Author: author, ParentID: epic.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create blocked child: %v", err)
	}
	err = svc.AddRelationship(t.Context(), blockedChild.Issue.ID().String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: blocker.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.ShowIssue(t.Context(), epic.Issue.ID().String())
	// Then — each child must have a populated DisplayStatus with secondary
	// state annotation.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(result.Children))
	}
	statusByID := make(map[string]string, len(result.Children))
	for _, c := range result.Children {
		statusByID[c.ID] = c.DisplayStatus
	}
	readyStatus := statusByID[readyChild.Issue.ID().String()]
	if readyStatus != "open (ready)" {
		t.Errorf("ready child: got DisplayStatus %q, want %q", readyStatus, "open (ready)")
	}
	blockedStatus := statusByID[blockedChild.Issue.ID().String()]
	if blockedStatus != "open (blocked)" {
		t.Errorf("blocked child: got DisplayStatus %q, want %q", blockedStatus, "open (blocked)")
	}
}

func TestShowIssue_IncludesAllComments(t *testing.T) {
	t.Parallel()

	// Given — a task with 4 comments.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Commented task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	for i, body := range []string{"First", "Second", "Third", "Fourth"} {
		_, err = svc.AddComment(t.Context(), driving.AddCommentInput{
			IssueID: created.Issue.ID().String(), Author: author, Body: body,
		})
		if err != nil {
			t.Fatalf("precondition: add comment %d: %v", i+1, err)
		}
	}

	// When — showing the domain.
	result, err := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	// Then — all comments are included.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CommentCount != 4 {
		t.Errorf("expected CommentCount 4, got %d", result.CommentCount)
	}
	if len(result.Comments) != 4 {
		t.Fatalf("expected 4 Comments, got %d", len(result.Comments))
	}
	if result.Comments[0].Body != "First" {
		t.Errorf("expected first comment body 'First', got %q", result.Comments[0].Body)
	}
	if result.Comments[3].Body != "Fourth" {
		t.Errorf("expected last comment body 'Fourth', got %q", result.Comments[3].Body)
	}
}

// --- ListIssues ---

func TestListIssues_FilterByReady(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	// Create two tasks — one open (ready), one claimed (not ready).
	_, _ = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Ready task",
		Author: author,
	})
	_, _ = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Claimed task",
		Author: author,
		Claim:  true,
	})

	// When
	output, err := svc.ListIssues(t.Context(), driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{Ready: true},
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

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked task: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(),
		driving.RelationshipInput{Type: domain.RelBlockedBy, TargetID: blockerOut.Issue.ID().String()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// Close the blocker.
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: blockerOut.Issue.ID().String(), ClaimID: blockerOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close blocker: %v", err)
	}

	// When — list ready issues.
	listOut, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{Ready: true},
	})
	// Then — the blocked task should appear as ready.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, item := range listOut.Items {
		if item.ID == blockedOut.Issue.ID().String() {
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

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(),
		driving.RelationshipInput{Type: domain.RelBlockedBy, TargetID: blockerOut.Issue.ID().String()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: blockerOut.Issue.ID().String(), ClaimID: blockerOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close blocker: %v", err)
	}

	// When — show the blocked task.
	showOut, err := svc.ShowIssue(ctx, blockedOut.Issue.ID().String())
	// Then — the blocked task should be ready.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !showOut.IsReady {
		t.Error("expected blocked task to be ready when blocker is closed")
	}
}

func TestShowIssue_IncludesSyntheticChildOfRelationship(t *testing.T) {
	t.Parallel()

	// Given — a task with a parent epic.
	svc, _ := setupService(t)
	author := mustAuthor(t, "agent-c")

	epicOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Parent epic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	taskOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child task", ParentID: epicOut.Issue.ID().String(), Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When — show the child task.
	showOut, err := svc.ShowIssue(t.Context(), taskOut.Issue.ID().String())
	// Then — relationships include a child_of entry.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, r := range showOut.Relationships {
		if r.Type == domain.RelChildOf.String() && r.TargetID == epicOut.Issue.ID().String() {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected synthetic child_of relationship in ShowIssueOutput")
	}
}

func TestShowIssue_IncludesSyntheticParentOfRelationships(t *testing.T) {
	t.Parallel()

	// Given — an epic with two child tasks.
	svc, _ := setupService(t)
	author := mustAuthor(t, "agent-d")

	epicOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Parent epic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	childA, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child A", ParentID: epicOut.Issue.ID().String(), Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create child A: %v", err)
	}

	childB, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child B", ParentID: epicOut.Issue.ID().String(), Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create child B: %v", err)
	}

	// When — show the epic.
	showOut, err := svc.ShowIssue(t.Context(), epicOut.Issue.ID().String())
	// Then — relationships include parent_of entries for both children.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parentOfCount := 0
	childIDs := map[string]bool{childA.Issue.ID().String(): false, childB.Issue.ID().String(): false}
	for _, r := range showOut.Relationships {
		if r.Type == domain.RelParentOf.String() {
			parentOfCount++
			childIDs[r.TargetID] = true
		}
	}
	if parentOfCount != 2 {
		t.Errorf("parent_of count: got %d, want 2", parentOfCount)
	}
	for id, found := range childIDs {
		if !found {
			t.Errorf("missing parent_of for child %s", id)
		}
	}
}

func TestListIssues_ExcludeClosed_HidesClosedIssues(t *testing.T) {
	t.Parallel()

	// Given: one open task and one closed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Open task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closed, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Closed task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed task: %v", err)
	}
	err = svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: closed.Issue.ID().String(),
		ClaimID: closed.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When: listing with ExcludeClosed.
	output, err := svc.ListIssues(t.Context(), driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{ExcludeClosed: true},
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

	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Open task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closed, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Closed task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed task: %v", err)
	}
	err = svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: closed.Issue.ID().String(),
		ClaimID: closed.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When: ExcludeClosed is set but States explicitly requests closed — States
	// takes precedence because it represents an explicit user intent.
	output, err := svc.ListIssues(t.Context(), driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{
			ExcludeClosed: true,
			States:        []domain.State{domain.StateClosed},
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

	a, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}
	b, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create B: %v", err)
	}
	err = svc.AddRelationship(t.Context(), a.Issue.ID().String(),
		driving.RelationshipInput{Type: domain.RelBlockedBy, TargetID: b.Issue.ID().String()}, author)
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
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.DeleteIssue(t.Context(), driving.DeleteInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Show should fail.
	_, err = svc.ShowIssue(t.Context(), created.Issue.ID().String())
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
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.ExtendStaleThreshold(t.Context(), created.Issue.ID().String(), created.ClaimID, 12*time.Hour)
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
	created, _ := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.ShowHistory(t.Context(), driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
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
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	parentOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Parent", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create parent: %v", err)
	}

	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child", Author: author, ParentID: parentOut.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// When — try to close the parent.
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: parentOut.Issue.ID().String(),
		ClaimID: parentOut.ClaimID,
		Action:  driving.ActionClose,
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
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	parentOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Parent", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create parent: %v", err)
	}

	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child", Author: author, Claim: true, ParentID: parentOut.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// Close the child first.
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childOut.Issue.ID().String(),
		ClaimID: childOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child: %v", err)
	}

	// Claim the parent.
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: parentOut.Issue.ID().String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim parent: %v", err)
	}

	// When — close the parent.
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: parentOut.Issue.ID().String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
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

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelBlockedBy, TargetID: blockerOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — list all issues.
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{ExcludeClosed: true},
		Limit:  -1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then — the blocked issue should have IsBlocked true.
	for _, item := range result.Items {
		if item.ID == blockedOut.Issue.ID().String() {
			if !item.IsBlocked {
				t.Error("expected IsBlocked=true for blocked issue")
			}
		} else if item.ID == blockerOut.Issue.ID().String() {
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

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Was blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelBlockedBy, TargetID: blockerOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: blockerOut.Issue.ID().String(), ClaimID: blockerOut.ClaimID, Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close blocker: %v", err)
	}

	// When — list non-closed issues.
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{ExcludeClosed: true},
		Limit:  -1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then — the previously-blocked issue should have IsBlocked false.
	for _, item := range result.Items {
		if item.ID == blockedOut.Issue.ID().String() && item.IsBlocked {
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

	aOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create B: %v", err)
	}

	// When — add A refs B.
	err = svc.AddRelationship(ctx, aOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelRefs, TargetID: bOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add refs: %v", err)
	}

	// Then — ShowIssue for B should include the refs relationship.
	showB, err := svc.ShowIssue(ctx, bOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundRefs := false
	for _, rel := range showB.Relationships {
		if rel.Type == domain.RelRefs.String() {
			foundRefs = true
			// From B's perspective, B is the source and A is the target.
			if rel.SourceID != bOut.Issue.ID().String() || rel.TargetID != aOut.Issue.ID().String() {
				t.Errorf("expected B→refs→A, got %s→refs→%s", rel.SourceID, rel.TargetID)
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

	aOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	err = svc.AddRelationship(ctx, aOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelRefs, TargetID: bOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add refs: %v", err)
	}

	// When — add B refs A (reverse direction of same symmetric link).
	err = svc.AddRelationship(ctx, bOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelRefs, TargetID: aOut.Issue.ID().String(),
	}, author)
	// Then — should succeed (idempotent), and only one relationship should exist.
	if err != nil {
		t.Fatalf("unexpected error adding reverse refs: %v", err)
	}
	showA, err := svc.ShowIssue(ctx, aOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	refsCount := 0
	for _, rel := range showA.Relationships {
		if rel.Type == domain.RelRefs.String() {
			refsCount++
		}
	}
	if refsCount != 1 {
		t.Errorf("expected exactly 1 refs relationship from A's side, got %d", refsCount)
	}
}

func TestAddRelationship_BlockedBy_AppearsOnSource(t *testing.T) {
	t.Parallel()

	// Given — two tasks.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "block-agent")

	aOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create B: %v", err)
	}

	// When — add A blocked_by B.
	err = svc.AddRelationship(ctx, aOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelBlockedBy, TargetID: bOut.Issue.ID().String(),
	}, author)
	// Then — A's relationships include blocked_by B.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showA, err := svc.ShowIssue(ctx, aOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("show A: %v", err)
	}
	found := false
	for _, rel := range showA.Relationships {
		if rel.Type == domain.RelBlockedBy.String() && rel.TargetID == bOut.Issue.ID().String() {
			found = true
		}
	}
	if !found {
		t.Error("expected blocked_by relationship on A targeting B")
	}
}

func TestAddRelationship_Blocks_CreatesInverseBlockedBy(t *testing.T) {
	t.Parallel()

	// Given — two tasks.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "block-agent")

	aOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create B: %v", err)
	}

	// When — add A blocks B.
	err = svc.AddRelationship(ctx, aOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelBlocks, TargetID: bOut.Issue.ID().String(),
	}, author)
	// Then — B's relationships include the blocks relationship from A's
	// perspective (A is the source, B is the target, type is blocks).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showB, err := svc.ShowIssue(ctx, bOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("show B: %v", err)
	}
	found := false
	for _, rel := range showB.Relationships {
		if rel.Type == domain.RelBlocks.String() && rel.SourceID == aOut.Issue.ID().String() {
			found = true
		}
	}
	if !found {
		t.Error("expected blocks relationship visible from B's side (A blocks B)")
	}
}

func TestAddRelationship_NonExistentTarget_Fails(t *testing.T) {
	t.Parallel()

	// Given — one task exists, target does not.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "block-agent")

	aOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Source task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}

	fakeTarget, err := domain.ParseID("NP-zzzzz")
	if err != nil {
		t.Fatalf("precondition: parse ID: %v", err)
	}

	// When — add a relationship to a non-existent target.
	err = svc.AddRelationship(ctx, aOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelBlockedBy, TargetID: fakeTarget.String(),
	}, author)
	// Then — fails with not-found.
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAddRelationship_SelfReference_Fails(t *testing.T) {
	t.Parallel()

	// Given — one task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "block-agent")

	aOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Self-ref task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create A: %v", err)
	}

	// When — add A blocked_by A.
	err = svc.AddRelationship(ctx, aOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelBlockedBy, TargetID: aOut.Issue.ID().String(),
	}, author)
	// Then — fails (self-referencing not allowed).
	if err == nil {
		t.Fatal("expected error for self-referencing relationship, got nil")
	}
}

func TestRemoveRelationship_Refs_DeletesEitherDirection(t *testing.T) {
	t.Parallel()

	// Given — A refs B exists.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "refs-test")

	aOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	bOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	err = svc.AddRelationship(ctx, aOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelRefs, TargetID: bOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — remove B refs A (reverse of stored direction).
	err = svc.RemoveRelationship(ctx, bOut.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelRefs, TargetID: aOut.Issue.ID().String(),
	}, author)
	// Then — the relationship should be gone from both sides.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showA, err := svc.ShowIssue(ctx, aOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, rel := range showA.Relationships {
		if rel.Type == domain.RelRefs.String() {
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
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
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

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "A ready task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
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

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Closable task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: out.Issue.ID().String(),
		ClaimID: out.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
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

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Blocker task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Blocked task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: blockerOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
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

func TestDoctor_BlockedByCloseCompletedEpic_ReportsBlockerCloseCompleted(t *testing.T) {
	t.Parallel()

	// Given — task A is blocked by epic B whose only child C is closed.
	// Epic B is completed, so closing it would unblock task A.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Completed epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Epic child",
		Author:   author,
		ParentID: epicOut.Issue.ID().String(),
		Claim:    true,
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childOut.Issue.ID().String(),
		ClaimID: childOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Blocked by epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked task: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: epicOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report blocker_close_completed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "blocker_close_completed")
	if finding == nil {
		t.Fatal("expected 'blocker_close_completed' finding")
	}
	if finding.Action == nil || finding.Action.Kind != driving.ActionKindCloseCompleted {
		t.Errorf("expected action kind %q, got %v", driving.ActionKindCloseCompleted, finding.Action)
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

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Deferred blocker",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: blockerOut.Issue.ID().String(),
		ClaimID: blockerOut.ClaimID,
		Action:  driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Blocked by deferred",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: blockerOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report blocker_deferred and suggest undefer.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "blocker_deferred")
	if finding == nil {
		t.Fatal("expected 'blocker_deferred' finding")
	}
	if finding.Action == nil || finding.Action.Kind != driving.ActionKindUndefer {
		t.Errorf("expected action kind %q, got %v", driving.ActionKindUndefer, finding.Action)
	}
	if !slices.Contains(finding.IssueIDs, blockerOut.Issue.ID().String()) {
		t.Errorf("expected deferred blocker %s in IssueIDs, got %v",
			blockerOut.Issue.ID(), finding.IssueIDs)
	}
}

func TestDoctor_V2Schema_NoSchemaMigrationFinding(t *testing.T) {
	t.Parallel()

	// Given — a fresh in-memory repository, which always reports schema version 2.
	svc, _ := setupService(t)
	ctx := t.Context()

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — no schema_migration_required finding; in-memory store is always v2.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "schema_migration_required")
	if finding != nil {
		t.Errorf("unexpected schema_migration_required finding on v2 database: %v", finding.Message)
	}
}

func TestDoctor_CloseCompletedEpic_ReportsCloseCompleted(t *testing.T) {
	t.Parallel()

	// Given — an epic with all children closed (completed).
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Completed epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Epic child",
		Author:   author,
		ParentID: epicOut.Issue.ID().String(),
		Claim:    true,
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childOut.Issue.ID().String(),
		ClaimID: childOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report close_completed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "close_completed")
	if finding == nil {
		t.Fatal("expected 'close_completed' finding")
	}
	if !slices.Contains(finding.IssueIDs, epicOut.Issue.ID().String()) {
		t.Errorf("expected epic %s in IssueIDs, got %v", epicOut.Issue.ID(), finding.IssueIDs)
	}
}

func TestDoctor_ClosedParent_ReportsClosedParent(t *testing.T) {
	t.Parallel()

	// Given — an open task whose parent epic is closed.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Will-be-closed epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task",
		Author:   author,
		ParentID: epicOut.Issue.ID().String(),
		Claim:    true,
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childOut.Issue.ID().String(),
		ClaimID: childOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child: %v", err)
	}

	// Close the epic.
	epicClaim, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: epicOut.Issue.ID().String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim epic: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: epicOut.Issue.ID().String(),
		ClaimID: epicClaim.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close epic: %v", err)
	}

	// Create a new open task under the closed epic.
	orphanOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Orphaned by closed parent",
		Author:   author,
		ParentID: epicOut.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create orphan: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report closed_parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "closed_parent")
	if finding == nil {
		t.Fatal("expected 'closed_parent' finding")
	}
	if !slices.Contains(finding.IssueIDs, orphanOut.Issue.ID().String()) {
		t.Errorf("expected orphan %s in IssueIDs, got %v", orphanOut.Issue.ID(), finding.IssueIDs)
	}
}

func TestDoctor_LowPriorityBlocker_ReportsPriorityInversion(t *testing.T) {
	t.Parallel()

	// Given — P1 task blocked by P4 task.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Low priority blocker",
		Author:   author,
		Priority: domain.P4,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "High priority blocked",
		Author:   author,
		Priority: domain.P1,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: blockerOut.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report priority_inversion.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "priority_inversion")
	if finding == nil {
		t.Fatal("expected 'priority_inversion' finding")
	}
	if !strings.Contains(finding.Message, "P4") || !strings.Contains(finding.Message, "P1") {
		t.Errorf("expected message to mention P4 and P1, got %q", finding.Message)
	}
}

func TestDoctor_LowPriorityParent_ReportsPriorityInversion(t *testing.T) {
	t.Parallel()

	// Given — P3 epic with P1 child task.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleEpic,
		Title:    "Low priority epic",
		Author:   author,
		Priority: domain.P3,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "High priority child",
		Author:   author,
		Priority: domain.P1,
		ParentID: epicOut.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report priority_inversion for parent-child.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "priority_inversion")
	if finding == nil {
		t.Fatal("expected 'priority_inversion' finding")
	}
	if !strings.Contains(finding.Message, "P3") || !strings.Contains(finding.Message, "P1") {
		t.Errorf("expected message to mention P3 and P1, got %q", finding.Message)
	}
}

func TestDoctor_OrphanTask_ReportsOrphanTask(t *testing.T) {
	t.Parallel()

	// Given — an open task with no parent.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Orphan task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report orphan_task.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "orphan_task")
	if finding == nil {
		t.Fatal("expected 'orphan_task' finding")
	}
}

func TestDoctor_MissingKindLabel_ReportsMissingLabel(t *testing.T) {
	t.Parallel()

	// Given — an open task without a kind label.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "No kind label",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should report missing_label.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "missing_label")
	if finding == nil {
		t.Fatal("expected 'missing_label' finding")
	}
}

func TestDoctor_TaskWithKindLabel_NoMissingLabel(t *testing.T) {
	t.Parallel()

	// Given — an open task WITH a kind label.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "doctor-test")

	lbl := driving.LabelInput{Key: "kind", Value: "feature"}
	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Has kind label",
		Author: author,
		Labels: []driving.LabelInput{lbl},
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	result, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — should NOT report missing_label.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finding := findingByCategory(result.Findings, "missing_label")
	if finding != nil {
		t.Error("expected no 'missing_label' finding when kind label is set")
	}
}

// --- CreateIssue (Epic and Parent) ---

func TestCreateIssue_Epic_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "epic-agent")

	// When
	output, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Auth overhaul", Author: author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Issue.Role() != domain.RoleEpic {
		t.Errorf("role: got %v, want %v", output.Issue.Role(), domain.RoleEpic)
	}
	if output.Issue.Title() != "Auth overhaul" {
		t.Errorf("title: got %q, want %q", output.Issue.Title(), "Auth overhaul")
	}
}

func TestCreateIssue_TaskWithParentEpic_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — an epic exists.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "parent-agent")

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Parent epic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	// When — create a task under the epic.
	taskOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child task", Author: author,
		ParentID: epicOut.Issue.ID().String(),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskOut.Issue.ParentID() != epicOut.Issue.ID() {
		t.Errorf("parent_id: got %v, want %v", taskOut.Issue.ParentID(), epicOut.Issue.ID())
	}
}

func TestCreateIssue_NonExistentParent_Fails(t *testing.T) {
	t.Parallel()

	// Given — no issues exist.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "parent-agent")

	fakeParent, err := domain.ParseID("NP-zzzzz")
	if err != nil {
		t.Fatalf("precondition: parse ID: %v", err)
	}

	// When — create a task with a non-existent parent.
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Orphan task", Author: author,
		ParentID: fakeParent.String(),
	})
	// Then — fails with not-found error.
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateIssue_TaskParentIsTask_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — the domain does not restrict parent role; a task can parent
	// another task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "parent-agent")

	parentTask, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Parent task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create parent task: %v", err)
	}

	// When — create a task whose parent is also a task.
	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child of task", Author: author,
		ParentID: parentTask.Issue.ID().String(),
	})
	// Then — succeeds (no role restriction in the domain).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if childOut.Issue.ParentID() != parentTask.Issue.ID() {
		t.Errorf("parent_id: got %v, want %v", childOut.Issue.ParentID(), parentTask.Issue.ID())
	}
}

func TestCreateIssue_ExceedsMaxDepth_Fails(t *testing.T) {
	t.Parallel()

	// Given — a chain at MaxDepth (3): root → child → grandchild (task).
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "depth-agent")

	root, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Root (level 1)", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create root: %v", err)
	}
	child, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Child (level 2)", Author: author,
		ParentID: root.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}
	grandchild, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Grandchild (level 3)", Author: author,
		ParentID: child.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create grandchild: %v", err)
	}

	// When — attempt to create a great-grandchild (level 4, exceeds MaxDepth=3).
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Great-grandchild (level 4)", Author: author,
		ParentID: grandchild.Issue.ID().String(),
	})
	// Then — fails with ErrDepthExceeded.
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Errorf("expected ErrDepthExceeded, got %v", err)
	}
}

func TestCreateIssue_TaskAtMaxDepth_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — root epic (level 1) → child epic (level 2).
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "depth-agent")

	root, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Root (level 1)", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create root: %v", err)
	}
	child, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Child (level 2)", Author: author,
		ParentID: root.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// When — create a task at level 3 (MaxDepth). Tasks are leaf nodes, so
	// this should succeed.
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task at level 3", Author: author,
		ParentID: child.Issue.ID().String(),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateIssue_EpicAtMaxDepth_Fails(t *testing.T) {
	t.Parallel()

	// Given — root epic (level 1) → child epic (level 2).
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "depth-agent")

	root, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Root (level 1)", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create root: %v", err)
	}
	child, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Child (level 2)", Author: author,
		ParentID: root.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// When — attempt to create an epic at level 3. Epics organize children,
	// but level 3 is MaxDepth — no children can exist below it. This is
	// structurally invalid.
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Epic at level 3", Author: author,
		ParentID: child.Issue.ID().String(),
	})

	// Then — fails with ErrDepthExceeded.
	if !errors.Is(err, domain.ErrDepthExceeded) {
		t.Errorf("expected ErrDepthExceeded, got %v", err)
	}
}

// --- GC ---

func TestGC_NoDeletions_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a service with one open task (nothing to GC).
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "gc-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Active task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — run GC.
	output, err := svc.GC(ctx, driving.GCInput{})
	// Then — succeeds with no error and reports zero removals.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.DeletedIssuesRemoved != 0 {
		t.Errorf("deleted issues removed = %d, want 0", output.DeletedIssuesRemoved)
	}
	if output.ClosedIssuesRemoved != 0 {
		t.Errorf("closed issues removed = %d, want 0", output.ClosedIssuesRemoved)
	}

	// Open task is still visible.
	list, err := svc.ListIssues(ctx, driving.ListIssuesInput{})
	if err != nil {
		t.Fatalf("unexpected error listing issues: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 issue after GC, got %d", len(list.Items))
	}
}

func TestGC_RemovesSoftDeletedIssues(t *testing.T) {
	t.Parallel()

	// Given — one soft-deleted task and one open task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "gc-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Surviving task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create surviving task: %v", err)
	}

	doomed, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Doomed task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create doomed task: %v", err)
	}
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: doomed.Issue.ID().String(), ClaimID: doomed.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete issue: %v", err)
	}

	// When — run GC.
	output, err := svc.GC(ctx, driving.GCInput{})
	// Then — only the surviving task remains and deleted count is 1.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.DeletedIssuesRemoved != 1 {
		t.Errorf("deleted issues removed = %d, want 1", output.DeletedIssuesRemoved)
	}
	if output.ClosedIssuesRemoved != 0 {
		t.Errorf("closed issues removed = %d, want 0", output.ClosedIssuesRemoved)
	}

	list, err := svc.ListIssues(ctx, driving.ListIssuesInput{})
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 issue after GC, got %d", len(list.Items))
	}

	// The deleted issue should be truly gone (not even show with IncludeDeleted).
	_, err = svc.ShowIssue(ctx, doomed.Issue.ID().String())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected deleted issue to be gone after GC, got err: %v", err)
	}
}

func TestGC_IncludeClosed_RemovesClosedIssues(t *testing.T) {
	t.Parallel()

	// Given — one closed task and one open task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "gc-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Open task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closed, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Closed task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed task: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: closed.Issue.ID().String(), ClaimID: closed.ClaimID, Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When — run GC with IncludeClosed.
	output, err := svc.GC(ctx, driving.GCInput{IncludeClosed: true})
	// Then — the closed issue is removed and closed count is 1.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.DeletedIssuesRemoved != 0 {
		t.Errorf("deleted issues removed = %d, want 0", output.DeletedIssuesRemoved)
	}
	if output.ClosedIssuesRemoved != 1 {
		t.Errorf("closed issues removed = %d, want 1", output.ClosedIssuesRemoved)
	}

	list, err := svc.ListIssues(ctx, driving.ListIssuesInput{})
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 issue after GC, got %d", len(list.Items))
	}
}

func TestGC_PreservesOpenAndClaimedIssues(t *testing.T) {
	t.Parallel()

	// Given — one open task and one claimed task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "gc-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Open task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Claimed task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create claimed task: %v", err)
	}

	// When — run GC (even with IncludeClosed, open and claimed survive).
	output, err := svc.GC(ctx, driving.GCInput{IncludeClosed: true})
	// Then — both issues remain and counts are zero.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.DeletedIssuesRemoved != 0 {
		t.Errorf("deleted issues removed = %d, want 0", output.DeletedIssuesRemoved)
	}
	if output.ClosedIssuesRemoved != 0 {
		t.Errorf("closed issues removed = %d, want 0", output.ClosedIssuesRemoved)
	}

	list, err := svc.ListIssues(ctx, driving.ListIssuesInput{})
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}
	if len(list.Items) != 2 {
		t.Errorf("expected 2 issues after GC, got %d", len(list.Items))
	}
}

// --- ShowComment ---

func TestShowComment_ExistingComment_ReturnsComment(t *testing.T) {
	t.Parallel()

	// Given — an issue with a comment.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "comment-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Comment target", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	addOut, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: author, Body: "First comment",
	})
	if err != nil {
		t.Fatalf("precondition: add comment: %v", err)
	}
	commentID := addOut.Comment.CommentID

	// When — show the comment by ID.
	result, err := svc.ShowComment(ctx, commentID)
	// Then — returns the correct comment.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Body != "First comment" {
		t.Errorf("body: got %q, want %q", result.Body, "First comment")
	}
	if result.CommentID != commentID {
		t.Errorf("id: got %d, want %d", result.CommentID, commentID)
	}
}

func TestShowComment_NonExistent_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	// Given — an initialized service with no comments.
	svc, _ := setupService(t)

	// When — show a non-existent comment.
	_, err := svc.ShowComment(t.Context(), 99999)
	// Then — returns ErrNotFound.
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListComments ---

func TestListComments_ReturnsCommentsForIssue(t *testing.T) {
	t.Parallel()

	// Given — two issues, each with comments.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "list-agent")

	issue1, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Issue one", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue 1: %v", err)
	}
	issue2, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Issue two", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue 2: %v", err)
	}

	for _, body := range []string{"Comment A", "Comment B"} {
		_, err = svc.AddComment(ctx, driving.AddCommentInput{
			IssueID: issue1.Issue.ID().String(), Author: author, Body: body,
		})
		if err != nil {
			t.Fatalf("precondition: add comment to issue 1: %v", err)
		}
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issue2.Issue.ID().String(), Author: author, Body: "Comment C",
	})
	if err != nil {
		t.Fatalf("precondition: add comment to issue 2: %v", err)
	}

	// When — list comments for issue 1.
	output, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: issue1.Issue.ID().String(),
	})
	// Then — returns only issue 1's comments.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(output.Comments))
	}
}

func TestListComments_EmptyIssue_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	// Given — an issue with no comments.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "list-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "No comments", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	output, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: created.Issue.ID().String(),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(output.Comments))
	}
}

func TestListComments_LimitTruncates_SetsHasMore(t *testing.T) {
	t.Parallel()

	// Given — an issue with three comments.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "list-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Many comments", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	for _, body := range []string{"One", "Two", "Three"} {
		_, err = svc.AddComment(ctx, driving.AddCommentInput{
			IssueID: created.Issue.ID().String(), Author: author, Body: body,
		})
		if err != nil {
			t.Fatalf("precondition: add comment: %v", err)
		}
	}

	// When — list with limit 2.
	output, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: created.Issue.ID().String(),
		Limit:   2,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(output.Comments))
	}
	if !output.HasMore {
		t.Error("expected HasMore to be true")
	}
}

func TestListComments_FilterByAuthor_NarrowsResults(t *testing.T) {
	t.Parallel()

	// Given — an issue with comments from two authors.
	ctx := t.Context()
	svc, _ := setupService(t)
	alice := mustAuthor(t, "alice")
	bob := mustAuthor(t, "bob")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Multi-author", Author: alice,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: alice, Body: "Alice's comment",
	})
	if err != nil {
		t.Fatalf("precondition: add alice comment: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: bob, Body: "Bob's comment",
	})
	if err != nil {
		t.Fatalf("precondition: add bob comment: %v", err)
	}

	// When — list comments filtered to alice.
	output, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: created.Issue.ID().String(),
		Filter:  driving.CommentFilterInput{Authors: []string{alice}},
	})
	// Then — only alice's comment.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(output.Comments))
	}
	if output.Comments[0].Body != "Alice's comment" {
		t.Errorf("body: got %q, want %q", output.Comments[0].Body, "Alice's comment")
	}
}

// --- SearchComments ---

func TestSearchComments_PartialTextMatch_ReturnsMatchingComments(t *testing.T) {
	t.Parallel()

	// Given — two comments, only one containing "root cause".
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Search target", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: author, Body: "Found the root cause in auth.go",
	})
	if err != nil {
		t.Fatalf("precondition: add comment 1: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: author, Body: "Deployed the fix to staging",
	})
	if err != nil {
		t.Fatalf("precondition: add comment 2: %v", err)
	}

	// When — search for "root cause".
	output, err := svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query: "root cause",
	})
	// Then — one matching comment.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Comments))
	}
	if !strings.Contains(output.Comments[0].Body, "root cause") {
		t.Errorf("body should contain 'root cause', got %q", output.Comments[0].Body)
	}
}

func TestSearchComments_FilterByAuthor_NarrowsResults(t *testing.T) {
	t.Parallel()

	// Given — comments from two authors both containing "fix".
	ctx := t.Context()
	svc, _ := setupService(t)
	alice := mustAuthor(t, "alice")
	bob := mustAuthor(t, "bob")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Author search", Author: alice,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: alice, Body: "Applied the fix",
	})
	if err != nil {
		t.Fatalf("precondition: add alice comment: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: bob, Body: "Verified the fix",
	})
	if err != nil {
		t.Fatalf("precondition: add bob comment: %v", err)
	}

	// When — search for "fix" filtered to bob.
	output, err := svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query:  "fix",
		Filter: driving.CommentFilterInput{Authors: []string{bob}},
	})
	// Then — only bob's comment.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Comments))
	}
	if output.Comments[0].Body != "Verified the fix" {
		t.Errorf("body: got %q, want %q", output.Comments[0].Body, "Verified the fix")
	}
}

func TestSearchComments_ScopedToIssue_ExcludesOtherIssues(t *testing.T) {
	t.Parallel()

	// Given — two issues, each with a comment containing "deploy".
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "scope-agent")

	issue1, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Issue one", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue 1: %v", err)
	}
	issue2, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Issue two", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue 2: %v", err)
	}

	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issue1.Issue.ID().String(), Author: author, Body: "Deploy to staging",
	})
	if err != nil {
		t.Fatalf("precondition: add comment to issue 1: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issue2.Issue.ID().String(), Author: author, Body: "Deploy to production",
	})
	if err != nil {
		t.Fatalf("precondition: add comment to issue 2: %v", err)
	}

	// When — search for "deploy" scoped to issue 1.
	output, err := svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query:   "deploy",
		IssueID: issue1.Issue.ID().String(),
	})
	// Then — only issue 1's comment.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Comments))
	}
	if output.Comments[0].Body != "Deploy to staging" {
		t.Errorf("body: got %q, want %q", output.Comments[0].Body, "Deploy to staging")
	}
}

func TestSearchComments_NoMatch_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Given — a comment that does not match the query.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "No match target", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: created.Issue.ID().String(), Author: author, Body: "This is about logging",
	})
	if err != nil {
		t.Fatalf("precondition: add comment: %v", err)
	}

	// When — search for "xyzzy".
	output, err := svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query: "xyzzy",
	})
	// Then — empty results.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Comments) != 0 {
		t.Errorf("expected 0 results, got %d", len(output.Comments))
	}
}

// --- GetPrefix ---

func TestGetPrefix_AfterInit_ReturnsConfiguredPrefix(t *testing.T) {
	t.Parallel()

	// Given — a service initialized with prefix "NP".
	svc, _ := setupService(t)

	// When
	prefix, err := svc.GetPrefix(t.Context())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefix != "NP" {
		t.Errorf("prefix: got %q, want %q", prefix, "NP")
	}
}

// --- LookupClaimIssueID ---

func TestLookupClaimIssueID_ValidClaim_ReturnsIssueID(t *testing.T) {
	t.Parallel()

	// Given — a claimed domain.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "lookup-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Lookup target", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — look up the claim ID.
	issueID, err := svc.LookupClaimIssueID(ctx, created.ClaimID)
	// Then — returns the correct issue ID.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issueID != created.Issue.ID().String() {
		t.Errorf("issue ID: got %v, want %v", issueID, created.Issue.ID())
	}
}

func TestLookupClaimIssueID_UnknownClaim_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	// Given — an initialized service with no claims.
	svc, _ := setupService(t)

	// When — look up a non-existent claim ID.
	_, err := svc.LookupClaimIssueID(t.Context(), "nonexistent-claim-id")
	// Then — returns ErrNotFound.
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- LookupClaimAuthor ---

func TestLookupClaimAuthor_ValidClaim_ReturnsAuthor(t *testing.T) {
	t.Parallel()

	// Given — a claimed domain.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "claim-author-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Author lookup target", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — look up the claim author.
	got, err := svc.LookupClaimAuthor(ctx, created.ClaimID)
	// Then — returns the correct author.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != author {
		t.Errorf("author: got %q, want %q", got, author)
	}
}

func TestLookupClaimAuthor_UnknownClaim_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	// Given — an initialized service with no claims.
	svc, _ := setupService(t)

	// When — look up a non-existent claim ID.
	_, err := svc.LookupClaimAuthor(t.Context(), "nonexistent-claim-id")
	// Then — returns ErrNotFound.
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListDistinctLabels ---

func TestListDistinctLabels_NoLabels_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	// Given — a task with no labels.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "label-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "No labels", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	labels, err := svc.ListDistinctLabels(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected 0 labels, got %d", len(labels))
	}
}

func TestListDistinctLabels_OneLabel_ReturnsSingleLabel(t *testing.T) {
	t.Parallel()

	// Given — a task with one label.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "label-agent")

	kindBug := driving.LabelInput{Key: "kind", Value: "bug"}
	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Bug fix", Author: author,
		Labels: []driving.LabelInput{kindBug},
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	labels, err := svc.ListDistinctLabels(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
	if labels[0].Key != "kind" || labels[0].Value != "bug" {
		t.Errorf("label: got %s:%s, want kind:bug", labels[0].Key, labels[0].Value)
	}
}

func TestListDistinctLabels_MultipleIssues_ReturnsDistinctLabels(t *testing.T) {
	t.Parallel()

	// Given — two tasks sharing one label key with different values,
	// plus a unique label on each.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "label-agent")

	kindBug := driving.LabelInput{Key: "kind", Value: "bug"}
	areaAuth := driving.LabelInput{Key: "area", Value: "auth"}
	kindFeature := driving.LabelInput{Key: "kind", Value: "feature"}
	areaAPI := driving.LabelInput{Key: "area", Value: "api"}

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Bug in auth", Author: author,
		Labels: []driving.LabelInput{kindBug, areaAuth},
	})
	if err != nil {
		t.Fatalf("precondition: create issue 1: %v", err)
	}
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "New API feature", Author: author,
		Labels: []driving.LabelInput{kindFeature, areaAPI},
	})
	if err != nil {
		t.Fatalf("precondition: create issue 2: %v", err)
	}

	// When
	labels, err := svc.ListDistinctLabels(ctx)
	// Then — four distinct key:value pairs.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 4 {
		t.Fatalf("expected 4 distinct labels, got %d", len(labels))
	}

	// Verify all expected labels are present.
	found := make(map[string]bool)
	for _, l := range labels {
		found[l.Key+":"+l.Value] = true
	}
	for _, want := range []string{"kind:bug", "kind:feature", "area:auth", "area:api"} {
		if !found[want] {
			t.Errorf("expected label %q not found in results", want)
		}
	}
}

// --- SearchIssues ---

func TestSearchIssues_PartialTitleMatch_ReturnsMatchingIssues(t *testing.T) {
	t.Parallel()

	// Given — two tasks; only one title contains "retry".
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Implement retry logic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Add logging middleware", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — search for "retry".
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "retry",
	})
	// Then — exactly one result with the matching title.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Items))
	}
	if output.Items[0].Title != "Implement retry logic" {
		t.Errorf("title: got %q, want %q", output.Items[0].Title, "Implement retry logic")
	}
}

func TestSearchIssues_PartialDescriptionMatch_ReturnsMatchingIssues(t *testing.T) {
	t.Parallel()

	// Given — two tasks; only one description contains "timeout".
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Fix network issue",
		Description: "Requests fail with timeout after 30 seconds",
		Author:      author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Update dependencies",
		Description: "Bump all packages to latest versions",
		Author:      author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — search for "timeout".
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "timeout",
	})
	// Then — exactly one result matching on description.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Items))
	}
	if output.Items[0].Title != "Fix network issue" {
		t.Errorf("title: got %q, want %q", output.Items[0].Title, "Fix network issue")
	}
}

func TestSearchIssues_CaseInsensitive_MatchesRegardlessOfCase(t *testing.T) {
	t.Parallel()

	// Given — a task with an uppercase title.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "CRITICAL Bug in Auth Module", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — search with lowercase query.
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "critical bug",
	})
	// Then — matches despite case difference.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Items))
	}
}

func TestSearchIssues_WithRoleFilter_NarrowsResults(t *testing.T) {
	t.Parallel()

	// Given — a task and an epic both containing "auth" in the title.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Fix auth token expiry", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Auth system overhaul", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	// When — search for "auth" filtered to tasks only.
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query:  "auth",
		Filter: driving.IssueFilterInput{Roles: []domain.Role{domain.RoleTask}},
	})
	// Then — only the task matches.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Items))
	}
	if output.Items[0].Role != domain.RoleTask {
		t.Errorf("role: got %v, want %v", output.Items[0].Role, domain.RoleTask)
	}
}

func TestSearchIssues_WithStateFilter_NarrowsResults(t *testing.T) {
	t.Parallel()

	// Given — two tasks with "deploy" in the title; one is closed.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	openOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Deploy to staging", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Deploy to production", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create claimed task: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: closedOut.Issue.ID().String(),
		ClaimID: closedOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}
	_ = openOut

	// When — search for "deploy" filtered to closed state.
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query:  "deploy",
		Filter: driving.IssueFilterInput{States: []domain.State{domain.StateClosed}},
	})
	// Then — only the closed task matches.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Items))
	}
	if output.Items[0].Title != "Deploy to production" {
		t.Errorf("title: got %q, want %q", output.Items[0].Title, "Deploy to production")
	}
}

func TestSearchIssues_WithLabelFilter_NarrowsResults(t *testing.T) {
	t.Parallel()

	// Given — two tasks matching "logging"; only one has the kind:bug label.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	bugLabel := driving.LabelInput{Key: "kind", Value: "bug"}

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Fix logging format bug",
		Author: author, Labels: []driving.LabelInput{bugLabel},
	})
	if err != nil {
		t.Fatalf("precondition: create bug task: %v", err)
	}
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Add structured logging",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create feature task: %v", err)
	}

	// When — search for "logging" filtered to kind:bug.
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "logging",
		Filter: driving.IssueFilterInput{
			LabelFilters: []driving.LabelFilterInput{{Key: "kind", Value: "bug"}},
		},
	})
	// Then — only the bug task matches.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Items))
	}
	if output.Items[0].Title != "Fix logging format bug" {
		t.Errorf("title: got %q, want %q", output.Items[0].Title, "Fix logging format bug")
	}
}

func TestSearchIssues_NoMatch_ReturnsEmptyList(t *testing.T) {
	t.Parallel()

	// Given — two tasks, neither containing "xyzzy".
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Implement feature A", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Implement feature B", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — search for a term that matches nothing.
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "xyzzy",
	})
	// Then — empty results, no error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 0 {
		t.Errorf("expected 0 results, got %d", len(output.Items))
	}
	if output.HasMore {
		t.Error("expected HasMore to be false for empty results")
	}
}

func TestSearchIssues_LimitTruncatesResults_SetsHasMore(t *testing.T) {
	t.Parallel()

	// Given — three tasks all matching "widget".
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "search-agent")

	for _, title := range []string{"Widget alpha", "Widget beta", "Widget gamma"} {
		_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
			Role: domain.RoleTask, Title: title, Author: author,
		})
		if err != nil {
			t.Fatalf("precondition: create issue %q: %v", title, err)
		}
	}

	// When — search with limit of 2.
	output, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "widget",
		Limit: 2,
	})
	// Then — two items returned, HasMore is true.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 2 {
		t.Errorf("expected 2 results, got %d", len(output.Items))
	}
	if !output.HasMore {
		t.Error("expected HasMore to be true when results exceed limit")
	}
}

// findingByCategory returns the first finding with the given category, or nil.
func findingByCategory(findings []driving.DoctorFinding, category string) *driving.DoctorFinding {
	for i := range findings {
		if findings[i].Category == category {
			return &findings[i]
		}
	}
	return nil
}

// --- Natural Sort Order ---

func TestListIssues_NaturalSort_ChildrenClusterWithParent(t *testing.T) {
	t.Parallel()

	// Given — three issues at the same priority, created in this order:
	//   1. Epic (parent)
	//   2. Independent task (no parent)
	//   3. Child task of the epic
	// The family-anchored sort should place the child immediately after its
	// parent, pushing the independent task after the family cluster.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "sort-agent")

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleEpic,
		Title:    "Parent epic",
		Author:   author,
		Priority: domain.P2,
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}
	parentID := epicOut.Issue.ID()

	time.Sleep(time.Millisecond)

	independentOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Independent task",
		Author:   author,
		Priority: domain.P2,
	})
	if err != nil {
		t.Fatalf("precondition: create independent task failed: %v", err)
	}
	independentID := independentOut.Issue.ID()

	time.Sleep(time.Millisecond)

	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task",
		Author:   author,
		Priority: domain.P2,
		ParentID: parentID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child task failed: %v", err)
	}
	childID := childOut.Issue.ID()

	// When — list all issues with the default priority-based natural sort
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		OrderBy: driving.OrderByPriority,
	})
	// Then — the order should be: parent, child, independent.
	// The child's family anchor is the parent's created_at, which is earlier
	// than the independent task's created_at.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items: got %d, want 3", len(result.Items))
	}

	gotIDs := make([]string, len(result.Items))
	for i, item := range result.Items {
		gotIDs[i] = item.ID
	}

	wantIDs := []string{parentID.String(), childID.String(), independentID.String()}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("sort order mismatch:\n  got:  %v\n  want: %v", gotIDs, wantIDs)
	}
}

// --- ReopenIssue ---

func TestReopenIssue_ClosedIssue_RecordsReopenedEvent(t *testing.T) {
	t.Parallel()

	// Given — a closed task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Closed task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close issue: %v", err)
	}

	// When — reopen the closed domain.
	err = svc.ReopenIssue(ctx, driving.ReopenInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, err := svc.ShowIssue(ctx, created.Issue.ID().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.State != domain.StateOpen {
		t.Errorf("expected state open, got %v", show.State)
	}

	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("unexpected error fetching history: %v", err)
	}
	var found bool
	for _, e := range histOut.Entries {
		if e.EventType == history.EventReopened.String() {
			found = true
			changes := e.Changes
			var hasState bool
			for _, c := range changes {
				if c.Field == "state" && c.Before == "closed" && c.After == "open" {
					hasState = true
				}
			}
			if !hasState {
				t.Errorf("expected state change closed→open, got %v", changes)
			}
		}
	}
	if !found {
		t.Error("expected reopened history entry, none found")
	}
}

func TestReopenIssue_DeferredIssue_RecordsUndeferredEvent(t *testing.T) {
	t.Parallel()

	// Given — a deferred task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Deferred task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: created.Issue.ID().String(), ClaimID: created.ClaimID,
		Action: driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer issue: %v", err)
	}

	// When — reopen the deferred domain.
	err = svc.ReopenIssue(ctx, driving.ReopenInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, err := svc.ShowIssue(ctx, created.Issue.ID().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.State != domain.StateOpen {
		t.Errorf("expected state open, got %v", show.State)
	}

	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("unexpected error fetching history: %v", err)
	}
	var found bool
	for _, e := range histOut.Entries {
		if e.EventType == history.EventUndeferred.String() {
			found = true
			changes := e.Changes
			var hasState bool
			for _, c := range changes {
				if c.Field == "state" && c.Before == "deferred" && c.After == "open" {
					hasState = true
				}
			}
			if !hasState {
				t.Errorf("expected state change deferred→open, got %v", changes)
			}
		}
	}
	if !found {
		t.Error("expected undeferred history entry, none found")
	}
}

func TestReopenIssue_OpenIssue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — an open task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Open task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — try to reopen an already-open domain.
	err = svc.ReopenIssue(ctx, driving.ReopenInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
	})

	// Then — should fail.
	if err == nil {
		t.Fatal("expected error reopening an open issue, got nil")
	}
}

func TestReopenIssue_ClaimedIssue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Claimed task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}
	_ = created

	// When — try to reopen a claimed domain.
	err = svc.ReopenIssue(ctx, driving.ReopenInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
	})

	// Then — should fail.
	if err == nil {
		t.Fatal("expected error reopening a claimed issue, got nil")
	}
}

// --- DeferIssue ---

func TestDeferIssue_WithoutUntil_TransitionsToDeferred(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task to defer",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When: deferring without an until value.
	err = svc.DeferIssue(t.Context(), driving.DeferIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
	})
	// Then: issue is deferred and has no defer-until label.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, showErr := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if showErr != nil {
		t.Fatalf("show issue: %v", showErr)
	}
	if show.State != domain.StateDeferred {
		t.Errorf("state: got %v, want %v", show.State, domain.StateDeferred)
	}
	if _, ok := show.Labels["defer-until"]; ok {
		t.Error("defer-until label should not be set when Until is empty")
	}
}

func TestDeferIssue_WithUntil_SetsLabelAndTransitions(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Defer with date",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When: deferring with an until value.
	err = svc.DeferIssue(t.Context(), driving.DeferIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Until:   "2026-04-15",
	})
	// Then: issue is deferred and has the defer-until label.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	show, showErr := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if showErr != nil {
		t.Fatalf("show issue: %v", showErr)
	}
	if show.State != domain.StateDeferred {
		t.Errorf("state: got %v, want %v", show.State, domain.StateDeferred)
	}
	val, ok := show.Labels["defer-until"]
	if !ok {
		t.Fatal("expected defer-until label to be present")
	}
	if val != "2026-04-15" {
		t.Errorf("defer-until: got %q, want %q", val, "2026-04-15")
	}
}

func TestDeferIssue_InvalidClaim_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with a bogus claim ID.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Wrong claim",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When: deferring with the wrong claim ID.
	err = svc.DeferIssue(t.Context(), driving.DeferIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: "bogus-claim",
		Until:   "2026-04-15",
	})

	// Then: returns an error and the issue is unchanged.
	if err == nil {
		t.Fatal("expected error for invalid claim ID")
	}
	show, showErr := svc.ShowIssue(t.Context(), created.Issue.ID().String())
	if showErr != nil {
		t.Fatalf("show issue: %v", showErr)
	}
	if show.State != domain.StateOpen {
		t.Errorf("state should remain open (claimed), got %v", show.State)
	}
}

func TestDeferIssue_InvalidUntilValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with an invalid until value (label validation
	// rejects values that are too long or contain invalid characters).
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Invalid until",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When: deferring with a value that fails label validation.
	err = svc.DeferIssue(t.Context(), driving.DeferIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Until:   strings.Repeat("x", 500),
	})

	// Then: returns an error.
	if err == nil {
		t.Fatal("expected error for invalid until value")
	}
}

func TestDeferIssue_WithUntil_RecordsLabelHistory(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "History check",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When: deferring with an until value.
	err = svc.DeferIssue(t.Context(), driving.DeferIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Until:   "2026-05-01",
	})
	// Then: history contains both a label-added event and a state-changed event.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	histOut, histErr := svc.ShowHistory(t.Context(), driving.ListHistoryInput{
		IssueID: created.Issue.ID().String(),
		Limit:   50,
	})
	if histErr != nil {
		t.Fatalf("show history: %v", histErr)
	}

	hasLabelEvent := slices.ContainsFunc(histOut.Entries, func(e driving.HistoryEntryDTO) bool {
		return e.EventType == history.EventLabelAdded.String()
	})
	hasStateEvent := slices.ContainsFunc(histOut.Entries, func(e driving.HistoryEntryDTO) bool {
		return e.EventType == history.EventStateChanged.String()
	})
	if !hasLabelEvent {
		t.Error("expected a label-added history event")
	}
	if !hasStateEvent {
		t.Error("expected a state-changed history event")
	}
}

// --- GetIssueSummary ---

func TestGetIssueSummary_EmptyDatabase_ReturnsZeros(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	summary, err := svc.GetIssueSummary(t.Context())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Total != 0 {
		t.Errorf("total: got %d, want 0", summary.Total)
	}
}

func TestGetIssueSummary_CountsByState(t *testing.T) {
	t.Parallel()

	// Given — create issues in different states.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	// 2 open tasks.
	for _, title := range []string{"Open A", "Open B"} {
		_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
			Role: domain.RoleTask, Title: title, Author: author,
		})
		if err != nil {
			t.Fatalf("precondition: create open task failed: %v", err)
		}
	}

	// 1 claimed task.
	claimed, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Claimed", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create claimed task failed: %v", err)
	}

	// 1 closed task.
	closable, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "To close", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create closable task failed: %v", err)
	}
	err = svc.CloseWithReason(ctx, driving.CloseWithReasonInput{
		IssueID: closable.Issue.ID().String(), ClaimID: closable.ClaimID, Reason: "Done.",
	})
	if err != nil {
		t.Fatalf("precondition: close task failed: %v", err)
	}

	// 1 deferred task.
	deferrable, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "To defer", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create deferrable task failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: deferrable.Issue.ID().String(), ClaimID: deferrable.ClaimID, Action: driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer task failed: %v", err)
	}

	// When
	summary, err := svc.GetIssueSummary(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Claimed is now a secondary state of open; the claimed issue's primary
	// state remains open, so Open counts both unclaimed and claimed open issues.
	if summary.Open != 3 {
		t.Errorf("open: got %d, want 3 (2 unclaimed + 1 claimed)", summary.Open)
	}
	if summary.Closed != 1 {
		t.Errorf("closed: got %d, want 1", summary.Closed)
	}
	if summary.Deferred != 1 {
		t.Errorf("deferred: got %d, want 1", summary.Deferred)
	}
	if summary.Total != 5 {
		t.Errorf("total: got %d, want 5", summary.Total)
	}

	// The 2 unclaimed open tasks should be ready; the claimed task is not
	// ready because it has an active non-stale claim.
	if summary.Ready != 2 {
		t.Errorf("ready: got %d, want 2", summary.Ready)
	}

	// No blocked issues.
	if summary.Blocked != 0 {
		t.Errorf("blocked: got %d, want 0", summary.Blocked)
	}

	// Verify the claimed task variable is consumed.
	_ = claimed
}

func TestGetIssueSummary_BlockedIssue_CountedInBlocked(t *testing.T) {
	t.Parallel()

	// Given — a task blocked by another open task.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	blocker, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker failed: %v", err)
	}

	blocked, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked failed: %v", err)
	}

	err = svc.AddRelationship(ctx, blocked.Issue.ID().String(), driving.RelationshipInput{
		Type: domain.RelBlockedBy, TargetID: blocker.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	// When
	summary, err := svc.GetIssueSummary(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Ready != 1 {
		t.Errorf("ready: got %d, want 1 (only the blocker is ready)", summary.Ready)
	}
	if summary.Blocked != 1 {
		t.Errorf("blocked: got %d, want 1", summary.Blocked)
	}
}

// --- CloseCompletedEpics ---

func TestCloseCompletedEpics_IncludeTasks_ClosesParentTaskWithAllChildrenClosed(t *testing.T) {
	t.Parallel()

	// Given — a parent task with two closed children.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	parentOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Parent task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create parent: %v", err)
	}
	parentID := parentOut.Issue.ID()

	for _, title := range []string{"Child A", "Child B"} {
		childOut, createErr := svc.CreateIssue(ctx, driving.CreateIssueInput{
			Role: domain.RoleTask, Title: title, Author: author,
			ParentID: parentID.String(), Claim: true,
		})
		if createErr != nil {
			t.Fatalf("precondition: create child: %v", createErr)
		}
		if closeErr := svc.TransitionState(ctx, driving.TransitionInput{
			IssueID: childOut.Issue.ID().String(),
			ClaimID: childOut.ClaimID,
			Action:  driving.ActionClose,
		}); closeErr != nil {
			t.Fatalf("precondition: close child: %v", closeErr)
		}
	}

	// When — close completed with IncludeTasks.
	out, err := svc.CloseCompletedEpics(ctx, driving.CloseCompletedEpicsInput{
		Author:       author,
		IncludeTasks: true,
	})
	// Then — the parent task should be closed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results: got %d, want 1", len(out.Results))
	}
	if out.Results[0].ID != parentID.String() {
		t.Errorf("ID: got %s, want %s", out.Results[0].ID, parentID)
	}
	if !out.Results[0].Closed {
		t.Error("expected parent task to be closed")
	}
}

func TestCloseCompletedEpics_WithoutIncludeTasks_IgnoresParentTasks(t *testing.T) {
	t.Parallel()

	// Given — a parent task with all children closed (same setup as above).
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	parentOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Parent task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create parent: %v", err)
	}
	parentID := parentOut.Issue.ID()

	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child", Author: author,
		ParentID: parentID.String(), Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}
	if err := svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childOut.Issue.ID().String(),
		ClaimID: childOut.ClaimID,
		Action:  driving.ActionClose,
	}); err != nil {
		t.Fatalf("precondition: close child: %v", err)
	}

	// When — close completed WITHOUT IncludeTasks.
	out, err := svc.CloseCompletedEpics(ctx, driving.CloseCompletedEpicsInput{
		Author: author,
	})
	// Then — no results because parent tasks are excluded by default.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 0 {
		t.Errorf("results: got %d, want 0 (tasks should be excluded without IncludeTasks)", len(out.Results))
	}
}

func TestCloseCompletedEpics_IncludeTasks_IgnoresChildlessTasks(t *testing.T) {
	t.Parallel()

	// Given — a task with no children.
	svc, _ := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "alice")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Childless task", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When — close completed with IncludeTasks.
	out, err := svc.CloseCompletedEpics(ctx, driving.CloseCompletedEpicsInput{
		Author:       author,
		IncludeTasks: true,
	})
	// Then — no results because childless tasks are excluded.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 0 {
		t.Errorf("results: got %d, want 0 (childless tasks should be excluded)", len(out.Results))
	}
}

// --- ShowIssue: self-referential blocked_by regression ---

func TestShowIssue_BlockerTarget_NoSelfReferentialBlockerDetails(t *testing.T) {
	t.Parallel()

	// Given — task A is blocked_by task B. We will show task B.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "self-ref-agent")

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(),
		driving.RelationshipInput{Type: domain.RelBlockedBy, TargetID: blockerOut.Issue.ID().String()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When — show the blocker (the TARGET of the blocked_by relationship).
	showOut, err := svc.ShowIssue(ctx, blockerOut.Issue.ID().String())
	// Then — BlockerDetails must be empty; the blocker is not blocked by anything.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(showOut.BlockerDetails) != 0 {
		t.Errorf("BlockerDetails: got %d entries, want 0 (blocker should not appear blocked by itself)", len(showOut.BlockerDetails))
		for _, d := range showOut.BlockerDetails {
			t.Logf("  spurious blocker detail: ID=%s Title=%s", d.ID, d.Title)
		}
	}
}

func TestShowIssue_BlockedIssue_HasCorrectBlockerDetails(t *testing.T) {
	t.Parallel()

	// Given — task A is blocked_by task B. We will show task A.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "blocked-detail-agent")

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "The Blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blockedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "The Blocked", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked: %v", err)
	}

	err = svc.AddRelationship(ctx, blockedOut.Issue.ID().String(),
		driving.RelationshipInput{Type: domain.RelBlockedBy, TargetID: blockerOut.Issue.ID().String()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When — show the blocked domain.
	showOut, err := svc.ShowIssue(ctx, blockedOut.Issue.ID().String())
	// Then — BlockerDetails must contain exactly the blocker.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(showOut.BlockerDetails) != 1 {
		t.Fatalf("BlockerDetails: got %d entries, want 1", len(showOut.BlockerDetails))
	}
	if showOut.BlockerDetails[0].ID != blockerOut.Issue.ID().String() {
		t.Errorf("BlockerDetails[0].ID: got %s, want %s", showOut.BlockerDetails[0].ID, blockerOut.Issue.ID().String())
	}
	if showOut.BlockerDetails[0].Title != "The Blocker" {
		t.Errorf("BlockerDetails[0].Title: got %q, want %q", showOut.BlockerDetails[0].Title, "The Blocker")
	}
}

func TestShowIssue_InheritedBlocking_NoSelfReferentialBlockerIDs(t *testing.T) {
	t.Parallel()

	// Given — epic P is blocked_by task B. Another task X is blocked_by P
	// (making P the TARGET of a blocked_by). Task C is a child of P.
	// When we show C, the inherited blocking section should list only B as
	// the blocker — not P itself (which would appear if the code fails to
	// filter out relationships where P is the target rather than source).
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "inherited-agent")

	blockerOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "External blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Blocked epic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	err = svc.AddRelationship(ctx, epicOut.Issue.ID().String(),
		driving.RelationshipInput{Type: domain.RelBlockedBy, TargetID: blockerOut.Issue.ID().String()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by epic->blocker: %v", err)
	}

	// Create a third issue that is blocked_by the epic, making the epic
	// the TARGET of a blocked_by relationship.
	otherOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Other task blocked by epic", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create other task: %v", err)
	}

	err = svc.AddRelationship(ctx, otherOut.Issue.ID().String(),
		driving.RelationshipInput{Type: domain.RelBlockedBy, TargetID: epicOut.Issue.ID().String()}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by other->epic: %v", err)
	}

	childOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child of blocked epic", Author: author,
		ParentID: epicOut.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child: %v", err)
	}

	// When — show the child task.
	showOut, err := svc.ShowIssue(ctx, childOut.Issue.ID().String())
	// Then — InheritedBlocking should reference the epic as ancestor and
	// the external blocker as the blocker — not the epic itself.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showOut.InheritedBlocking == nil {
		t.Fatal("expected InheritedBlocking to be present")
	}
	if showOut.InheritedBlocking.AncestorID != epicOut.Issue.ID().String() {
		t.Errorf("AncestorID: got %s, want %s", showOut.InheritedBlocking.AncestorID, epicOut.Issue.ID().String())
	}

	blockerIDs := showOut.InheritedBlocking.BlockerIDs
	epicIDStr := epicOut.Issue.ID().String()
	blockerIDStr := blockerOut.Issue.ID().String()

	for _, id := range blockerIDs {
		if id == epicIDStr {
			t.Errorf("InheritedBlocking.BlockerIDs contains the ancestor epic %s — self-referential", epicIDStr)
		}
	}
	if !slices.Contains(blockerIDs, blockerIDStr) {
		t.Errorf("InheritedBlocking.BlockerIDs missing expected blocker %s; got %v", blockerIDStr, blockerIDs)
	}
}

// --- Edge cases: claim precedence, expired claims, concurrent claims, and deletion ---

// TestShowIssue_ClaimedAndBlocked_ClaimedTakesPrecedence verifies that an open
// issue with an active claim and an unresolved blocker displays as open
// (claimed), not open (blocked). The claimed secondary state takes precedence
// over blocked in the display model.
func TestShowIssue_ClaimedAndBlocked_ClaimedTakesPrecedence(t *testing.T) {
	t.Parallel()

	// Given — a task that is both actively claimed AND blocked by another open task.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	blocker, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Open blocker", Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	// Create the claimed issue and also add it as blocked_by the blocker.
	claimedAndBlocked, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Claimed and blocked task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create claimed issue: %v", err)
	}

	err = svc.AddRelationship(ctx, claimedAndBlocked.Issue.ID().String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: blocker.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When
	show, err := svc.ShowIssue(ctx, claimedAndBlocked.Issue.ID().String())
	// Then — primary state is open and secondary state is claimed (not blocked).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.State != domain.StateOpen {
		t.Errorf("state: got %v, want open", show.State)
	}
	if show.SecondaryState != domain.SecondaryClaimed {
		t.Errorf("secondary state: got %v, want claimed", show.SecondaryState)
	}
}

// TestCloseWithReason_ExpiredClaim_Fails verifies that close fails with a clear
// error when the claim used as authorization has gone stale. Callers must
// re-claim the issue before retrying.
func TestCloseWithReason_ExpiredClaim_Fails(t *testing.T) {
	t.Parallel()

	// Given — a task claimed with a 1 ns stale threshold so it expires immediately.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task to close",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID:        created.Issue.ID().String(),
		Author:         author,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim issue: %v", err)
	}

	// Let the claim go stale.
	time.Sleep(2 * time.Millisecond)

	// When — attempt to close using the stale claim.
	closeErr := svc.CloseWithReason(ctx, driving.CloseWithReasonInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: claimOut.ClaimID,
		Reason:  "done",
	})

	// Then — must fail with ErrStaleClaim so the caller knows to re-claim.
	if !errors.Is(closeErr, domain.ErrStaleClaim) {
		t.Errorf("expected ErrStaleClaim, got %v", closeErr)
	}
}

// TestUpdateIssue_ExpiredClaim_Fails verifies that json update fails with a
// clear error when the claim has gone stale.
func TestUpdateIssue_ExpiredClaim_Fails(t *testing.T) {
	t.Parallel()

	// Given — a task claimed with a 1 ns stale threshold so it expires immediately.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task to update",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID:        created.Issue.ID().String(),
		Author:         author,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim issue: %v", err)
	}

	// Let the claim go stale.
	time.Sleep(2 * time.Millisecond)

	// When — attempt to update using the stale claim.
	revisedTitle := "Revised title"
	updateErr := svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: claimOut.ClaimID,
		Title:   &revisedTitle,
	})

	// Then — must fail with ErrStaleClaim so the caller knows to re-claim.
	if !errors.Is(updateErr, domain.ErrStaleClaim) {
		t.Errorf("expected ErrStaleClaim, got %v", updateErr)
	}
}

// TestClaimByID_ClosedIssue_Fails verifies that closed issues cannot be claimed.
// Reopen is a claim-free operation; there is no need to claim a closed issue.
func TestClaimByID_ClosedIssue_Fails(t *testing.T) {
	t.Parallel()

	// Given — a task that has been closed.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Closed task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	err = svc.CloseWithReason(ctx, driving.CloseWithReasonInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
		Reason:  "done",
	})
	if err != nil {
		t.Fatalf("precondition: close issue: %v", err)
	}

	// When — try to claim the closed issue.
	bob := mustAuthor(t, "bob")
	_, claimErr := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: created.Issue.ID().String(),
		Author:  bob,
	})

	// Then
	if !errors.Is(claimErr, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition for closed issue, got %v", claimErr)
	}
}

// TestClaimByID_DeferredIssue_Fails verifies that deferred issues cannot be
// claimed. Undefer is a claim-free operation.
func TestClaimByID_DeferredIssue_Fails(t *testing.T) {
	t.Parallel()

	// Given — a task that has been deferred.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task to defer", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	err = svc.DeferIssue(ctx, driving.DeferIssueInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: defer issue: %v", err)
	}

	// When — try to claim the deferred issue.
	bob := mustAuthor(t, "bob")
	_, claimErr := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: created.Issue.ID().String(),
		Author:  bob,
	})

	// Then
	if !errors.Is(claimErr, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition for deferred issue, got %v", claimErr)
	}
}

// TestClaimByID_SelfReclaimActiveClaim_Fails verifies that the same author
// cannot re-claim an issue they already hold an active claim on. There is no
// special bypass for the current claim holder; callers must use
// ExtendStaleThreshold or wait for the claim to expire.
func TestClaimByID_SelfReclaimActiveClaim_Fails(t *testing.T) {
	t.Parallel()

	// Given — a task actively claimed by alice.
	ctx := t.Context()
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Active task", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — alice tries to claim again (self-re-claim).
	_, claimErr := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: created.Issue.ID().String(),
		Author:  author,
	})

	// Then — conflict even for the same author.
	if !errors.Is(claimErr, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError for self-re-claim, got %v", claimErr)
	}
}

// TestDeleteIssue_WithActiveClaim_DeletesClaimAssSideEffect verifies that
// deleting an issue with an active claim removes the claim row, preventing
// orphaned claim records that would pollute the claims table.
func TestDeleteIssue_WithActiveClaim_DeletesClaimAsSideEffect(t *testing.T) {
	t.Parallel()

	// Given — a task with an active claim.
	ctx := t.Context()
	svc, repo := setupService(t)
	author := mustAuthor(t, "alice")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task to delete", Author: author, Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// Confirm the claim exists before deletion.
	issueID, parseErr := domain.ParseID(created.Issue.ID().String())
	if parseErr != nil {
		t.Fatalf("precondition: parse issue ID: %v", parseErr)
	}
	if _, claimErr := repo.GetClaimByIssue(ctx, issueID); claimErr != nil {
		t.Fatalf("precondition: expected active claim before deletion, got err: %v", claimErr)
	}

	// When
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: created.Issue.ID().String(),
		ClaimID: created.ClaimID,
	})
	// Then — delete succeeds.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the claim row has been removed as a side effect.
	_, claimAfterErr := repo.GetClaimByIssue(ctx, issueID)
	if !errors.Is(claimAfterErr, domain.ErrNotFound) {
		t.Errorf("expected claim to be deleted as side effect of issue deletion, got %v", claimAfterErr)
	}
}

// TestConcurrentClaims_ExactlyOneSucceeds simulates two sequential claims
// against the same issue and verifies that only the first succeeds. True
// concurrent access is enforced at the database layer; this test verifies the
// service-level conflict detection logic that backs it.
func TestConcurrentClaims_ExactlyOneSucceeds(t *testing.T) {
	t.Parallel()

	// Given — a single ready task.
	ctx := t.Context()
	svc, _ := setupService(t)

	alice := mustAuthor(t, "alice")
	bob := mustAuthor(t, "bob")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Contested task",
		Author: alice,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — alice claims first, then bob tries to claim the same issue.
	_, err = svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: created.Issue.ID().String(),
		Author:  alice,
	})
	if err != nil {
		t.Fatalf("first claim (alice): unexpected error: %v", err)
	}

	_, bobErr := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: created.Issue.ID().String(),
		Author:  bob,
	})

	// Then — bob's claim fails with ClaimConflictError because alice holds an
	// active claim. Exactly one caller wins the claim race.
	if !errors.Is(bobErr, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError for concurrent claim attempt, got %v", bobErr)
	}
}

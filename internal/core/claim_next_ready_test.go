package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- ClaimNextReady ---

func TestClaimNextReady_SingleReadyIssue_ClaimsIt(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Only task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ClaimID == "" {
		t.Error("expected non-empty claim ID")
	}
	if output.IssueID != created.Issue.ID().String() {
		t.Errorf("expected issue %s, got %s", created.Issue.ID(), output.IssueID)
	}
}

func TestClaimNextReady_MultipleReadyIssues_ClaimsHighestPriority(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Low priority task",
		Author:   author,
		Priority: domain.P3,
	})
	if err != nil {
		t.Fatalf("precondition: create P3 issue: %v", err)
	}

	highPri, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "High priority task",
		Author:   author,
		Priority: domain.P1,
	})
	if err != nil {
		t.Fatalf("precondition: create P1 issue: %v", err)
	}

	_, err = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Medium priority task",
		Author:   author,
		Priority: domain.P2,
	})
	if err != nil {
		t.Fatalf("precondition: create P2 issue: %v", err)
	}

	// When
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != highPri.Issue.ID().String() {
		t.Errorf("expected highest-priority issue %s, got %s", highPri.Issue.ID(), output.IssueID)
	}
}

func TestClaimNextReady_NoReadyIssues_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	// Given — empty database, no issues at all.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	// When
	_, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: author,
	})

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestClaimNextReady_AllIssuesClaimed_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	// Given — one issue exists but is already claimed with a non-stale claim.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Already claimed",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create claimed issue: %v", err)
	}

	// When
	bob := mustAuthor(t, "bob")
	_, err = svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: bob,
	})

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestClaimNextReady_StaleClaim_TreatedAsReady(t *testing.T) {
	t.Parallel()

	// Given — one issue claimed with a very short stale threshold so it's
	// immediately stale. A stale claim is treated as nonexistent, so the
	// issue should appear ready again.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Stale task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// Claim with a 1-nanosecond threshold so it goes stale immediately.
	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        created.Issue.ID().String(),
		Author:         author,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim issue: %v", err)
	}

	// Let the claim go stale.
	time.Sleep(2 * time.Millisecond)

	// When — a second author claims; the stale claim is overwritten.
	bob := mustAuthor(t, "bob")
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: bob,
	})
	// Then — claim succeeds because stale claims are treated as nonexistent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != created.Issue.ID().String() {
		t.Errorf("expected issue %s, got %s", created.Issue.ID(), output.IssueID)
	}
}

func TestClaimNextReady_FilterByRole_ClaimsMatchingRole(t *testing.T) {
	t.Parallel()

	// Given — one task and one epic, both ready. Filter for tasks only.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Epic without children",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	task, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "A task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// When
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: author,
		Role:   domain.RoleTask,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != task.Issue.ID().String() {
		t.Errorf("expected task %s, got %s", task.Issue.ID(), output.IssueID)
	}
}

func TestClaimNextReady_FilterByLabel_ClaimsMatchingLabel(t *testing.T) {
	t.Parallel()

	// Given — two tasks, one with label kind:fix, one without.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	labeled, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Fix task",
		Author: author,
		Labels: []driving.LabelInput{{Key: "kind", Value: "fix"}},
	})
	if err != nil {
		t.Fatalf("precondition: create labeled issue: %v", err)
	}

	_, err = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Feature task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create unlabeled issue: %v", err)
	}

	// When
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: author,
		LabelFilters: []driving.LabelFilterInput{
			{Key: "kind", Value: "fix"},
		},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != labeled.Issue.ID().String() {
		t.Errorf("expected labeled issue %s, got %s", labeled.Issue.ID(), output.IssueID)
	}
}

func TestClaimNextReady_DeferredAncestor_BlocksReadiness(t *testing.T) {
	t.Parallel()

	// Given — an epic with a child task. The epic is deferred, so the child
	// should not appear as ready.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	epic, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Deferred epic",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	_, err = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task under deferred epic",
		Author:   author,
		ParentID: epic.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child task: %v", err)
	}

	// Defer the epic.
	err = svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: epic.Issue.ID().String(),
		ClaimID: epic.ClaimID,
		Action:  driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer epic: %v", err)
	}

	// When — attempt to claim next ready; the child task should not be ready.
	_, err = svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: author,
	})

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound (child under deferred epic is not ready), got %v", err)
	}
}

func TestClaimNextReady_BlockedIssue_NotReady(t *testing.T) {
	t.Parallel()

	// Given — a task blocked by another open task. The blocked task should not
	// appear as ready.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	blocker, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Blocker task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocker: %v", err)
	}

	blocked, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Blocked task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create blocked issue: %v", err)
	}

	// Add blocked_by relationship.
	err = svc.AddRelationship(t.Context(), blocked.Issue.ID().String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: blocker.Issue.ID().String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by: %v", err)
	}

	// When — claim next ready; only the blocker should be claimable.
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != blocker.Issue.ID().String() {
		t.Errorf("expected unblocked issue %s, got %s", blocker.Issue.ID(), output.IssueID)
	}
}

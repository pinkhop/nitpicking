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
	if output.Stolen {
		t.Error("expected Stolen to be false")
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

	// Given — one issue exists but is already claimed.
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

func TestClaimNextReady_StealIfNeeded_StealsStaleClaimedIssue(t *testing.T) {
	t.Parallel()

	// Given — one issue claimed with a very short stale threshold so it's
	// immediately stale, and no ready issues.
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

	// When
	bob := mustAuthor(t, "bob")
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author:        bob,
		StealIfNeeded: true,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != created.Issue.ID().String() {
		t.Errorf("expected stolen issue %s, got %s", created.Issue.ID(), output.IssueID)
	}
	if !output.Stolen {
		t.Error("expected Stolen to be true")
	}
}

func TestClaimNextReady_StealIfNeeded_NoStaleClaims_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	// Given — one issue claimed with a long threshold (not stale).
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Active claim",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create claimed issue: %v", err)
	}

	// When — steal requested, but no stale claims exist.
	bob := mustAuthor(t, "bob")
	_, err = svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author:        bob,
		StealIfNeeded: true,
	})

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
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

func TestClaimNextReady_StealIfNeeded_FilterByRole_StealsMatchingRole(t *testing.T) {
	t.Parallel()

	// Given — two stale-claimed issues: one epic, one task. Steal with role
	// filter for tasks should only steal the task.
	svc, _ := setupService(t)
	alice := mustAuthor(t, "alice")

	epic, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Stale epic",
		Author: alice,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	task, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Stale task",
		Author: alice,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}

	// Claim both with 1ns threshold so they go stale immediately.
	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        epic.Issue.ID().String(),
		Author:         alice,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim epic: %v", err)
	}

	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        task.Issue.ID().String(),
		Author:         alice,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim task: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// When — steal with role filter for tasks.
	bob := mustAuthor(t, "bob")
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author:        bob,
		Role:          domain.RoleTask,
		StealIfNeeded: true,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != task.Issue.ID().String() {
		t.Errorf("expected task %s, got %s", task.Issue.ID(), output.IssueID)
	}
	if !output.Stolen {
		t.Error("expected Stolen to be true")
	}
}

func TestClaimNextReady_StealIfNeeded_FilterByLabel_StealsMatchingLabel(t *testing.T) {
	t.Parallel()

	// Given — two stale-claimed tasks: one with label kind:fix, one without.
	// Steal with label filter should only steal the labeled one.
	svc, _ := setupService(t)
	alice := mustAuthor(t, "alice")

	labeled, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Fix task",
		Author: alice,
		Labels: []driving.LabelInput{{Key: "kind", Value: "fix"}},
	})
	if err != nil {
		t.Fatalf("precondition: create labeled issue: %v", err)
	}

	unlabeled, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Feature task",
		Author: alice,
	})
	if err != nil {
		t.Fatalf("precondition: create unlabeled issue: %v", err)
	}

	// Claim both with 1ns threshold so they go stale immediately.
	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        labeled.Issue.ID().String(),
		Author:         alice,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim labeled: %v", err)
	}

	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        unlabeled.Issue.ID().String(),
		Author:         alice,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim unlabeled: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// When — steal with label filter for kind:fix.
	bob := mustAuthor(t, "bob")
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: bob,
		LabelFilters: []driving.LabelFilterInput{
			{Key: "kind", Value: "fix"},
		},
		StealIfNeeded: true,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != labeled.Issue.ID().String() {
		t.Errorf("expected labeled issue %s, got %s", labeled.Issue.ID(), output.IssueID)
	}
	if !output.Stolen {
		t.Error("expected Stolen to be true")
	}
}

func TestClaimNextReady_StealIfNeeded_FilterByRole_NoMatch_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	// Given — one stale-claimed epic. Steal with role filter for tasks should
	// return not found.
	svc, _ := setupService(t)
	alice := mustAuthor(t, "alice")

	epic, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Stale epic",
		Author: alice,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}

	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        epic.Issue.ID().String(),
		Author:         alice,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim epic: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// When — steal with role filter for tasks (no matching stale claims).
	bob := mustAuthor(t, "bob")
	_, err = svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author:        bob,
		Role:          domain.RoleTask,
		StealIfNeeded: true,
	})

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestClaimNextReady_StealIfNeeded_WithFilters_StealsHighestPriority(t *testing.T) {
	t.Parallel()

	// Given — two stale-claimed tasks with label kind:fix but different
	// priorities. Steal should pick the highest priority.
	svc, _ := setupService(t)
	alice := mustAuthor(t, "alice")

	lowPri, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Low priority fix",
		Author:   alice,
		Priority: domain.P3,
		Labels:   []driving.LabelInput{{Key: "kind", Value: "fix"}},
	})
	if err != nil {
		t.Fatalf("precondition: create P3 issue: %v", err)
	}

	highPri, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "High priority fix",
		Author:   alice,
		Priority: domain.P1,
		Labels:   []driving.LabelInput{{Key: "kind", Value: "fix"}},
	})
	if err != nil {
		t.Fatalf("precondition: create P1 issue: %v", err)
	}

	// Claim both with 1ns threshold.
	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        lowPri.Issue.ID().String(),
		Author:         alice,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim low: %v", err)
	}

	_, err = svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID:        highPri.Issue.ID().String(),
		Author:         alice,
		StaleThreshold: 1 * time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("precondition: claim high: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// When — steal with label filter; should pick highest priority.
	bob := mustAuthor(t, "bob")
	output, err := svc.ClaimNextReady(t.Context(), driving.ClaimNextReadyInput{
		Author: bob,
		LabelFilters: []driving.LabelFilterInput{
			{Key: "kind", Value: "fix"},
		},
		StealIfNeeded: true,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IssueID != highPri.Issue.ID().String() {
		t.Errorf("expected highest-priority issue %s, got %s", highPri.Issue.ID(), output.IssueID)
	}
	if !output.Stolen {
		t.Error("expected Stolen to be true")
	}
}

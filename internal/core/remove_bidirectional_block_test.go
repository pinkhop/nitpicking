package core_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func TestRemoveBidirectionalBlock_ForwardDirection_RemovesBlock(t *testing.T) {
	t.Parallel()

	// Given — A is blocked_by B.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	taskA := createStandaloneTask(t, svc, "Task A", author)
	taskB := createStandaloneTask(t, svc, "Task B", author)

	err := svc.AddRelationship(t.Context(), taskA.String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: taskB.String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship: %v", err)
	}

	// When — removing bidirectional block.
	err = svc.RemoveBidirectionalBlock(t.Context(), taskA.String(), taskB.String(), author)
	// Then — no error, relationship removed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, showErr := svc.ShowIssue(t.Context(), taskA.String())
	if showErr != nil {
		t.Fatalf("show: %v", showErr)
	}
	for _, r := range shown.Relationships {
		if r.Type == domain.RelBlockedBy.String() && r.TargetID == taskB.String() {
			t.Error("blocked_by relationship still exists after removal")
		}
	}
}

func TestRemoveBidirectionalBlock_ReverseDirection_RemovesBlock(t *testing.T) {
	t.Parallel()

	// Given — B is blocked_by A (reverse from the call order).
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	taskA := createStandaloneTask(t, svc, "Task A", author)
	taskB := createStandaloneTask(t, svc, "Task B", author)

	err := svc.AddRelationship(t.Context(), taskB.String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: taskA.String(),
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship: %v", err)
	}

	// When — removing bidirectional block with A first.
	err = svc.RemoveBidirectionalBlock(t.Context(), taskA.String(), taskB.String(), author)
	// Then — no error, relationship removed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, showErr := svc.ShowIssue(t.Context(), taskB.String())
	if showErr != nil {
		t.Fatalf("show: %v", showErr)
	}
	for _, r := range shown.Relationships {
		if r.Type == domain.RelBlockedBy.String() && r.TargetID == taskA.String() {
			t.Error("blocked_by relationship still exists after removal")
		}
	}
}

func TestRemoveBidirectionalBlock_NoRelationship_SucceedsIdempotently(t *testing.T) {
	t.Parallel()

	// Given — no blocking relationship between A and B.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	taskA := createStandaloneTask(t, svc, "Task A", author)
	taskB := createStandaloneTask(t, svc, "Task B", author)

	// When — removing bidirectional block (idempotent).
	err := svc.RemoveBidirectionalBlock(t.Context(), taskA.String(), taskB.String(), author)
	// Then — no error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// createStandaloneTask creates a task with no parent and returns its ID.
func createStandaloneTask(t *testing.T, svc driving.Service, title string, author string) domain.ID {
	t.Helper()

	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: author,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return out.Issue.ID()
}

package relcmd_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- RunRemove Tests ---

func TestRunRemove_BlockedBy_RemovesRelationship(t *testing.T) {
	t.Parallel()

	// Given: A is blocked_by B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	if err := svc.AddRelationship(t.Context(), taskA.String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: taskB.String(),
	}, author); err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: removing the blocked_by relationship.
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "blocked_by",
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the relationship is gone.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), taskA.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	for _, r := range shown.Relationships {
		if r.Type == domain.RelBlockedBy.String() && r.TargetID == taskB.String() {
			t.Error("blocked_by relationship still present after remove")
		}
	}
}

func TestRunRemove_Blocks_RemovesReverseRelationship(t *testing.T) {
	t.Parallel()

	// Given: A blocks B (stored as B blocked_by A).
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	if err := svc.AddRelationship(t.Context(), taskB.String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: taskA.String(),
	}, author); err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: removing "A blocks B".
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "blocks",
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the blocking relationship is gone.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), taskB.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	for _, r := range shown.Relationships {
		if r.Type == domain.RelBlockedBy.String() && r.TargetID == taskA.String() {
			t.Error("blocks relationship still present after remove")
		}
	}
}

func TestRunRemove_Refs_RemovesRelationship(t *testing.T) {
	t.Parallel()

	// Given: A refs B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	if err := svc.AddRelationship(t.Context(), taskA.String(), driving.RelationshipInput{
		Type:     domain.RelRefs,
		TargetID: taskB.String(),
	}, author); err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: removing the refs relationship.
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "refs",
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the relationship is gone.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), taskA.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	for _, r := range shown.Relationships {
		if r.Type == domain.RelRefs.String() && r.TargetID == taskB.String() {
			t.Error("refs relationship still present after remove")
		}
	}
}

func TestRunRemove_ParentOf_DetachesChild(t *testing.T) {
	t.Parallel()

	// Given: epic is the parent of task, set via one-shot update to avoid
	// leaving an active claim that would conflict with RunRemove's detach.
	svc := setupService(t)
	epic := createEpic(t, svc, "Parent epic")
	task := createTask(t, svc, "Child task")
	author := mustAuthor(t, "test-agent")

	epicStr := epic.String()
	if err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  task.String(),
		Author:   author,
		ParentID: &epicStr,
	}); err != nil {
		t.Fatalf("precondition: set parent failed: %v", err)
	}

	var buf bytes.Buffer

	// When: removing "epic parent_of task".
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       epic.String(),
		Rel:     "parent_of",
		B:       task.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the task has no parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != "" {
		t.Errorf("parent: got %q, want empty", shown.ParentID)
	}
}

func TestRunRemove_ChildOf_DetachesChild(t *testing.T) {
	t.Parallel()

	// Given: task is a child of epic, set via one-shot update to avoid leaving
	// an active claim that would conflict with RunRemove's detach.
	svc := setupService(t)
	task := createTask(t, svc, "Child task")
	epic := createEpic(t, svc, "Parent epic")
	author := mustAuthor(t, "test-agent")

	epicStr := epic.String()
	if err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  task.String(),
		Author:   author,
		ParentID: &epicStr,
	}); err != nil {
		t.Fatalf("precondition: set parent failed: %v", err)
	}

	var buf bytes.Buffer

	// When: removing "task child_of epic".
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       task.String(),
		Rel:     "child_of",
		B:       epic.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the task has no parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != "" {
		t.Errorf("parent: got %q, want empty", shown.ParentID)
	}
}

func TestRunRemove_ParentOf_WrongParent_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: task is a child of epicB, and epicA is an unrelated epic.
	svc := setupService(t)
	epicA := createEpic(t, svc, "Unrelated epic")
	epicB := createEpic(t, svc, "Actual parent epic")
	task := createTask(t, svc, "Child task")
	author := mustAuthor(t, "test-agent")

	epicBStr := epicB.String()
	if err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  task.String(),
		Author:   author,
		ParentID: &epicBStr,
	}); err != nil {
		t.Fatalf("precondition: set parent failed: %v", err)
	}

	var buf bytes.Buffer

	// When: attempting to remove "epicA parent_of task" (epicA is NOT task's parent).
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       epicA.String(),
		Rel:     "parent_of",
		B:       task.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: an error is returned and the task's parent is unchanged.
	if err == nil {
		t.Fatal("expected error when removing non-existent parent_of relationship, got nil")
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != epicB.String() {
		t.Errorf("parent: got %q, want %q — task was detached from unrelated parent", shown.ParentID, epicB.String())
	}
}

func TestRunRemove_ChildOf_WrongParent_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: task is a child of epicB, and epicA is an unrelated epic.
	svc := setupService(t)
	epicA := createEpic(t, svc, "Unrelated epic")
	epicB := createEpic(t, svc, "Actual parent epic")
	task := createTask(t, svc, "Child task")
	author := mustAuthor(t, "test-agent")

	epicBStr := epicB.String()
	if err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  task.String(),
		Author:   author,
		ParentID: &epicBStr,
	}); err != nil {
		t.Fatalf("precondition: set parent failed: %v", err)
	}

	var buf bytes.Buffer

	// When: attempting to remove "task child_of epicA" (epicA is NOT task's parent).
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       task.String(),
		Rel:     "child_of",
		B:       epicA.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: an error is returned and the task's parent is unchanged.
	if err == nil {
		t.Fatal("expected error when removing non-existent child_of relationship, got nil")
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != epicB.String() {
		t.Errorf("parent: got %q, want %q — task was detached from unrelated parent", shown.ParentID, epicB.String())
	}
}

func TestRunRemove_InvalidRel_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: two tasks.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: removing with an invalid relationship type.
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "depends_on",
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for invalid relationship type, got nil")
	}
}

func TestRunRemove_JSON_OutputsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given: A is blocked_by B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	if err := svc.AddRelationship(t.Context(), taskA.String(), driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: taskB.String(),
	}, author); err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: removing the relationship with JSON output.
	err := relcmd.RunRemove(t.Context(), relcmd.RunRemoveInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "blocked_by",
		B:       taskB.String(),
		Author:  author,
		JSON:    true,
		WriteTo: &buf,
	})
	// Then: output is valid JSON with the "removed" action field.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["action"] != "removed" {
		t.Errorf("action: got %q, want %q", result["action"], "removed")
	}
}

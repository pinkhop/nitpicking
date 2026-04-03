package relcmd_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- RunDetach Tests ---

func TestRunDetach_ChildFirst_DetachesParent(t *testing.T) {
	t.Parallel()

	// Given: a task that is a child of an epic.
	svc := setupService(t)
	epic := createEpic(t, svc, "Parent epic")
	task := createTask(t, svc, "Child task")
	author := mustAuthor(t, "test-agent")

	// Set the task's parent to the epic via one-shot update.
	epicStr := epic.String()
	err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  task.String(),
		Author:   author,
		ParentID: &epicStr,
	})
	if err != nil {
		t.Fatalf("precondition: set parent failed: %v", err)
	}

	var buf bytes.Buffer

	// When: detaching with child listed first.
	err = relcmd.RunDetach(t.Context(), relcmd.RunDetachInput{
		Service: svc,
		A:       task.String(),
		B:       epic.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the parent is cleared.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != "" {
		t.Errorf("parent should be cleared, got %s", shown.ParentID)
	}
}

func TestRunDetach_ParentFirst_DetachesParent(t *testing.T) {
	t.Parallel()

	// Given: a task that is a child of an epic.
	svc := setupService(t)
	epic := createEpic(t, svc, "Parent epic")
	task := createTask(t, svc, "Child task")
	author := mustAuthor(t, "test-agent")

	epicStr := epic.String()
	err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  task.String(),
		Author:   author,
		ParentID: &epicStr,
	})
	if err != nil {
		t.Fatalf("precondition: set parent failed: %v", err)
	}

	var buf bytes.Buffer

	// When: detaching with parent listed first.
	err = relcmd.RunDetach(t.Context(), relcmd.RunDetachInput{
		Service: svc,
		A:       epic.String(),
		B:       task.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the parent is cleared.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != "" {
		t.Errorf("parent should be cleared, got %s", shown.ParentID)
	}
}

func TestRunDetach_NoRelationship_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: two unrelated issues.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: detaching two issues with no parent-child relationship.
	err := relcmd.RunDetach(t.Context(), relcmd.RunDetachInput{
		Service: svc,
		A:       taskA.String(),
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for unrelated issues, got nil")
	}
}

func TestRunDetach_JSON_OutputsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given: a task that is a child of an epic.
	svc := setupService(t)
	epic := createEpic(t, svc, "Parent epic")
	task := createTask(t, svc, "Child task")
	author := mustAuthor(t, "test-agent")

	epicStr := epic.String()
	err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  task.String(),
		Author:   author,
		ParentID: &epicStr,
	})
	if err != nil {
		t.Fatalf("precondition: set parent failed: %v", err)
	}

	var buf bytes.Buffer

	// When: detaching with JSON output.
	err = relcmd.RunDetach(t.Context(), relcmd.RunDetachInput{
		Service: svc,
		A:       task.String(),
		B:       epic.String(),
		Author:  author,
		JSON:    true,
		WriteTo: &buf,
	})
	// Then: valid JSON with expected fields.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v", jsonErr)
	}
	if result["action"] != "detached" {
		t.Errorf("action: got %q, want %q", result["action"], "detached")
	}
}

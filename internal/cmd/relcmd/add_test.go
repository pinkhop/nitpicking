package relcmd_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

func createTask(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}
	return out.Issue.ID()
}

func createEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}
	return out.Issue.ID()
}

func claimIssue(t *testing.T, svc driving.Service, id domain.ID) string {
	t.Helper()
	ctx := t.Context()
	out, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: id.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	return out.ClaimID
}

// --- ParseRelArg Tests ---

func TestParseRelArg_ValidTypes_ReturnsExpectedResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantType  relcmd.RelArgType
		wantLabel string
	}{
		{
			name:      "blocked_by",
			input:     "blocked_by",
			wantType:  relcmd.RelArgRelationship,
			wantLabel: "blocked_by",
		},
		{
			name:      "blocks",
			input:     "blocks",
			wantType:  relcmd.RelArgRelationship,
			wantLabel: "blocks",
		},
		{
			name:      "cites",
			input:     "cites",
			wantType:  relcmd.RelArgRelationship,
			wantLabel: "cites",
		},
		{
			name:      "cited_by",
			input:     "cited_by",
			wantType:  relcmd.RelArgRelationship,
			wantLabel: "cited_by",
		},
		{
			name:      "refs",
			input:     "refs",
			wantType:  relcmd.RelArgRelationship,
			wantLabel: "refs",
		},
		{
			name:      "parent_of",
			input:     "parent_of",
			wantType:  relcmd.RelArgParentOf,
			wantLabel: "parent_of",
		},
		{
			name:      "child_of",
			input:     "child_of",
			wantType:  relcmd.RelArgChildOf,
			wantLabel: "child_of",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When: parsing the relationship argument.
			result, err := relcmd.ParseRelArg(tc.input)
			// Then: no error and correct type.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Type != tc.wantType {
				t.Errorf("type: got %v, want %v", result.Type, tc.wantType)
			}
			if result.Label != tc.wantLabel {
				t.Errorf("label: got %q, want %q", result.Label, tc.wantLabel)
			}
		})
	}
}

func TestParseRelArg_InvalidType_ReturnsError(t *testing.T) {
	t.Parallel()

	// When: parsing an invalid relationship argument.
	_, err := relcmd.ParseRelArg("depends_on")

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for invalid relationship type, got nil")
	}
}

// --- RunAdd Tests ---

func TestRunAdd_BlockedBy_CreatesRelationship(t *testing.T) {
	t.Parallel()

	// Given: two tasks.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding a blocked_by relationship.
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "blocked_by",
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the relationship exists.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), taskA.String())
	if err != nil {
		t.Fatalf("precondition: show issue failed: %v", err)
	}
	found := false
	for _, r := range shown.Relationships {
		if r.Type == domain.RelBlockedBy.String() && r.SourceID == taskA.String() && r.TargetID == taskB.String() {
			found = true
		}
	}
	if !found {
		t.Error("blocked_by relationship not found")
	}
}

func TestRunAdd_Refs_CreatesRelationship(t *testing.T) {
	t.Parallel()

	// Given: two tasks.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding a refs relationship.
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "refs",
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the relationship exists from both sides.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), taskA.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	found := false
	for _, r := range shown.Relationships {
		if r.Type == domain.RelRefs.String() && r.SourceID == taskA.String() && r.TargetID == taskB.String() {
			found = true
		}
	}
	if !found {
		t.Error("refs relationship not found")
	}
}

func TestRunAdd_Blocks_CreatesInverseRelationship(t *testing.T) {
	t.Parallel()

	// Given: two tasks.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding "A blocks B" (should store as B blocked_by A).
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "blocks",
		B:       taskB.String(),
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAdd_ParentOf_SetsParent(t *testing.T) {
	t.Parallel()

	// Given: an epic and a task, with the task claimed.
	svc := setupService(t)
	epic := createEpic(t, svc, "Parent epic")
	task := createTask(t, svc, "Child task")
	claimID := claimIssue(t, svc, task)
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding "epic parent_of task" (sets task's parent to epic).
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
		Service: svc,
		A:       epic.String(),
		Rel:     "parent_of",
		B:       task.String(),
		ClaimID: claimID,
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the task has the epic as its parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != epic.String() {
		t.Errorf("parent: got %s, want %s", shown.ParentID, epic)
	}
}

func TestRunAdd_ChildOf_SetsParent(t *testing.T) {
	t.Parallel()

	// Given: a task and an epic, with the task claimed.
	svc := setupService(t)
	task := createTask(t, svc, "Child task")
	epic := createEpic(t, svc, "Parent epic")
	claimID := claimIssue(t, svc, task)
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding "task child_of epic" (sets task's parent to epic).
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
		Service: svc,
		A:       task.String(),
		Rel:     "child_of",
		B:       epic.String(),
		ClaimID: claimID,
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the task has the epic as its parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), task.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != epic.String() {
		t.Errorf("parent: got %s, want %s", shown.ParentID, epic)
	}
}

func TestRunAdd_ParentOf_NoClaim_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: an epic and a task, with the task NOT claimed.
	svc := setupService(t)
	epic := createEpic(t, svc, "Parent epic")
	task := createTask(t, svc, "Child task")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding "epic parent_of task" without a claim.
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
		Service: svc,
		A:       epic.String(),
		Rel:     "parent_of",
		B:       task.String(),
		ClaimID: "",
		Author:  author,
		WriteTo: &buf,
	})

	// Then: an error is returned requiring a claim.
	if err == nil {
		t.Fatal("expected error for missing claim, got nil")
	}
}

func TestRunAdd_InvalidRel_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: two tasks.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding an invalid relationship type.
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
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

func TestRunAdd_JSON_OutputsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given: two tasks.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: adding a relationship with JSON output.
	err := relcmd.RunAdd(t.Context(), relcmd.RunAddInput{
		Service: svc,
		A:       taskA.String(),
		Rel:     "blocked_by",
		B:       taskB.String(),
		Author:  author,
		JSON:    true,
		WriteTo: &buf,
	})
	// Then: output is valid JSON with expected fields.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["action"] != "added" {
		t.Errorf("action: got %q, want %q", result["action"], "added")
	}
}

package jsoncmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/jsoncmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Update test helpers ---

// setupUpdateService initialises a service backed by in-memory fakes and
// returns it ready for update tests.
func setupUpdateService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// createAndClaimTask creates a task and claims it, returning the issue ID and
// claim ID.
func createAndClaimTask(t *testing.T, svc driving.Service, title string) (domain.ID, string) {
	t.Helper()
	ctx := t.Context()

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: out.Issue.ID().String(),
		Author:  "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	return out.Issue.ID(), claimOut.ClaimID
}

// --- RunUpdate Tests ---

func TestRunUpdate_UpdatesTitle_ReturnsJSON(t *testing.T) {
	t.Parallel()

	// Given: a claimed task and JSON input that sets a new title.
	svc := setupUpdateService(t)
	issueID, claimID := createAndClaimTask(t, svc, "Original title")

	stdin := strings.NewReader(`{"title": "Updated title"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)
	// Then: no error, JSON output confirms the update, and title changed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
	if result["updated"] != true {
		t.Errorf("updated: got %v, want true", result["updated"])
	}

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Title != "Updated title" {
		t.Errorf("title: got %q, want %q", shown.Title, "Updated title")
	}
}

func TestRunUpdate_MissingFieldsUnchanged(t *testing.T) {
	t.Parallel()

	// Given: a task with a specific title and description, and JSON that only
	// updates the priority.
	svc := setupUpdateService(t)
	ctx := t.Context()

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:        domain.RoleTask,
		Title:       "Keep this title",
		Description: "Keep this description",
		Author:      "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: out.Issue.ID().String(),
		Author:  "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	stdin := strings.NewReader(`{"priority": "P0"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimOut.ClaimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err = jsoncmd.RunUpdate(ctx, input)
	// Then: title and description remain unchanged; only priority changed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(ctx, out.Issue.ID().String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Title != "Keep this title" {
		t.Errorf("title should be unchanged: got %q", shown.Title)
	}
	if shown.Description != "Keep this description" {
		t.Errorf("description should be unchanged: got %q", shown.Description)
	}
	if shown.Priority != domain.P0 {
		t.Errorf("priority: got %s, want %s", shown.Priority, domain.P0)
	}
}

func TestRunUpdate_NullFieldUnsetsDescription(t *testing.T) {
	t.Parallel()

	// Given: a task with a description, and JSON that sets description to null.
	svc := setupUpdateService(t)
	ctx := t.Context()

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:        domain.RoleTask,
		Title:       "Has description",
		Description: "This should be cleared",
		Author:      "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: out.Issue.ID().String(),
		Author:  "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	stdin := strings.NewReader(`{"description": null}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimOut.ClaimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err = jsoncmd.RunUpdate(ctx, input)
	// Then: description is cleared.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(ctx, out.Issue.ID().String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Description != "" {
		t.Errorf("description should be cleared, got %q", shown.Description)
	}
}

func TestRunUpdate_SetsLabels(t *testing.T) {
	t.Parallel()

	// Given: a claimed task and JSON input with labels in key:value string
	// format (unified schema).
	svc := setupUpdateService(t)
	issueID, claimID := createAndClaimTask(t, svc, "Label task")

	stdin := strings.NewReader(`{"labels": ["kind:bug"]}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)
	// Then: the label is set.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	val, exists := shown.Labels["kind"]
	if !exists || val != "bug" {
		t.Errorf("label kind: got %q (exists=%v), want \"bug\"", val, exists)
	}
}

func TestRunUpdate_RemovesLabels(t *testing.T) {
	t.Parallel()

	// Given: a task with a label, and JSON input that removes it.
	svc := setupUpdateService(t)
	ctx := t.Context()

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Labeled task",
		Author: "test-agent",
		Labels: []driving.LabelInput{{Key: "kind", Value: "fix"}},
	})
	if err != nil {
		t.Fatalf("precondition: create task failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: out.Issue.ID().String(),
		Author:  "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	stdin := strings.NewReader(`{"label_remove": ["kind"]}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimOut.ClaimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err = jsoncmd.RunUpdate(ctx, input)
	// Then: the label is removed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(ctx, out.Issue.ID().String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if _, exists := shown.Labels["kind"]; exists {
		t.Error("label 'kind' should have been removed")
	}
}

func TestRunUpdate_CommentField_Rejected(t *testing.T) {
	t.Parallel()

	// Given: a claimed task and JSON input with a comment field — removed from
	// json update; comments should be added via "np json comment" instead.
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Comment task")

	stdin := strings.NewReader(`{"comment": "Reason for change"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)

	// Then: an error is returned because comment is an unknown field.
	if err == nil {
		t.Fatal("expected error for comment field, got nil")
	}
}

func TestRunUpdate_SetsParent(t *testing.T) {
	t.Parallel()

	// Given: an epic and a claimed task, with JSON input setting the parent.
	svc := setupUpdateService(t)
	ctx := t.Context()

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Parent epic",
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}

	issueID, claimID := createAndClaimTask(t, svc, "Child task")

	stdin := strings.NewReader(`{"parent": "` + epicOut.Issue.ID().String() + `"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err = jsoncmd.RunUpdate(ctx, input)
	// Then: the parent is set.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(ctx, issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != epicOut.Issue.ID().String() {
		t.Errorf("parent: got %s, want %s", shown.ParentID, epicOut.Issue.ID())
	}
}

func TestRunUpdate_NullParentRemovesParent(t *testing.T) {
	t.Parallel()

	// Given: a task under an epic, with JSON input setting parent to null.
	svc := setupUpdateService(t)
	ctx := t.Context()

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Parent epic",
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}

	taskOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child with parent",
		Author:   "test-agent",
		ParentID: epicOut.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child task failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: taskOut.Issue.ID().String(),
		Author:  "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	stdin := strings.NewReader(`{"parent": null}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimOut.ClaimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err = jsoncmd.RunUpdate(ctx, input)
	// Then: the parent is removed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(ctx, taskOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.ParentID != "" {
		t.Errorf("parent should be cleared, got %s", shown.ParentID)
	}
}

func TestRunUpdate_EmptyStdin_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: an empty stdin.
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Empty stdin task")

	stdin := strings.NewReader("")
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}
}

func TestRunUpdate_UnknownField_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: JSON with an unknown field.
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Unknown field task")

	stdin := strings.NewReader(`{"title": "ok", "bogus": true}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)

	// Then: an error is returned because unknown fields are rejected.
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestRunUpdate_RoleMatchesCurrent_SilentlyAccepted(t *testing.T) {
	t.Parallel()

	// Given: a claimed task and JSON with role matching the current role.
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Role match task")

	stdin := strings.NewReader(`{"role": "task", "title": "Updated"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)
	// Then: no error — matching role is silently accepted.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUpdate_RoleDiffers_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a claimed task and JSON with role set to "epic" (different from
	// the task's role).
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Role mismatch task")

	stdin := strings.NewReader(`{"role": "epic", "title": "Switched"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)

	// Then: an error is returned because the role doesn't match.
	if err == nil {
		t.Fatal("expected error for role mismatch, got nil")
	}
}

func TestRunUpdate_ClaimInJSON_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a claimed task and JSON with claim=true in the body (claiming is
	// done via np claim, not the JSON body).
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Claim in JSON task")

	stdin := strings.NewReader(`{"claim": true, "title": "Updated title"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)

	// Then: an error is returned because claim is not accepted in JSON input.
	if err == nil {
		t.Fatal("expected error for claim in JSON, got nil")
	}
}

func TestRunUpdate_StateInJSON_Rejected(t *testing.T) {
	t.Parallel()

	// Given: a claimed task and JSON input with a state field — state is an
	// unknown field for json update and is rejected by the decoder.
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "State task")

	stdin := strings.NewReader(`{"state": "open", "title": "Updated title"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)

	// Then: an error is returned because state is an unknown field.
	if err == nil {
		t.Fatal("expected error for state field, got nil")
	}
}

func TestRunUpdate_IdempotencyKey_Rejected(t *testing.T) {
	t.Parallel()

	// Given: JSON with the removed idempotency_key field.
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Idem key task")

	stdin := strings.NewReader(`{"title": "ok", "idempotency_key": "key-123"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)

	// Then: an error is returned because idempotency_key is an unknown field.
	if err == nil {
		t.Fatal("expected error for idempotency_key field, got nil")
	}
}

func TestRunUpdate_AllFields_Accepted(t *testing.T) {
	t.Parallel()

	// Given: a JSON object with all content fields accepted by json update.
	svc := setupUpdateService(t)
	_, claimID := createAndClaimTask(t, svc, "Unified schema task")

	stdinJSON := `{
		"role": "task",
		"title": "Unified update",
		"description": "desc",
		"acceptance_criteria": "ac",
		"priority": "P2",
		"labels": ["kind:test"],
		"label_remove": ["old-key"]
	}`
	stdin := strings.NewReader(stdinJSON)
	var stdout bytes.Buffer

	input := jsoncmd.RunUpdateInput{
		Service: svc,
		ClaimID: claimID,
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunUpdate(t.Context(), input)
	// Then: no error — all fields are accepted.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

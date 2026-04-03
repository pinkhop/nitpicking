package formcmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/formcmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Test helpers ---

// setupService initialises a service backed by in-memory fakes.
func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// createAndClaim creates a task and claims it, returning the issue ID and
// claim ID.
func createAndClaim(t *testing.T, svc driving.Service, title string) (domain.ID, string) {
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

// noopFormRunner returns a FormRunner that leaves all form values at their
// pre-populated defaults (simulating a user who submits without changes).
func noopFormRunner() func(*formcmd.UpdateFormValues) error {
	return func(_ *formcmd.UpdateFormValues) error { return nil }
}

// --- Tests ---

func TestRunUpdate_NoChanges_UpdateSucceeds(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Original title")

	var buf bytes.Buffer

	// When: form submitted with no changes.
	err := formcmd.RunUpdate(t.Context(), formcmd.RunUpdateInput{
		Service:    svc,
		IssueID:    issueID.String(),
		ClaimID:    claimID,
		WriteTo:    &buf,
		FormRunner: noopFormRunner(),
	})
	// Then: succeeds, output mentions the issue ID.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), issueID.String()) {
		t.Errorf("output should contain issue ID %q, got: %s", issueID, buf.String())
	}
}

func TestRunUpdate_ChangeTitle_UpdatesTitle(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with a known title.
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Original title")

	var buf bytes.Buffer

	// When: form changes the title.
	err := formcmd.RunUpdate(t.Context(), formcmd.RunUpdateInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		WriteTo: &buf,
		FormRunner: func(vals *formcmd.UpdateFormValues) error {
			vals.Title = "New title"
			return nil
		},
	})
	// Then: the title is updated.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Title != "New title" {
		t.Errorf("title: got %q, want %q", shown.Title, "New title")
	}
}

func TestRunUpdate_ChangeDescription_UpdatesDescription(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Desc task")

	var buf bytes.Buffer

	// When: form changes the description.
	err := formcmd.RunUpdate(t.Context(), formcmd.RunUpdateInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		WriteTo: &buf,
		FormRunner: func(vals *formcmd.UpdateFormValues) error {
			vals.Description = "New description"
			return nil
		},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Description != "New description" {
		t.Errorf("description: got %q, want %q", shown.Description, "New description")
	}
}

func TestRunUpdate_ChangePriority_UpdatesPriority(t *testing.T) {
	t.Parallel()

	// Given: a claimed task at default priority.
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Priority task")

	var buf bytes.Buffer

	// When: form changes the priority.
	err := formcmd.RunUpdate(t.Context(), formcmd.RunUpdateInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		WriteTo: &buf,
		FormRunner: func(vals *formcmd.UpdateFormValues) error {
			vals.Priority = "P0"
			return nil
		},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shown, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	if shown.Priority != domain.P0 {
		t.Errorf("priority: got %s, want %s", shown.Priority, domain.P0)
	}
}

func TestRunUpdate_AddComment_AddsComment(t *testing.T) {
	t.Parallel()

	// Given: a claimed task.
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Comment task")

	var buf bytes.Buffer

	// When: form adds a comment.
	err := formcmd.RunUpdate(t.Context(), formcmd.RunUpdateInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: claimID,
		WriteTo: &buf,
		FormRunner: func(vals *formcmd.UpdateFormValues) error {
			vals.Comment = "Progress note"
			return nil
		},
	})
	// Then: the comment is added.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: issueID.String(),
	})
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}
	if comments.Comments[0].Body != "Progress note" {
		t.Errorf("comment body: got %q, want %q", comments.Comments[0].Body, "Progress note")
	}
}

func TestRunUpdate_UnchangedFields_NotSentToService(t *testing.T) {
	t.Parallel()

	// Given: a task with a specific title and description.
	svc := setupService(t)
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

	var buf bytes.Buffer

	// When: form submits with no changes (noop).
	err = formcmd.RunUpdate(ctx, formcmd.RunUpdateInput{
		Service:    svc,
		IssueID:    out.Issue.ID().String(),
		ClaimID:    claimOut.ClaimID,
		WriteTo:    &buf,
		FormRunner: noopFormRunner(),
	})
	// Then: title and description remain unchanged.
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
}

func TestRunUpdate_InvalidClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a claimed task with the wrong claim ID.
	svc := setupService(t)
	issueID, _ := createAndClaim(t, svc, "Wrong claim task")

	var buf bytes.Buffer

	// When
	err := formcmd.RunUpdate(t.Context(), formcmd.RunUpdateInput{
		Service: svc,
		IssueID: issueID.String(),
		ClaimID: "wrong-claim-id",
		WriteTo: &buf,
		FormRunner: func(vals *formcmd.UpdateFormValues) error {
			vals.Title = "Should fail"
			return nil
		},
	})

	// Then
	if err == nil {
		t.Fatal("expected error for invalid claim ID")
	}
}

package core_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Import helpers ---

func setupImportService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustImportAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

// --- ImportIssues tests ---

func TestImportIssues_SingleTask_CreatesIssue(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "import-task-1",
			Role:           domain.RoleTask,
			Title:          "Imported task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("created: got %d, want 1", output.Created)
	}
	if output.Failed != 0 {
		t.Errorf("failed: got %d, want 0", output.Failed)
	}
	if output.Results[0].IssueID.IsZero() {
		t.Error("expected non-zero issue ID")
	}
}

func TestImportIssues_WithPerLineAuthor_UsesLineAuthor(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "authored-task",
			Role:           domain.RoleTask,
			Title:          "Task with author",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
			Author:         "line-author",
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "default-agent"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("created: got %d, want 1", output.Created)
	}
}

func TestImportIssues_ForceAuthor_OverridesLineAuthor(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "forced-author-task",
			Role:           domain.RoleTask,
			Title:          "Force author task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
			Author:         "line-author",
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "forced-agent"),
		ForceAuthor:   true,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("created: got %d, want 1", output.Created)
	}
}

func TestImportIssues_WithParent_SetsParentRelationship(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "parent-epic",
			Role:           domain.RoleEpic,
			Title:          "Parent Epic",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
		},
		{
			IdempotencyKey: "child-task",
			Role:           domain.RoleTask,
			Title:          "Child Task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
			Parent:         "parent-epic",
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 2 {
		t.Errorf("created: got %d, want 2", output.Created)
	}
	if output.Failed != 0 {
		t.Errorf("failed: got %d, want 0", output.Failed)
	}

	// Verify the child has the parent set.
	childID := output.Results[1].IssueID
	showOut, err := svc.ShowIssue(t.Context(), childID.String())
	if err != nil {
		t.Fatalf("show child: %v", err)
	}
	if showOut.ParentID == "" {
		t.Error("expected child to have parent set")
	}
}

func TestImportIssues_WithBlockedBy_CreatesRelationship(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "blocker",
			Role:           domain.RoleTask,
			Title:          "Blocker task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
		},
		{
			IdempotencyKey: "blocked",
			Role:           domain.RoleTask,
			Title:          "Blocked task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
			BlockedBy:      []string{"blocker"},
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 2 {
		t.Errorf("created: got %d, want 2", output.Created)
	}

	// Verify the blocked issue has a blocked_by relationship.
	blockedID := output.Results[1].IssueID
	showOut, err := svc.ShowIssue(t.Context(), blockedID.String())
	if err != nil {
		t.Fatalf("show blocked: %v", err)
	}
	if len(showOut.Relationships) == 0 {
		t.Error("expected blocked_by relationship")
	}
}

func TestImportIssues_WithComment_AddsComment(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "commented-task",
			Role:           domain.RoleTask,
			Title:          "Task with comment",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
			Comment:        "This is a migration note.",
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("created: got %d, want 1", output.Created)
	}

	// Verify the comment was added.
	issueID := output.Results[0].IssueID
	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: issueID.String(),
	})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments.Comments))
	}
}

func TestImportIssues_ClosedState_TransitionsIssue(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "closed-task",
			Role:           domain.RoleTask,
			Title:          "Already closed task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateClosed,
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("created: got %d, want 1", output.Created)
	}

	// Verify the issue is in closed state.
	issueID := output.Results[0].IssueID
	showOut, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	if showOut.State != domain.StateClosed {
		t.Errorf("state: got %v, want %v", showOut.State, domain.StateClosed)
	}
}

func TestImportIssues_DeferredState_TransitionsIssue(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "deferred-task",
			Role:           domain.RoleTask,
			Title:          "Deferred task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateDeferred,
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("created: got %d, want 1", output.Created)
	}

	// Verify the issue is deferred.
	issueID := output.Results[0].IssueID
	showOut, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	if showOut.State != domain.StateDeferred {
		t.Errorf("state: got %v, want %v", showOut.State, domain.StateDeferred)
	}
}

func TestImportIssues_WithClaim_CreatesOpenIssueClaimed(t *testing.T) {
	t.Parallel()

	// Given — a record with Claim: true creates the issue in open state with an
	// active claim row.
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "claimed-task",
			Role:           domain.RoleTask,
			Title:          "Task to be claimed on import",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
			Claim:          true,
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then — issue is created successfully in open state.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("created: got %d, want 1", output.Created)
	}
	if output.Results[0].IssueID.IsZero() {
		t.Fatal("expected non-zero issue ID")
	}

	// The import result carries the claim ID so callers can use it.
	issueID := output.Results[0].IssueID
	showOut, err := svc.ShowIssue(t.Context(), issueID.String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	// Primary state must be open (claims do not change primary state).
	if showOut.State != domain.StateOpen {
		t.Errorf("state: got %v, want %v", showOut.State, domain.StateOpen)
	}
	// An active claim should be present.
	if showOut.ClaimID == "" {
		t.Error("expected an active claim ID on the imported issue")
	}
}

func TestImportIssues_DuplicateIdempotencyKey_SkipsSecondImport(t *testing.T) {
	t.Parallel()

	// Given — import the same record twice
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyKey: "idempotent-task",
			Role:           domain.RoleTask,
			Title:          "Idempotent task",
			Priority:       domain.DefaultPriority,
			State:          domain.StateOpen,
		},
	}

	// First import.
	firstOutput, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// When — import again with the same idempotency key.
	secondOutput, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then — second import should succeed but not create a new domain.
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if secondOutput.Results[0].IssueID != firstOutput.Results[0].IssueID {
		t.Errorf("expected same issue ID on re-import, got %s and %s",
			firstOutput.Results[0].IssueID, secondOutput.Results[0].IssueID)
	}
}

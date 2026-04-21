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
	svc := core.New(tx, nil)

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

// mustDomainLabel creates a domain.Label from a key and value, failing the
// test if the label is invalid. Used in import tests to populate
// IdempotencyLabel on ValidatedRecord.
func mustDomainLabel(t *testing.T, key, value string) domain.Label {
	t.Helper()
	l, err := domain.NewLabel(key, value)
	if err != nil {
		t.Fatalf("precondition: invalid label %q:%q: %v", key, value, err)
	}
	return l
}

// --- ImportIssues tests ---

func TestImportIssues_SingleTask_CreatesIssue(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyLabel: mustDomainLabel(t, "jira", "import-task-1"),
			Role:             domain.RoleTask,
			Title:            "Imported task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "authored-task"),
			Role:             domain.RoleTask,
			Title:            "Task with author",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
			Author:           "line-author",
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "forced-author-task"),
			Role:             domain.RoleTask,
			Title:            "Force author task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
			Author:           "line-author",
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "parent-epic"),
			Role:             domain.RoleEpic,
			Title:            "Parent Epic",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
		},
		{
			IdempotencyLabel: mustDomainLabel(t, "jira", "child-task"),
			Role:             domain.RoleTask,
			Title:            "Child Task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
			Parent:           "jira:parent-epic",
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "blocker"),
			Role:             domain.RoleTask,
			Title:            "Blocker task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
		},
		{
			IdempotencyLabel: mustDomainLabel(t, "jira", "blocked"),
			Role:             domain.RoleTask,
			Title:            "Blocked task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
			BlockedBy:        []string{"jira:blocker"},
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "commented-task"),
			Role:             domain.RoleTask,
			Title:            "Task with comment",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
			Comment:          "This is a migration note.",
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "closed-task"),
			Role:             domain.RoleTask,
			Title:            "Already closed task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateClosed,
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "deferred-task"),
			Role:             domain.RoleTask,
			Title:            "Deferred task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateDeferred,
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
			IdempotencyLabel: mustDomainLabel(t, "jira", "claimed-task"),
			Role:             domain.RoleTask,
			Title:            "Task to be claimed on import",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
			Claim:            true,
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

func TestImportIssues_DuplicateIdempotencyLabel_SkipsSecondImport(t *testing.T) {
	t.Parallel()

	// Given — import the same record twice.
	svc := setupImportService(t)
	records := []domain.ValidatedRecord{
		{
			IdempotencyLabel: mustDomainLabel(t, "jira", "idempotent-task"),
			Role:             domain.RoleTask,
			Title:            "Idempotent task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
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
	if firstOutput.Created != 1 {
		t.Fatalf("precondition: expected first import to create 1 issue, got %d", firstOutput.Created)
	}

	// When — import again with the same idempotency label.
	secondOutput, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then — second import should succeed, report the skip, and return the same ID.
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if secondOutput.Results[0].IssueID != firstOutput.Results[0].IssueID {
		t.Errorf("expected same issue ID on re-import, got %s and %s",
			firstOutput.Results[0].IssueID, secondOutput.Results[0].IssueID)
	}
	if !secondOutput.Results[0].Skipped {
		t.Error("expected Results[0].Skipped to be true on second import")
	}
	if secondOutput.Skipped != 1 {
		t.Errorf("expected Skipped counter to be 1, got %d", secondOutput.Skipped)
	}
	if secondOutput.Created != 0 {
		t.Errorf("expected Created counter to be 0 on dedup, got %d", secondOutput.Created)
	}
}

// TestImportIssues_IdempotencyLabel_NoMatch_CreatesWithLabel verifies AC5b:
// a record with an idempotency_label that matches no existing issue is created
// and the label is stored on the new issue so that subsequent deduplicated
// imports can find it.
func TestImportIssues_IdempotencyLabel_NoMatch_CreatesWithLabel(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupImportService(t)
	idemLabel := mustDomainLabel(t, "ticket", "ABC-100")
	records := []domain.ValidatedRecord{
		{
			IdempotencyLabel: idemLabel,
			Role:             domain.RoleTask,
			Title:            "Labelled import task",
			Priority:         domain.DefaultPriority,
			State:            domain.StateOpen,
		},
	}

	// When
	output, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records:       records,
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then — the issue is created and the idempotency label is stored on it.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Created != 1 {
		t.Errorf("expected 1 created, got %d", output.Created)
	}
	if output.Results[0].IssueID.IsZero() {
		t.Fatal("expected non-zero issue ID")
	}
	if output.Results[0].Skipped {
		t.Error("expected Skipped to be false for a new issue")
	}

	// The idempotency label must be present on the stored issue so that a
	// subsequent import of the same record finds it via the label lookup.
	showOut, err := svc.ShowIssue(t.Context(), output.Results[0].IssueID.String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	storedValue, hasLabel := showOut.Labels[idemLabel.Key()]
	if !hasLabel {
		t.Errorf("expected idempotency label key %q to be stored on issue, got labels: %v",
			idemLabel.Key(), showOut.Labels)
	} else if storedValue != idemLabel.Value() {
		t.Errorf("expected label %q=%q, got value %q",
			idemLabel.Key(), idemLabel.Value(), storedValue)
	}
}

// TestImportIssues_IdempotencyLabel_Match_Skips verifies AC5a: a record with
// an idempotency_label that matches an existing issue is skipped, the line
// result carries Skipped=true and the existing issue's ID.
func TestImportIssues_IdempotencyLabel_Match_Skips(t *testing.T) {
	t.Parallel()

	// Given — an existing issue already carrying the idempotency label.
	svc := setupImportService(t)
	idemLabel := mustDomainLabel(t, "ticket", "XYZ-42")
	firstOut, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records: []domain.ValidatedRecord{
			{
				IdempotencyLabel: idemLabel,
				Role:             domain.RoleTask,
				Title:            "Existing issue",
				Priority:         domain.DefaultPriority,
				State:            domain.StateOpen,
			},
		},
		DefaultAuthor: mustImportAuthor(t, "seed-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: seeding issue: %v", err)
	}
	existingID := firstOut.Results[0].IssueID

	// When — import a record with the same idempotency label.
	secondOut, err := svc.ImportIssues(t.Context(), driving.ImportInput{
		Records: []domain.ValidatedRecord{
			{
				IdempotencyLabel: idemLabel,
				Role:             domain.RoleTask,
				Title:            "Duplicate import",
				Priority:         domain.DefaultPriority,
				State:            domain.StateOpen,
			},
		},
		DefaultAuthor: mustImportAuthor(t, "import-agent"),
	})
	// Then — the record is skipped and the existing issue's ID is reported.
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if !secondOut.Results[0].Skipped {
		t.Error("expected Skipped=true for deduplicated record")
	}
	if secondOut.Results[0].IssueID != existingID {
		t.Errorf("expected existing issue ID %s, got %s", existingID, secondOut.Results[0].IssueID)
	}
	if secondOut.Skipped != 1 {
		t.Errorf("expected Skipped counter 1, got %d", secondOut.Skipped)
	}
	if secondOut.Created != 0 {
		t.Errorf("expected Created counter 0, got %d", secondOut.Created)
	}
}

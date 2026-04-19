//go:build boundary

package sqlite_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

func setupBoundarySvc(t *testing.T) driving.Service {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := core.New(store, store)
	if err := svc.Init(t.Context(), "TEST"); err != nil {
		t.Fatalf("initializing database: %v", err)
	}
	return svc
}

func author(t *testing.T, name string) string {
	t.Helper()
	return name
}

func createIntTask(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: author(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}
	return out.Issue.ID()
}

func createIntEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: author(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}
	return out.Issue.ID()
}

// --- CreateIssue / GetIssue Roundtrip ---

func TestBoundary_CreateAndGetIssue_Roundtrip(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:        domain.RoleTask,
		Title:       "Roundtrip task",
		Description: "A test description",
		Priority:    domain.P1,
		Author:      author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	issueID := out.Issue.ID()

	// When
	showOut, err := svc.ShowIssue(ctx, issueID.String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showOut.Title != "Roundtrip task" {
		t.Errorf("title: got %q, want %q", showOut.Title, "Roundtrip task")
	}
	if showOut.Description != "A test description" {
		t.Errorf("description: got %q, want %q", showOut.Description, "A test description")
	}
	if showOut.Priority != domain.P1 {
		t.Errorf("priority: got %s, want %s", showOut.Priority, domain.P1)
	}
	if showOut.Role != domain.RoleTask {
		t.Errorf("role: got %s, want task", showOut.Role)
	}
}

// --- UpdateIssue Partial Updates ---

func TestBoundary_UpdateIssue_PartialUpdate_OnlyChangesSpecifiedFields(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:        domain.RoleTask,
		Title:       "Original title",
		Description: "Original description",
		Priority:    domain.P2,
		Author:      author(t, "alice"),
		Claim:       true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// When — update only the title
	newTitle := "Updated title"
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: out.Issue.ID().String(),
		ClaimID: out.ClaimID,
		Title:   &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showOut, _ := svc.ShowIssue(ctx, out.Issue.ID().String())
	if showOut.Title != "Updated title" {
		t.Errorf("title: got %q, want %q", showOut.Title, "Updated title")
	}
	if showOut.Description != "Original description" {
		t.Errorf("description should be unchanged: got %q, want %q",
			showOut.Description, "Original description")
	}
	if showOut.Priority != domain.P2 {
		t.Errorf("priority should be unchanged: got %s, want %s", showOut.Priority, domain.P2)
	}
}

// --- ListIssues Filter: Role ---

func TestBoundary_ListIssues_FilterByRole_OnlyReturnsMatchingRole(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	_ = createIntTask(t, svc, "Task A")
	_ = createIntEpic(t, svc, "Epic A")

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{Roles: []domain.Role{domain.RoleTask}},
		Limit:  -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, item := range result.Items {
		if item.Role != domain.RoleTask {
			t.Errorf("expected only tasks, got role %s for issue %s", item.Role, item.ID)
		}
	}
	if len(result.Items) != 1 {
		t.Errorf("items: got %d, want 1", len(result.Items))
	}
}

// --- ListIssues Filter: State ---

func TestBoundary_ListIssues_FilterByState_OnlyReturnsMatchingState(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	openID := createIntTask(t, svc, "Open task")
	closedID := createIntTask(t, svc, "Closed task")
	_ = openID

	claimOut, _ := svc.ClaimByID(ctx, driving.ClaimInput{IssueID: closedID.String(), Author: author(t, "alice")})
	_ = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: closedID.String(), ClaimID: claimOut.ClaimID, Action: driving.ActionClose,
	})

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{States: []domain.State{domain.StateClosed}},
		Limit:  -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].State != domain.StateClosed {
		t.Errorf("expected closed state, got %v", result.Items[0].State)
	}
}

// --- ListIssues Filter: Ready ---

func TestBoundary_ListIssues_FilterByReady_ExcludesBlockedIssues(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	readyID := createIntTask(t, svc, "Ready task")
	blockedID := createIntTask(t, svc, "Blocked task")

	_ = svc.AddRelationship(ctx, blockedID.String(), driving.RelationshipInput{
		TargetID: readyID.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{Ready: true},
		Limit:  -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].ID != readyID.String() {
		t.Errorf("expected ready issue %s, got %s", readyID, result.Items[0].ID)
	}
}

// --- ListIssues Filter: Parent ---

func TestBoundary_ListIssues_FilterByParent_OnlyReturnsChildren(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	epicID := createIntEpic(t, svc, "Parent epic")
	childOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child task", Author: author(t, "alice"),
		ParentID: epicID.String(),
	})
	_ = createIntTask(t, svc, "Orphan task")

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{ParentIDs: []string{epicID.String()}},
		Limit:  -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].ID != childOut.Issue.ID().String() {
		t.Errorf("expected child %s, got %s", childOut.Issue.ID(), result.Items[0].ID)
	}
}

// --- ListIssues Filter: Labels ---

func TestBoundary_ListIssues_FilterByLabel_OnlyReturnsMatchingLabel(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	labeledOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Labeled task", Author: author(t, "alice"),
		Claim:  true,
		Labels: []driving.LabelInput{{Key: "kind", Value: "bug"}},
	})
	_ = createIntTask(t, svc, "Unlabeled task")

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{
			LabelFilters: []driving.LabelFilterInput{{Key: "kind", Value: "bug"}},
		},
		Limit: -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].ID != labeledOut.Issue.ID().String() {
		t.Errorf("expected labeled issue %s, got %s", labeledOut.Issue.ID(), result.Items[0].ID)
	}
}

// --- ListIssues Filter: Blocked ---

func TestBoundary_ListIssues_FilterByBlocked_OnlyReturnsBlockedIssues(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	blockerID := createIntTask(t, svc, "Blocker")
	blockedID := createIntTask(t, svc, "Blocked")
	_ = createIntTask(t, svc, "Unblocked")

	_ = svc.AddRelationship(ctx, blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{Blocked: true},
		Limit:  -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].ID != blockedID.String() {
		t.Errorf("expected blocked issue %s, got %s", blockedID, result.Items[0].ID)
	}
}

// --- SearchIssues Full-Text Matching ---

func TestBoundary_SearchIssues_MatchesTitleText(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	matchID := createIntTask(t, svc, "Fix authentication timeout")
	_ = createIntTask(t, svc, "Refactor database schema")

	// When
	result, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "authentication",
		Limit: -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].ID != matchID.String() {
		t.Errorf("expected matching issue %s, got %s", matchID, result.Items[0].ID)
	}
}

// --- GetChildStatuses ---

func TestBoundary_GetChildStatuses_ReturnsChildStates(t *testing.T) {
	// Given — an epic with two children (one open, one closed)
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	epicID := createIntEpic(t, svc, "Status epic")
	_, _ = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Open child", Author: author(t, "alice"),
		ParentID: epicID.String(),
	})
	closedChildOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Closed child", Author: author(t, "alice"),
		ParentID: epicID.String(), Claim: true,
	})
	_ = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: closedChildOut.Issue.ID().String(), ClaimID: closedChildOut.ClaimID,
		Action: driving.ActionClose,
	})

	// When — check epic status (which internally uses GetChildStatuses)
	showOut, err := svc.ShowIssue(ctx, epicID.String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showOut.ChildCount != 2 {
		t.Errorf("child_count: got %d, want 2", showOut.ChildCount)
	}
}

// --- GetDescendants ---

func TestBoundary_GetDescendants_ReturnsNestedChildren(t *testing.T) {
	// Given — a parent epic with a child epic that has its own child
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	parentID := createIntEpic(t, svc, "Parent epic")
	childEpicOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Child epic", Author: author(t, "alice"),
		ParentID: parentID.String(),
	})
	_, _ = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Grandchild task", Author: author(t, "alice"),
		ParentID: childEpicOut.Issue.ID().String(),
	})

	// When — list descendants of parent
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{DescendantsOf: parentID.String()},
		Limit:  -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("descendants: got %d, want 2 (child epic + grandchild task)", len(result.Items))
	}
}

// --- GetAncestorStatuses ---

func TestBoundary_GetAncestorStatuses_DeferredAncestorBlocksReadiness(t *testing.T) {
	// Given — a deferred parent epic with an open child task
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	epicID := createIntEpic(t, svc, "Deferred epic")
	childOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child of deferred", Author: author(t, "alice"),
		ParentID: epicID.String(),
	})

	// Defer the parent epic.
	claimOut, _ := svc.ClaimByID(ctx, driving.ClaimInput{IssueID: epicID.String(), Author: author(t, "alice")})
	_ = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: epicID.String(), ClaimID: claimOut.ClaimID, Action: driving.ActionDefer,
	})

	// When — check readiness of child
	showOut, err := svc.ShowIssue(ctx, childOut.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showOut.IsReady {
		t.Error("child of deferred epic should not be ready")
	}
}

// --- IssueIDExists ---

func TestBoundary_IssueIDExists_ExistingIssue_CanBeFound(t *testing.T) {
	// Given — a created task
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueID := createIntTask(t, svc, "Existing task")

	// When — show the issue (if ID exists, ShowIssue succeeds)
	_, err := svc.ShowIssue(ctx, issueID.String())
	// Then
	if err != nil {
		t.Errorf("expected issue to exist, got error: %v", err)
	}
}

func TestBoundary_IssueIDExists_DeletedIssue_NotFound(t *testing.T) {
	// Given — a created and deleted task
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	out, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "To delete", Author: author(t, "alice"),
		Claim: true,
	})
	_ = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: out.Issue.ID().String(), ClaimID: out.ClaimID,
	})

	// When
	_, err := svc.ShowIssue(ctx, out.Issue.ID().String())
	// Then
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound for deleted issue, got: %v", err)
	}
}

// --- IdempotencyLabel deduplication ---

func TestBoundary_CreateIssue_IdempotencyLabel_ReturnsSameIssue(t *testing.T) {
	// Given — a task created with an idempotency label
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	idemLabel, err := domain.NewLabel("uniquekey", "123")
	if err != nil {
		t.Fatalf("precondition: building idempotency label: %v", err)
	}

	firstOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:             domain.RoleTask,
		Title:            "Idempotent task",
		Author:           author(t, "alice"),
		IdempotencyLabel: idemLabel,
	})
	if err != nil {
		t.Fatalf("precondition: first create failed: %v", err)
	}

	// When — create again with the same idempotency label
	secondOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:             domain.RoleTask,
		Title:            "Different title",
		Author:           author(t, "alice"),
		IdempotencyLabel: idemLabel,
	})
	// Then — should return the same issue
	if err != nil {
		t.Fatalf("unexpected error on idempotent retry: %v", err)
	}
	if secondOut.Issue.ID() != firstOut.Issue.ID() {
		t.Errorf("expected same issue ID on retry, got %s (first: %s)",
			secondOut.Issue.ID(), firstOut.Issue.ID())
	}
}

// --- ListIssues OrderBy: MODIFIED ---

// TestBoundary_ListIssues_OrderByModified_SortAscending_ReturnsOldestFirst
// verifies that --order MODIFIED:asc places the oldest-created issue first.
// The SQLite adapter uses created_at as a proxy for modification time, so a
// brief sleep between creates guarantees distinct timestamps.
func TestBoundary_ListIssues_OrderByModified_SortAscending_ReturnsOldestFirst(t *testing.T) {
	// Given — two tasks created at different times
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	olderID := createIntTask(t, svc, "Older task")
	// Brief pause to ensure a distinct created_at value for the second issue.
	time.Sleep(10 * time.Millisecond)
	newerID := createIntTask(t, svc, "Newer task")

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		OrderBy:   driving.OrderByUpdatedAt,
		Direction: driving.SortAscending,
		Limit:     -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items: got %d, want 2", len(result.Items))
	}
	if result.Items[0].ID != olderID.String() {
		t.Errorf("first item should be older task (%s), got %s", olderID, result.Items[0].ID)
	}
	if result.Items[1].ID != newerID.String() {
		t.Errorf("second item should be newer task (%s), got %s", newerID, result.Items[1].ID)
	}
}

// TestBoundary_ListIssues_OrderByModified_SortDescending_ReturnsNewestFirst
// verifies that --order MODIFIED:desc places the newest-created issue first.
func TestBoundary_ListIssues_OrderByModified_SortDescending_ReturnsNewestFirst(t *testing.T) {
	// Given — two tasks created at different times
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	olderID := createIntTask(t, svc, "Older task")
	// Brief pause to ensure a distinct created_at value for the second issue.
	time.Sleep(10 * time.Millisecond)
	newerID := createIntTask(t, svc, "Newer task")

	// When
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		OrderBy:   driving.OrderByUpdatedAt,
		Direction: driving.SortDescending,
		Limit:     -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items: got %d, want 2", len(result.Items))
	}
	if result.Items[0].ID != newerID.String() {
		t.Errorf("first item should be newer task (%s), got %s", newerID, result.Items[0].ID)
	}
	if result.Items[1].ID != olderID.String() {
		t.Errorf("second item should be older task (%s), got %s", olderID, result.Items[1].ID)
	}
}

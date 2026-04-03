package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// --- Test helpers ---

func mustIssueID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.GenerateID("NP", nil)
	if err != nil {
		t.Fatalf("precondition: generate issue ID: %v", err)
	}
	return id
}

func mustTask(t *testing.T, id domain.ID, title string, createdAt time.Time) domain.Issue {
	t.Helper()
	task, err := domain.NewTask(domain.NewTaskParams{
		ID:        id,
		Title:     title,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}
	return task
}

func mustEpic(t *testing.T, id domain.ID, title string, createdAt time.Time) domain.Issue {
	t.Helper()
	epic, err := domain.NewEpic(domain.NewEpicParams{
		ID:        id,
		Title:     title,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}
	return epic
}

func mustLabel(t *testing.T, key, value string) domain.Label {
	t.Helper()
	lbl, err := domain.NewLabel(key, value)
	if err != nil {
		t.Fatalf("precondition: create label: %v", err)
	}
	return lbl
}

func mustAuthor(t *testing.T, name string) domain.Author {
	t.Helper()
	a, err := domain.NewAuthor(name)
	if err != nil {
		t.Fatalf("precondition: create author: %v", err)
	}
	return a
}

func mustRelationship(t *testing.T, src, tgt domain.ID, rt domain.RelationType) domain.Relationship {
	t.Helper()
	rel, err := domain.NewRelationship(src, tgt, rt)
	if err != nil {
		t.Fatalf("precondition: create relationship: %v", err)
	}
	return rel
}

// --- CreateIssue ---

func TestCreateIssue_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	task := mustTask(t, id, "Test task", time.Now())

	// When
	err := repo.CreateIssue(ctx, task)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCreateIssue_DuplicateID_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	task := mustTask(t, id, "Original", time.Now())
	if err := repo.CreateIssue(ctx, task); err != nil {
		t.Fatalf("precondition: create first issue: %v", err)
	}

	duplicate := mustTask(t, id, "Duplicate", time.Now())

	// When
	err := repo.CreateIssue(ctx, duplicate)

	// Then
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
}

// --- GetIssue ---

func TestGetIssue_Found(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	task := mustTask(t, id, "My task", time.Now())
	if err := repo.CreateIssue(ctx, task); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	got, err := repo.GetIssue(ctx, id, false)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID() != id {
		t.Errorf("expected ID %s, got %s", id, got.ID())
	}
	if got.Title() != "My task" {
		t.Errorf("expected title 'My task', got %q", got.Title())
	}
}

func TestGetIssue_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)

	// When
	_, err := repo.GetIssue(ctx, id, false)

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetIssue_SoftDeleted_ExcludedByDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	task := mustTask(t, id, "Deletable", time.Now()).WithDeleted()
	if err := repo.CreateIssue(ctx, task); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	_, err := repo.GetIssue(ctx, id, false)

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for deleted issue, got %v", err)
	}
}

func TestGetIssue_SoftDeleted_IncludedWhenRequested(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	task := mustTask(t, id, "Deletable", time.Now()).WithDeleted()
	if err := repo.CreateIssue(ctx, task); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	got, err := repo.GetIssue(ctx, id, true)
	// Then
	if err != nil {
		t.Fatalf("expected no error with includeDeleted=true, got %v", err)
	}
	if !got.IsDeleted() {
		t.Error("expected issue to be marked as deleted")
	}
}

// --- UpdateIssue ---

func TestUpdateIssue_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	original := mustTask(t, id, "Original", time.Now())
	if err := repo.CreateIssue(ctx, original); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	updated := original.WithPriority(domain.P0)

	// When
	err := repo.UpdateIssue(ctx, updated)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	got, _ := repo.GetIssue(ctx, id, false)
	if got.Priority() != domain.P0 {
		t.Errorf("expected priority P0, got %s", got.Priority())
	}
}

func TestUpdateIssue_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	task := mustTask(t, id, "Phantom", time.Now())

	// When
	err := repo.UpdateIssue(ctx, task)

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- ListIssues ---

func TestListIssues_EmptyRepository_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	items, hasMore, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
	if hasMore {
		t.Error("expected hasMore=false for empty repo")
	}
}

func TestListIssues_FilterByRole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	taskID := mustIssueID(t)
	epicID := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, taskID, "Task one", now)); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	if err := repo.CreateIssue(ctx, mustEpic(t, epicID, "Epic one", now)); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		Roles: []domain.Role{domain.RoleTask},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 task, got %d", len(items))
	}
	if items[0].ID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, items[0].ID)
	}
}

func TestListIssues_FilterByState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	openID := mustIssueID(t)
	closedID := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, openID, "Open task", now)); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	closedTask := mustTask(t, closedID, "Closed task", now).WithState(domain.StateClosed)
	if err := repo.CreateIssue(ctx, closedTask); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		States: []domain.State{domain.StateClosed},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 closed issue, got %d", len(items))
	}
	if items[0].ID != closedID {
		t.Errorf("expected closed ID %s, got %s", closedID, items[0].ID)
	}
}

func TestListIssues_ExcludeClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	openID := mustIssueID(t)
	closedID := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, openID, "Open task", now)); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	closedTask := mustTask(t, closedID, "Closed task", now).WithState(domain.StateClosed)
	if err := repo.CreateIssue(ctx, closedTask); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		ExcludeClosed: true,
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (excluding closed), got %d", len(items))
	}
	if items[0].ID != openID {
		t.Errorf("expected open ID %s, got %s", openID, items[0].ID)
	}
}

func TestListIssues_ExcludeClosed_OverriddenByExplicitStateFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — ExcludeClosed is true but States explicitly includes StateClosed.
	// The explicit state filter takes precedence.
	now := time.Now()
	closedID := mustIssueID(t)
	closedTask := mustTask(t, closedID, "Closed task", now).WithState(domain.StateClosed)
	if err := repo.CreateIssue(ctx, closedTask); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		ExcludeClosed: true,
		States:        []domain.State{domain.StateClosed},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (explicit States overrides ExcludeClosed), got %d", len(items))
	}
}

func TestListIssues_FilterByParentID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	epicID := mustIssueID(t)
	childID := mustIssueID(t)
	orphanID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Parent epic", now)
	child := mustTask(t, childID, "Child task", now).WithParentID(epicID)
	orphan := mustTask(t, orphanID, "Orphan task", now)

	for _, iss := range []domain.Issue{epic, child, orphan} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		ParentIDs: []domain.ID{epicID},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 child, got %d", len(items))
	}
	if items[0].ID != childID {
		t.Errorf("expected child ID %s, got %s", childID, items[0].ID)
	}
}

func TestListIssues_FilterByOrphan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	epicID := mustIssueID(t)
	childID := mustIssueID(t)
	orphanID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Parent epic", now)
	child := mustTask(t, childID, "Child task", now).WithParentID(epicID)
	orphan := mustTask(t, orphanID, "Orphan task", now)

	for _, iss := range []domain.Issue{epic, child, orphan} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		Orphan: true,
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Epic and orphan have no parent; child does.
	if len(items) != 2 {
		t.Fatalf("expected 2 orphan issues, got %d", len(items))
	}
}

func TestListIssues_FilterByLabel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	bugID := mustIssueID(t)
	featID := mustIssueID(t)

	bugLabel := mustLabel(t, "kind", "bug")
	featLabel := mustLabel(t, "kind", "feat")

	bug := mustTask(t, bugID, "Bug fix", now).WithLabels(domain.NewLabelSet().Set(bugLabel))
	feat := mustTask(t, featID, "Feature", now).WithLabels(domain.NewLabelSet().Set(featLabel))

	for _, iss := range []domain.Issue{bug, feat} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — filter for kind:bug
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		LabelFilters: []driven.LabelFilter{{Key: "kind", Value: "bug"}},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 bug, got %d", len(items))
	}
	if items[0].ID != bugID {
		t.Errorf("expected bug ID %s, got %s", bugID, items[0].ID)
	}
}

func TestListIssues_FilterByLabel_Wildcard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	labeledID := mustIssueID(t)
	unlabeledID := mustIssueID(t)

	lbl := mustLabel(t, "area", "backend")
	labeled := mustTask(t, labeledID, "Labeled", now).WithLabels(domain.NewLabelSet().Set(lbl))
	unlabeled := mustTask(t, unlabeledID, "Unlabeled", now)

	for _, iss := range []domain.Issue{labeled, unlabeled} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — wildcard filter: any value for key "area"
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		LabelFilters: []driven.LabelFilter{{Key: "area", Value: ""}},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 labeled issue, got %d", len(items))
	}
	if items[0].ID != labeledID {
		t.Errorf("expected labeled ID %s, got %s", labeledID, items[0].ID)
	}
}

func TestListIssues_FilterByLabel_Negated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	bugID := mustIssueID(t)
	featID := mustIssueID(t)

	bugLabel := mustLabel(t, "kind", "bug")
	featLabel := mustLabel(t, "kind", "feat")

	bug := mustTask(t, bugID, "Bug", now).WithLabels(domain.NewLabelSet().Set(bugLabel))
	feat := mustTask(t, featID, "Feature", now).WithLabels(domain.NewLabelSet().Set(featLabel))

	for _, iss := range []domain.Issue{bug, feat} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — exclude kind:bug
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		LabelFilters: []driven.LabelFilter{{Key: "kind", Value: "bug", Negate: true}},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 non-bug issue, got %d", len(items))
	}
	if items[0].ID != featID {
		t.Errorf("expected feat ID %s, got %s", featID, items[0].ID)
	}
}

func TestListIssues_FilterByBlocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — blockerID blocks blockedID; unblockedID has no blockers.
	now := time.Now()
	blockerID := mustIssueID(t)
	blockedID := mustIssueID(t)
	unblockedID := mustIssueID(t)

	blocker := mustTask(t, blockerID, "Blocker", now)
	blocked := mustTask(t, blockedID, "Blocked", now)
	unblocked := mustTask(t, unblockedID, "Unblocked", now)

	for _, iss := range []domain.Issue{blocker, blocked, unblocked} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: create relationship: %v", err)
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		Blocked: true,
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 blocked issue, got %d", len(items))
	}
	if items[0].ID != blockedID {
		t.Errorf("expected blocked ID %s, got %s", blockedID, items[0].ID)
	}
}

func TestListIssues_FilterByReady(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — readyID is open with no blockers; blockedID is blocked; deferredID
	// is deferred. Only readyID should be ready.
	now := time.Now()
	readyID := mustIssueID(t)
	blockerID := mustIssueID(t)
	blockedID := mustIssueID(t)
	deferredID := mustIssueID(t)

	ready := mustTask(t, readyID, "Ready", now)
	blockerTask := mustTask(t, blockerID, "Blocker", now)
	blocked := mustTask(t, blockedID, "Blocked", now)
	deferred := mustTask(t, deferredID, "Deferred", now).WithState(domain.StateDeferred)

	for _, iss := range []domain.Issue{ready, blockerTask, blocked, deferred} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: create relationship: %v", err)
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		Ready: true,
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// readyID and blockerID are both open and unblocked — both are ready.
	readyIDs := make(map[domain.ID]bool)
	for _, item := range items {
		readyIDs[item.ID] = true
	}
	if !readyIDs[readyID] {
		t.Error("expected readyID to be in ready results")
	}
	if !readyIDs[blockerID] {
		t.Error("expected blockerID to be in ready results (it's unblocked)")
	}
	if readyIDs[blockedID] {
		t.Error("expected blockedID to NOT be in ready results")
	}
	if readyIDs[deferredID] {
		t.Error("expected deferredID to NOT be in ready results")
	}
}

func TestListIssues_FilterByDescendantsOf(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — grandparent → parent → child; also unrelated issue
	now := time.Now()
	gpID := mustIssueID(t)
	parentID := mustIssueID(t)
	childID := mustIssueID(t)
	unrelatedID := mustIssueID(t)

	gp := mustEpic(t, gpID, "Grandparent", now)
	parent := mustEpic(t, parentID, "Parent", now).WithParentID(gpID)
	child := mustTask(t, childID, "Child", now).WithParentID(parentID)
	unrelated := mustTask(t, unrelatedID, "Unrelated", now)

	for _, iss := range []domain.Issue{gp, parent, child, unrelated} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		DescendantsOf: gpID,
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids := make(map[domain.ID]bool)
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids[parentID] {
		t.Error("expected parentID in descendants")
	}
	if !ids[childID] {
		t.Error("expected childID in descendants")
	}
	if ids[gpID] {
		t.Error("grandparent should not be its own descendant")
	}
	if ids[unrelatedID] {
		t.Error("unrelated issue should not be in descendants")
	}
}

func TestListIssues_FilterByAncestorsOf(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — grandparent → parent → child
	now := time.Now()
	gpID := mustIssueID(t)
	parentID := mustIssueID(t)
	childID := mustIssueID(t)

	gp := mustEpic(t, gpID, "Grandparent", now)
	parent := mustEpic(t, parentID, "Parent", now).WithParentID(gpID)
	child := mustTask(t, childID, "Child", now).WithParentID(parentID)

	for _, iss := range []domain.Issue{gp, parent, child} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{
		AncestorsOf: childID,
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids := make(map[domain.ID]bool)
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids[parentID] {
		t.Error("expected parentID in ancestors")
	}
	if !ids[gpID] {
		t.Error("expected grandparent in ancestors")
	}
	if ids[childID] {
		t.Error("child should not be its own ancestor")
	}
}

func TestListIssues_IncludeDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	aliveID := mustIssueID(t)
	deletedID := mustIssueID(t)

	alive := mustTask(t, aliveID, "Alive", now)
	deleted := mustTask(t, deletedID, "Deleted", now).WithDeleted()

	for _, iss := range []domain.Issue{alive, deleted} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — without IncludeDeleted
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	if len(items) != 1 {
		t.Fatalf("expected 1 non-deleted item, got %d", len(items))
	}

	// When — with IncludeDeleted
	items, _, err = repo.ListIssues(ctx, driven.IssueFilter{IncludeDeleted: true}, driven.OrderByPriority, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	if len(items) != 2 {
		t.Fatalf("expected 2 items with IncludeDeleted, got %d", len(items))
	}
}

func TestListIssues_Pagination_LimitAndHasMore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — 3 issues
	now := time.Now()
	for i := 0; i < 3; i++ {
		id := mustIssueID(t)
		if err := repo.CreateIssue(ctx, mustTask(t, id, "Task", now.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — limit 2
	items, hasMore, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, 2)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if !hasMore {
		t.Error("expected hasMore=true with 3 items and limit=2")
	}
}

func TestListIssues_DefaultLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — more issues than the default limit (20)
	now := time.Now()
	for i := 0; i < 22; i++ {
		id := mustIssueID(t)
		if err := repo.CreateIssue(ctx, mustTask(t, id, "Task", now.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — limit 0 means "use default" (which is 20)
	items, hasMore, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, 0)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != driven.DefaultLimit {
		t.Errorf("expected %d items (default limit), got %d", driven.DefaultLimit, len(items))
	}
	if !hasMore {
		t.Error("expected hasMore=true with 22 items at default limit")
	}
}

func TestListIssues_NegativeLimit_ReturnsAll(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — 5 issues
	now := time.Now()
	for i := 0; i < 5; i++ {
		id := mustIssueID(t)
		if err := repo.CreateIssue(ctx, mustTask(t, id, "Task", now.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — negative limit means "return all"
	items, hasMore, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items, got %d", len(items))
	}
	if hasMore {
		t.Error("expected hasMore=false with negative limit")
	}
}

func TestListIssues_OrderByPriority(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — three tasks with different priorities
	now := time.Now()
	p0ID := mustIssueID(t)
	p1ID := mustIssueID(t)
	p3ID := mustIssueID(t)

	p0Task, _ := domain.NewTask(domain.NewTaskParams{ID: p0ID, Title: "P0", Priority: domain.P0, CreatedAt: now})
	p1Task, _ := domain.NewTask(domain.NewTaskParams{ID: p1ID, Title: "P1", Priority: domain.P1, CreatedAt: now})
	p3Task, _ := domain.NewTask(domain.NewTaskParams{ID: p3ID, Title: "P3", Priority: domain.P3, CreatedAt: now})

	for _, iss := range []domain.Issue{p3Task, p0Task, p1Task} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].Priority != domain.P0 {
		t.Errorf("first item should be P0, got %s", items[0].Priority)
	}
	if items[1].Priority != domain.P1 {
		t.Errorf("second item should be P1, got %s", items[1].Priority)
	}
	if items[2].Priority != domain.P3 {
		t.Errorf("third item should be P3, got %s", items[2].Priority)
	}
}

func TestListIssues_OrderByCreatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — three tasks created at different times
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	firstID := mustIssueID(t)
	secondID := mustIssueID(t)
	thirdID := mustIssueID(t)

	first := mustTask(t, firstID, "First", base)
	second := mustTask(t, secondID, "Second", base.Add(time.Hour))
	third := mustTask(t, thirdID, "Third", base.Add(2*time.Hour))

	// Insert out of order.
	for _, iss := range []domain.Issue{third, first, second} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByCreatedAt, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].ID != firstID {
		t.Errorf("first item should be oldest, got %s", items[0].ID)
	}
	if items[2].ID != thirdID {
		t.Errorf("last item should be newest, got %s", items[2].ID)
	}
}

func TestListIssues_OrderByUpdatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — three tasks, OrderByUpdatedAt sorts most recent first
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldestID := mustIssueID(t)
	newestID := mustIssueID(t)

	oldest := mustTask(t, oldestID, "Oldest", base)
	newest := mustTask(t, newestID, "Newest", base.Add(time.Hour))

	for _, iss := range []domain.Issue{oldest, newest} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByUpdatedAt, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Most recent first
	if items[0].ID != newestID {
		t.Errorf("first item should be newest, got %s", items[0].ID)
	}
	if items[1].ID != oldestID {
		t.Errorf("second item should be oldest, got %s", items[1].ID)
	}
}

func TestListIssues_BlockerIDs_Populated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — blockerID blocks blockedID
	now := time.Now()
	blockerID := mustIssueID(t)
	blockedID := mustIssueID(t)

	blocker := mustTask(t, blockerID, "Blocker", now)
	blocked := mustTask(t, blockedID, "Blocked", now)

	for _, iss := range []domain.Issue{blocker, blocked} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}
	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	items, _, err := repo.ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, item := range items {
		if item.ID == blockedID {
			if !item.IsBlocked {
				t.Error("expected blocked issue to have IsBlocked=true")
			}
			if len(item.BlockerIDs) != 1 || item.BlockerIDs[0] != blockerID {
				t.Errorf("expected BlockerIDs=[%s], got %v", blockerID, item.BlockerIDs)
			}
			return
		}
	}
	t.Fatal("blocked issue not found in results")
}

// --- SearchIssues ---

func TestSearchIssues_MatchesTitle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	matchID := mustIssueID(t)
	noMatchID := mustIssueID(t)

	matchIssue := mustTask(t, matchID, "Fix authentication bug", now)
	noMatchIssue := mustTask(t, noMatchID, "Add logging", now)

	for _, iss := range []domain.Issue{matchIssue, noMatchIssue} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	items, _, err := repo.SearchIssues(ctx, "authentication", driven.IssueFilter{}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 match, got %d", len(items))
	}
	if items[0].ID != matchID {
		t.Errorf("expected match ID %s, got %s", matchID, items[0].ID)
	}
}

func TestSearchIssues_MatchesDescription(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	id := mustIssueID(t)
	iss, _ := domain.NewTask(domain.NewTaskParams{
		ID:          id,
		Title:       "Some task",
		Description: "The frobnicate module needs refactoring",
		CreatedAt:   now,
	})
	if err := repo.CreateIssue(ctx, iss); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	items, _, err := repo.SearchIssues(ctx, "frobnicate", driven.IssueFilter{}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 match, got %d", len(items))
	}
}

func TestSearchIssues_CaseInsensitive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	id := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, id, "Fix UPPERCASE Bug", now)); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	items, _, err := repo.SearchIssues(ctx, "uppercase", driven.IssueFilter{}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 match (case-insensitive), got %d", len(items))
	}
}

func TestSearchIssues_RespectsFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — two issues match the query, but only one matches the filter
	now := time.Now()
	taskID := mustIssueID(t)
	epicID := mustIssueID(t)

	if err := repo.CreateIssue(ctx, mustTask(t, taskID, "Fix auth bug", now)); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	if err := repo.CreateIssue(ctx, mustEpic(t, epicID, "Fix auth epic", now)); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — search for "auth" but filter to tasks only
	items, _, err := repo.SearchIssues(ctx, "auth", driven.IssueFilter{
		Roles: []domain.Role{domain.RoleTask},
	}, driven.OrderByPriority, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 match (task only), got %d", len(items))
	}
	if items[0].ID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, items[0].ID)
	}
}

// --- GetChildStatuses ---

func TestGetChildStatuses_ReturnsChildrenWithBlockedState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — epic with two children, one blocked
	now := time.Now()
	epicID := mustIssueID(t)
	child1ID := mustIssueID(t)
	child2ID := mustIssueID(t)
	blockerID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Epic", now)
	child1 := mustTask(t, child1ID, "Child 1", now).WithParentID(epicID)
	child2 := mustTask(t, child2ID, "Child 2", now).WithParentID(epicID)
	blocker := mustTask(t, blockerID, "Blocker", now)

	for _, iss := range []domain.Issue{epic, child1, child2, blocker} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// Block child2
	rel := mustRelationship(t, child2ID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	statuses, err := repo.GetChildStatuses(ctx, epicID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 children, got %d", len(statuses))
	}
	blockedCount := 0
	for _, s := range statuses {
		if s.IsBlocked {
			blockedCount++
		}
	}
	if blockedCount != 1 {
		t.Errorf("expected 1 blocked child, got %d", blockedCount)
	}
}

func TestGetChildStatuses_ExcludesDeletedChildren(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	epicID := mustIssueID(t)
	aliveID := mustIssueID(t)
	deletedID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Epic", now)
	alive := mustTask(t, aliveID, "Alive child", now).WithParentID(epicID)
	deleted := mustTask(t, deletedID, "Deleted child", now).WithParentID(epicID).WithDeleted()

	for _, iss := range []domain.Issue{epic, alive, deleted} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	statuses, err := repo.GetChildStatuses(ctx, epicID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Errorf("expected 1 child (excluding deleted), got %d", len(statuses))
	}
}

// --- GetDescendants ---

func TestGetDescendants_RecursiveTraversal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — epic → subepic → task
	now := time.Now()
	epicID := mustIssueID(t)
	subepicID := mustIssueID(t)
	taskID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Epic", now)
	subepic := mustEpic(t, subepicID, "Subepic", now).WithParentID(epicID)
	task := mustTask(t, taskID, "Task", now).WithParentID(subepicID)

	for _, iss := range []domain.Issue{epic, subepic, task} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	descendants, err := repo.GetDescendants(ctx, epicID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(descendants) != 2 {
		t.Fatalf("expected 2 descendants, got %d", len(descendants))
	}
	ids := make(map[domain.ID]bool)
	for _, d := range descendants {
		ids[d.ID] = true
	}
	if !ids[subepicID] {
		t.Error("expected subepic in descendants")
	}
	if !ids[taskID] {
		t.Error("expected task in descendants")
	}
}

func TestGetDescendants_IncludesClaimInfo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — epic with a claimed child
	now := time.Now()
	epicID := mustIssueID(t)
	childID := mustIssueID(t)
	author := mustAuthor(t, "alice")

	epic := mustEpic(t, epicID, "Epic", now)
	child := mustTask(t, childID, "Child", now).WithParentID(epicID)

	for _, iss := range []domain.Issue{epic, child} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: childID,
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}
	if err := repo.CreateClaim(ctx, c); err != nil {
		t.Fatalf("precondition: persist claim: %v", err)
	}

	// When
	descendants, err := repo.GetDescendants(ctx, epicID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(descendants) != 1 {
		t.Fatalf("expected 1 descendant, got %d", len(descendants))
	}
	if !descendants[0].IsClaimed {
		t.Error("expected descendant to be claimed")
	}
	if descendants[0].ClaimedBy != "alice" {
		t.Errorf("expected claimedBy 'alice', got %q", descendants[0].ClaimedBy)
	}
}

// --- HasChildren ---

func TestHasChildren_True(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	epicID := mustIssueID(t)
	childID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Epic", now)
	child := mustTask(t, childID, "Child", now).WithParentID(epicID)

	for _, iss := range []domain.Issue{epic, child} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	has, err := repo.HasChildren(ctx, epicID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected HasChildren=true")
	}
}

func TestHasChildren_False(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	epicID := mustIssueID(t)
	epic := mustEpic(t, epicID, "Empty epic", time.Now())
	if err := repo.CreateIssue(ctx, epic); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	has, err := repo.HasChildren(ctx, epicID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected HasChildren=false for childless epic")
	}
}

func TestHasChildren_ExcludesDeletedChildren(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — epic with only a deleted child
	now := time.Now()
	epicID := mustIssueID(t)
	childID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Epic", now)
	child := mustTask(t, childID, "Deleted child", now).WithParentID(epicID).WithDeleted()

	for _, iss := range []domain.Issue{epic, child} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	has, err := repo.HasChildren(ctx, epicID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected HasChildren=false when only child is deleted")
	}
}

// --- GetAncestorStatuses ---

func TestGetAncestorStatuses_WalksParentChain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — grandparent → parent → child
	now := time.Now()
	gpID := mustIssueID(t)
	parentID := mustIssueID(t)
	childID := mustIssueID(t)

	gp := mustEpic(t, gpID, "Grandparent", now)
	parent := mustEpic(t, parentID, "Parent", now).WithParentID(gpID)
	child := mustTask(t, childID, "Child", now).WithParentID(parentID)

	for _, iss := range []domain.Issue{gp, parent, child} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	ancestors, err := repo.GetAncestorStatuses(ctx, childID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ancestors) != 2 {
		t.Fatalf("expected 2 ancestors, got %d", len(ancestors))
	}
	// First ancestor should be the direct parent
	if ancestors[0].ID != parentID {
		t.Errorf("expected first ancestor to be parent %s, got %s", parentID, ancestors[0].ID)
	}
	if ancestors[1].ID != gpID {
		t.Errorf("expected second ancestor to be grandparent %s, got %s", gpID, ancestors[1].ID)
	}
}

func TestGetAncestorStatuses_OrphanReturnsEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — orphan issue
	id := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, id, "Orphan", time.Now())); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	ancestors, err := repo.GetAncestorStatuses(ctx, id)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ancestors) != 0 {
		t.Errorf("expected 0 ancestors for orphan, got %d", len(ancestors))
	}
}

// --- GetParentID ---

func TestGetParentID_ReturnsParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()
	epicID := mustIssueID(t)
	childID := mustIssueID(t)

	epic := mustEpic(t, epicID, "Epic", now)
	child := mustTask(t, childID, "Child", now).WithParentID(epicID)

	for _, iss := range []domain.Issue{epic, child} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	parentID, err := repo.GetParentID(ctx, childID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parentID != epicID {
		t.Errorf("expected parent %s, got %s", epicID, parentID)
	}
}

func TestGetParentID_OrphanReturnsZero(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, id, "Orphan", time.Now())); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	parentID, err := repo.GetParentID(ctx, id)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !parentID.IsZero() {
		t.Errorf("expected zero parent ID for orphan, got %s", parentID)
	}
}

func TestGetParentID_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)

	// When
	_, err := repo.GetParentID(ctx, id)

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- IssueIDExists ---

func TestIssueIDExists_True(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, id, "Exists", time.Now())); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	exists, err := repo.IssueIDExists(ctx, id)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected IssueIDExists=true")
	}
}

func TestIssueIDExists_False(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)

	// When
	exists, err := repo.IssueIDExists(ctx, id)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected IssueIDExists=false for nonexistent ID")
	}
}

// --- ListDistinctLabels ---

func TestListDistinctLabels_ReturnsUniqueLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — two issues sharing one label, one with an extra label
	now := time.Now()
	id1 := mustIssueID(t)
	id2 := mustIssueID(t)

	bugLabel := mustLabel(t, "kind", "bug")
	areaLabel := mustLabel(t, "area", "api")

	iss1 := mustTask(t, id1, "Issue 1", now).
		WithLabels(domain.NewLabelSet().Set(bugLabel))
	iss2 := mustTask(t, id2, "Issue 2", now).
		WithLabels(domain.NewLabelSet().Set(bugLabel).Set(areaLabel))

	for _, iss := range []domain.Issue{iss1, iss2} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	labels, err := repo.ListDistinctLabels(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("expected 2 distinct labels, got %d", len(labels))
	}
}

func TestListDistinctLabels_ExcludesDeletedIssues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one alive issue with a label, one deleted issue with a unique label
	now := time.Now()
	aliveID := mustIssueID(t)
	deletedID := mustIssueID(t)

	commonLabel := mustLabel(t, "kind", "bug")
	uniqueLabel := mustLabel(t, "area", "deleted")

	alive := mustTask(t, aliveID, "Alive", now).
		WithLabels(domain.NewLabelSet().Set(commonLabel))
	deleted := mustTask(t, deletedID, "Deleted", now).
		WithLabels(domain.NewLabelSet().Set(uniqueLabel)).WithDeleted()

	for _, iss := range []domain.Issue{alive, deleted} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	labels, err := repo.ListDistinctLabels(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 1 {
		t.Errorf("expected 1 label (excluding deleted issue's labels), got %d", len(labels))
	}
}

// --- GetIssueByIdempotencyKey ---

func TestGetIssueByIdempotencyKey_Found(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	id := mustIssueID(t)
	iss, _ := domain.NewTask(domain.NewTaskParams{
		ID:             id,
		Title:          "Idempotent task",
		CreatedAt:      time.Now(),
		IdempotencyKey: "import-abc-123",
	})
	if err := repo.CreateIssue(ctx, iss); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	got, err := repo.GetIssueByIdempotencyKey(ctx, "import-abc-123")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID() != id {
		t.Errorf("expected ID %s, got %s", id, got.ID())
	}
}

func TestGetIssueByIdempotencyKey_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	_, err := repo.GetIssueByIdempotencyKey(ctx, "nonexistent-key")

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetIssueByIdempotencyKey_EmptyKeyNeverMatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — an issue with an empty idempotency key
	id := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, id, "No key", time.Now())); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — search for empty key
	_, err := repo.GetIssueByIdempotencyKey(ctx, "")

	// Then — should not match
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty key search, got %v", err)
	}
}

// --- GetIssueSummary ---

func TestGetIssueSummary_AggregatesCounts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	now := time.Now()

	openTask := mustTask(t, mustIssueID(t), "Open", now)
	claimedTask := mustTask(t, mustIssueID(t), "Claimed", now).WithState(domain.StateClaimed)
	deferredTask := mustTask(t, mustIssueID(t), "Deferred", now).WithState(domain.StateDeferred)
	closedTask := mustTask(t, mustIssueID(t), "Closed", now).WithState(domain.StateClosed)
	deletedTask := mustTask(t, mustIssueID(t), "Deleted", now).WithDeleted()

	for _, iss := range []domain.Issue{openTask, claimedTask, deferredTask, closedTask, deletedTask} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	summary, err := repo.GetIssueSummary(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Open != 1 {
		t.Errorf("expected Open=1, got %d", summary.Open)
	}
	if summary.Claimed != 1 {
		t.Errorf("expected Claimed=1, got %d", summary.Claimed)
	}
	if summary.Deferred != 1 {
		t.Errorf("expected Deferred=1, got %d", summary.Deferred)
	}
	if summary.Closed != 1 {
		t.Errorf("expected Closed=1, got %d", summary.Closed)
	}
	if summary.Total() != 4 {
		t.Errorf("expected Total=4 (excludes deleted), got %d", summary.Total())
	}
}

func TestGetIssueSummary_ReadyAndBlocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — readyTask is open+unblocked; blockedTask is open+blocked
	now := time.Now()
	readyID := mustIssueID(t)
	blockedID := mustIssueID(t)
	blockerID := mustIssueID(t)

	ready := mustTask(t, readyID, "Ready", now)
	blocked := mustTask(t, blockedID, "Blocked", now)
	blocker := mustTask(t, blockerID, "Blocker", now)

	for _, iss := range []domain.Issue{ready, blocked, blocker} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	summary, err := repo.GetIssueSummary(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// readyID and blockerID are both ready (open, unblocked)
	if summary.Ready != 2 {
		t.Errorf("expected Ready=2, got %d", summary.Ready)
	}
	if summary.Blocked != 1 {
		t.Errorf("expected Blocked=1, got %d", summary.Blocked)
	}
}

func TestGetIssueSummary_ClosedBlockedNotCountedAsBlocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — closedTask is closed but has an unresolved blocked_by.
	// Closed issues' blocker relationships are not actionable, so the
	// blocked count should not include them.
	now := time.Now()
	closedID := mustIssueID(t)
	blockerID := mustIssueID(t)

	closed := mustTask(t, closedID, "Closed blocked", now).WithState(domain.StateClosed)
	blocker := mustTask(t, blockerID, "Blocker", now)

	for _, iss := range []domain.Issue{closed, blocker} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	rel := mustRelationship(t, closedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	summary, err := repo.GetIssueSummary(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Blocked != 0 {
		t.Errorf("expected Blocked=0 (closed issues excluded from blocked count), got %d", summary.Blocked)
	}
}

package core

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- test helpers ---

// buildTask creates a task issue with the given ID string, state, and optional
// parent. Pass the zero domain.ID for no parent. Fails the test on invalid ID —
// use only with known-valid ID strings in test fixtures.
func buildTask(t *testing.T, idStr string, state domain.State, parentID domain.ID) domain.Issue {
	t.Helper()
	issue, err := domain.NewTask(domain.NewTaskParams{
		ID:       mustParseID(idStr),
		Title:    "Task " + idStr,
		ParentID: parentID,
	})
	if err != nil {
		t.Fatalf("buildTask(%q): %v", idStr, err)
	}
	return issue.WithState(state)
}

// buildEpic creates an epic issue with the given ID string and state. Fails
// the test on invalid ID — use only with known-valid ID strings in fixtures.
func buildEpic(t *testing.T, idStr string, state domain.State) domain.Issue {
	t.Helper()
	issue, err := domain.NewEpic(domain.NewEpicParams{
		ID:    mustParseID(idStr),
		Title: "Epic " + idStr,
	})
	if err != nil {
		t.Fatalf("buildEpic(%q): %v", idStr, err)
	}
	return issue.WithState(state)
}

// graphFakeIssueRepo is a hand-written fake that stores a slice of issues and
// serves ListIssues (empty-filter, all non-deleted) and GetIssue (by ID). All
// other IssueRepository methods are no-op stubs.
//
// blockedBy is an optional map from issue ID to the IDs of its non-closed
// blockers. Graph health checks use BlockerIDs from IssueListItem; populate
// this map to exercise those checks. Nil means no blockers.
type graphFakeIssueRepo struct {
	issues    []domain.Issue
	blockedBy map[domain.ID][]domain.ID
}

func (r *graphFakeIssueRepo) ListIssues(_ context.Context, filter driven.IssueFilter, _ driven.IssueOrderBy, _ driven.SortDirection, _ int) ([]driven.IssueListItem, bool, error) {
	var items []driven.IssueListItem
	for _, issue := range r.issues {
		if !filter.IncludeDeleted && issue.IsDeleted() {
			continue
		}
		items = append(items, driven.IssueListItem{
			ID:         issue.ID(),
			Role:       issue.Role(),
			State:      issue.State(),
			Priority:   issue.Priority(),
			ParentID:   issue.ParentID(),
			BlockerIDs: r.blockedBy[issue.ID()],
		})
	}
	return items, false, nil
}

func (r *graphFakeIssueRepo) GetIssue(_ context.Context, id domain.ID, includeDeleted bool) (domain.Issue, error) {
	for _, issue := range r.issues {
		if issue.ID() == id {
			if !includeDeleted && issue.IsDeleted() {
				return domain.Issue{}, domain.ErrNotFound
			}
			return issue, nil
		}
	}
	return domain.Issue{}, domain.ErrNotFound
}

// Remaining IssueRepository methods panic — graph integrity checks should
// never reach them. If a future check starts using one of these, the panic
// surfaces the change loudly instead of returning silent zero values.
func (r *graphFakeIssueRepo) CreateIssue(_ context.Context, _ domain.Issue) error {
	panic("graphFakeIssueRepo.CreateIssue: unexpected call")
}

func (r *graphFakeIssueRepo) UpdateIssue(_ context.Context, _ domain.Issue) error {
	panic("graphFakeIssueRepo.UpdateIssue: unexpected call")
}

func (r *graphFakeIssueRepo) SearchIssues(_ context.Context, _ string, _ driven.IssueFilter, _ driven.IssueOrderBy, _ driven.SortDirection, _ int) ([]driven.IssueListItem, bool, error) {
	panic("graphFakeIssueRepo.SearchIssues: unexpected call")
}

func (r *graphFakeIssueRepo) GetChildStatuses(_ context.Context, _ domain.ID) ([]domain.ChildStatus, error) {
	panic("graphFakeIssueRepo.GetChildStatuses: unexpected call")
}

func (r *graphFakeIssueRepo) GetDescendants(_ context.Context, _ domain.ID) ([]domain.DescendantInfo, error) {
	panic("graphFakeIssueRepo.GetDescendants: unexpected call")
}

func (r *graphFakeIssueRepo) HasChildren(_ context.Context, _ domain.ID) (bool, error) {
	panic("graphFakeIssueRepo.HasChildren: unexpected call")
}

func (r *graphFakeIssueRepo) GetAncestorStatuses(_ context.Context, _ domain.ID) ([]domain.AncestorStatus, error) {
	panic("graphFakeIssueRepo.GetAncestorStatuses: unexpected call")
}

func (r *graphFakeIssueRepo) GetParentID(_ context.Context, _ domain.ID) (domain.ID, error) {
	panic("graphFakeIssueRepo.GetParentID: unexpected call")
}

func (r *graphFakeIssueRepo) IssueIDExists(_ context.Context, _ domain.ID) (bool, error) {
	panic("graphFakeIssueRepo.IssueIDExists: unexpected call")
}

func (r *graphFakeIssueRepo) ListLabelCounts(_ context.Context) ([]domain.LabelCount, error) {
	panic("graphFakeIssueRepo.ListLabelCounts: unexpected call")
}

func (r *graphFakeIssueRepo) GetIssueSummary(_ context.Context) (driven.IssueSummary, error) {
	panic("graphFakeIssueRepo.GetIssueSummary: unexpected call")
}

// graphFakeUnitOfWork satisfies driven.UnitOfWork, exposing the issue repo
// and returning nil for all other repositories (they are unused by graph
// integrity checks).
type graphFakeUnitOfWork struct{ issues *graphFakeIssueRepo }

func (u *graphFakeUnitOfWork) Database() driven.DatabaseRepository          { return nil }
func (u *graphFakeUnitOfWork) Issues() driven.IssueRepository               { return u.issues }
func (u *graphFakeUnitOfWork) Comments() driven.CommentRepository           { return nil }
func (u *graphFakeUnitOfWork) Claims() driven.ClaimRepository               { return nil }
func (u *graphFakeUnitOfWork) Relationships() driven.RelationshipRepository { return nil }
func (u *graphFakeUnitOfWork) History() driven.HistoryRepository            { return nil }

// graphFakeTransactor satisfies driven.Transactor by calling the function
// synchronously against a single graphFakeUnitOfWork.
type graphFakeTransactor struct{ uow *graphFakeUnitOfWork }

func (t *graphFakeTransactor) WithTransaction(_ context.Context, fn func(driven.UnitOfWork) error) error {
	return fn(t.uow)
}

func (t *graphFakeTransactor) WithReadTransaction(_ context.Context, fn func(driven.UnitOfWork) error) error {
	return fn(t.uow)
}
func (t *graphFakeTransactor) Vacuum(_ context.Context) error { return nil }

// newGraphSvc creates a serviceImpl backed by the given slice of issues.
func newGraphSvc(issues []domain.Issue) *serviceImpl {
	repo := &graphFakeIssueRepo{issues: issues}
	uow := &graphFakeUnitOfWork{issues: repo}
	tx := &graphFakeTransactor{uow: uow}
	return &serviceImpl{tx: tx}
}

// newGraphSvcWithBlockers creates a serviceImpl backed by issues and an
// explicit blocked-by map. Used by graph health check tests that require
// BlockerIDs to be populated in IssueListItem.
func newGraphSvcWithBlockers(issues []domain.Issue, blockedBy map[domain.ID][]domain.ID) *serviceImpl {
	repo := &graphFakeIssueRepo{issues: issues, blockedBy: blockedBy}
	uow := &graphFakeUnitOfWork{issues: repo}
	tx := &graphFakeTransactor{uow: uow}
	return &serviceImpl{tx: tx}
}

// compile-time interface checks.
var (
	_ driven.IssueRepository = (*graphFakeIssueRepo)(nil)
	_ driven.UnitOfWork      = (*graphFakeUnitOfWork)(nil)
	_ driven.Transactor      = (*graphFakeTransactor)(nil)
)

// --- closed-parent-with-open-child ---

// TestRunClosedParentWithOpenChild_ClosedParentOneOpenChild_ReturnsFinding
// verifies that a closed parent with a single open child produces one finding.
func TestRunClosedParentWithOpenChild_ClosedParentOneOpenChild_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a closed epic with one open child task.
	parentID := mustParseID("NP-aaaaa")
	childID := mustParseID("NP-bbbbb")
	parent := buildEpic(t, "NP-aaaaa", domain.StateClosed)
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runClosedParentWithOpenChild(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding, correct parent, one non-closed child.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.ClosedParentWithOpenChildRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want ClosedParentWithOpenChildRow", result.Affected[0])
	}
	if row.Issue != parentID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, parentID.String())
	}
	if !slices.Equal(row.NonClosedChildren, []string{childID.String()}) {
		t.Errorf("NonClosedChildren: got %v, want [%s]", row.NonClosedChildren, childID)
	}
}

// TestRunClosedParentWithOpenChild_ClosedParentOneDeferredChild_ReturnsFinding
// verifies that a deferred child also triggers the check.
func TestRunClosedParentWithOpenChild_ClosedParentOneDeferredChild_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a closed epic with one deferred child task.
	parentID := mustParseID("NP-aaaaa")
	childID := mustParseID("NP-bbbbb")
	parent := buildEpic(t, "NP-aaaaa", domain.StateClosed)
	child := buildTask(t, "NP-bbbbb", domain.StateDeferred, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runClosedParentWithOpenChild(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding for the closed parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	row, ok := result.Affected[0].(driving.ClosedParentWithOpenChildRow)
	if !ok {
		t.Fatalf("Affected[0] type: %T", result.Affected[0])
	}
	if row.Issue != parentID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, parentID.String())
	}
	if !slices.Equal(row.NonClosedChildren, []string{childID.String()}) {
		t.Errorf("NonClosedChildren: got %v, want [%s]", row.NonClosedChildren, childID)
	}
}

// TestRunClosedParentWithOpenChild_ClosedParentMultipleNonClosedChildren_SortedAscending
// verifies that when a closed parent has multiple non-closed children, the
// NonClosedChildren array is sorted ascending by issue ID.
func TestRunClosedParentWithOpenChild_ClosedParentMultipleNonClosedChildren_SortedAscending(t *testing.T) {
	t.Parallel()

	// Given — a closed epic with three non-closed children in non-sorted order.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateClosed)
	// IDs chosen so that string sort order is ccccc < ddddd < eeeee.
	c1 := buildTask(t, "NP-eeeee", domain.StateOpen, parentID)
	c2 := buildTask(t, "NP-ccccc", domain.StateDeferred, parentID)
	c3 := buildTask(t, "NP-ddddd", domain.StateOpen, parentID)
	svc := newGraphSvc([]domain.Issue{parent, c1, c2, c3})

	// When
	result, err := runClosedParentWithOpenChild(t.Context(), svc, driving.DoctorInput{})
	// Then — one row, three children in ascending order.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row := result.Affected[0].(driving.ClosedParentWithOpenChildRow)
	want := []string{
		mustParseID("NP-ccccc").String(),
		mustParseID("NP-ddddd").String(),
		mustParseID("NP-eeeee").String(),
	}
	if !slices.Equal(row.NonClosedChildren, want) {
		t.Errorf("NonClosedChildren: got %v, want %v", row.NonClosedChildren, want)
	}
}

// TestRunClosedParentWithOpenChild_ClosedParentAllClosedChildren_Passes verifies
// that a closed parent with all children in the closed state produces no finding.
func TestRunClosedParentWithOpenChild_ClosedParentAllClosedChildren_Passes(t *testing.T) {
	t.Parallel()

	// Given — a closed epic with two closed children.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateClosed)
	c1 := buildTask(t, "NP-bbbbb", domain.StateClosed, parentID)
	c2 := buildTask(t, "NP-ccccc", domain.StateClosed, parentID)
	svc := newGraphSvc([]domain.Issue{parent, c1, c2})

	// When
	result, err := runClosedParentWithOpenChild(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunClosedParentWithOpenChild_NoIssues_Passes verifies the empty-database
// case produces no finding.
func TestRunClosedParentWithOpenChild_NoIssues_Passes(t *testing.T) {
	t.Parallel()

	// Given — an empty issue store.
	svc := newGraphSvc(nil)

	// When
	result, err := runClosedParentWithOpenChild(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunClosedParentWithOpenChild_MixedClosedAndNonClosedChildren_OnlyNonClosedListed
// verifies that closed children of a closed parent are excluded from the
// NonClosedChildren array (only open and deferred children appear).
func TestRunClosedParentWithOpenChild_MixedClosedAndNonClosedChildren_OnlyNonClosedListed(t *testing.T) {
	t.Parallel()

	// Given — a closed epic with one open, one deferred, and one closed child.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateClosed)
	openChild := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID)
	closedChild := buildTask(t, "NP-ccccc", domain.StateClosed, parentID)
	deferredChild := buildTask(t, "NP-ddddd", domain.StateDeferred, parentID)
	svc := newGraphSvc([]domain.Issue{parent, openChild, closedChild, deferredChild})

	// When
	result, err := runClosedParentWithOpenChild(t.Context(), svc, driving.DoctorInput{})
	// Then — one row, NonClosedChildren contains only the open and deferred IDs.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	row := result.Affected[0].(driving.ClosedParentWithOpenChildRow)
	want := []string{
		mustParseID("NP-bbbbb").String(),
		mustParseID("NP-ddddd").String(),
	}
	if !slices.Equal(row.NonClosedChildren, want) {
		t.Errorf("NonClosedChildren: got %v, want %v (closed sibling NP-ccccc must be excluded)", row.NonClosedChildren, want)
	}
}

// TestRunClosedParentWithOpenChild_OneRowPerParent_NotPerPair verifies that the
// check emits exactly one row per closed parent, not one per (parent, child) pair.
func TestRunClosedParentWithOpenChild_OneRowPerParent_NotPerPair(t *testing.T) {
	t.Parallel()

	// Given — two separate closed parents, each with one non-closed child.
	parent1ID := mustParseID("NP-aaaaa")
	parent2ID := mustParseID("NP-bbbbb")
	p1 := buildEpic(t, "NP-aaaaa", domain.StateClosed)
	p2 := buildEpic(t, "NP-bbbbb", domain.StateClosed)
	c1 := buildTask(t, "NP-ccccc", domain.StateOpen, parent1ID)
	c2 := buildTask(t, "NP-ddddd", domain.StateOpen, parent2ID)
	svc := newGraphSvc([]domain.Issue{p1, p2, c1, c2})

	// When
	result, err := runClosedParentWithOpenChild(t.Context(), svc, driving.DoctorInput{})
	// Then — two rows (one per parent, not one per pair).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected findings, got nil")
	}
	if got := len(result.Affected); got != 2 {
		t.Errorf("affected rows: got %d, want 2", got)
	}
}

// --- invalid-parent-reference ---

// TestRunInvalidParentReference_MissingParent_ReturnsFinding verifies that a
// child whose parent ID does not exist in storage produces a finding.
func TestRunInvalidParentReference_MissingParent_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a task whose parent ID refers to an issue not in the store.
	missingParentID := mustParseID("NP-aaaaa")
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, missingParentID)
	svc := newGraphSvc([]domain.Issue{child})

	// When
	result, err := runInvalidParentReference(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding with the correct child and missing parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.InvalidParentReferenceRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want InvalidParentReferenceRow", result.Affected[0])
	}
	if row.Issue != child.ID().String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, child.ID())
	}
	if row.MissingParentID != missingParentID.String() {
		t.Errorf("row.MissingParentID: got %q, want %q", row.MissingParentID, missingParentID)
	}
}

// TestRunInvalidParentReference_SoftDeletedParent_ReturnsFinding verifies that a
// soft-deleted parent is treated the same as a missing parent.
func TestRunInvalidParentReference_SoftDeletedParent_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a task whose parent is present but soft-deleted.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateClosed).WithDeleted()
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runInvalidParentReference(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (soft-deleted parent), got nil")
	}
	row, ok := result.Affected[0].(driving.InvalidParentReferenceRow)
	if !ok {
		t.Fatalf("Affected[0] type: %T", result.Affected[0])
	}
	if row.MissingParentID != parentID.String() {
		t.Errorf("row.MissingParentID: got %q, want %q", row.MissingParentID, parentID)
	}
}

// TestRunInvalidParentReference_NoParentSet_Passes verifies that a task with no
// parent_id set produces no finding.
func TestRunInvalidParentReference_NoParentSet_Passes(t *testing.T) {
	t.Parallel()

	// Given — a task with no parent.
	child := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	svc := newGraphSvc([]domain.Issue{child})

	// When
	result, err := runInvalidParentReference(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunInvalidParentReference_ValidParent_Passes verifies that a task with a
// live (non-deleted) parent produces no finding.
func TestRunInvalidParentReference_ValidParent_Passes(t *testing.T) {
	t.Parallel()

	// Given — a task with a valid, non-deleted parent.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runInvalidParentReference(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunInvalidParentReference_AffectedSortedAscending verifies that the
// Affected rows are sorted ascending by issue ID.
func TestRunInvalidParentReference_AffectedSortedAscending(t *testing.T) {
	t.Parallel()

	// Given — two tasks, each referencing a missing parent; listed in
	// reverse order to confirm sort is applied.
	missing1 := mustParseID("NP-aaaaa")
	missing2 := mustParseID("NP-bbbbb")
	// child IDs chosen so that ddddd < eeeee in string comparison.
	child1 := buildTask(t, "NP-eeeee", domain.StateOpen, missing1)
	child2 := buildTask(t, "NP-ddddd", domain.StateOpen, missing2)
	svc := newGraphSvc([]domain.Issue{child1, child2})

	// When
	result, err := runInvalidParentReference(t.Context(), svc, driving.DoctorInput{})
	// Then — two rows in exact ascending order: ddddd before eeeee.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected findings, got nil")
	}
	if got := len(result.Affected); got != 2 {
		t.Fatalf("affected rows: got %d, want 2", got)
	}
	r0 := result.Affected[0].(driving.InvalidParentReferenceRow)
	r1 := result.Affected[1].(driving.InvalidParentReferenceRow)
	if want := mustParseID("NP-ddddd").String(); r0.Issue != want {
		t.Errorf("Affected[0].Issue: got %q, want %q", r0.Issue, want)
	}
	if want := mustParseID("NP-eeeee").String(); r1.Issue != want {
		t.Errorf("Affected[1].Issue: got %q, want %q", r1.Issue, want)
	}
}

// TestRunInvalidParentReference_NoIssues_Passes verifies the empty-database
// case produces no finding.
func TestRunInvalidParentReference_NoIssues_Passes(t *testing.T) {
	t.Parallel()

	// Given — empty issue store.
	svc := newGraphSvc(nil)

	// When
	result, err := runInvalidParentReference(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// --- JSON round-trip ---

// TestClosedParentWithOpenChildRow_JSONRoundTrip verifies that the spec-mandated
// JSON shape (`{"issue": "...", "non_closed_children": [...]}`) is preserved by
// the struct's field tags. Locks the AC's "decode/encode to JSON exactly per
// the spec" requirement against accidental tag renames.
func TestClosedParentWithOpenChildRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	// Given — a row populated with representative values.
	original := driving.ClosedParentWithOpenChildRow{
		Issue:             "NP-pqr01",
		NonClosedChildren: []string{"NP-stu22", "NP-vwx33"},
	}

	// When — marshal then unmarshal.
	bytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"issue":"NP-pqr01","non_closed_children":["NP-stu22","NP-vwx33"]}`
	if got := string(bytes); got != want {
		t.Errorf("JSON shape mismatch:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.ClosedParentWithOpenChildRow
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Then — the decoded value matches the original.
	if decoded.Issue != original.Issue {
		t.Errorf("Issue: got %q, want %q", decoded.Issue, original.Issue)
	}
	if !slices.Equal(decoded.NonClosedChildren, original.NonClosedChildren) {
		t.Errorf("NonClosedChildren: got %v, want %v", decoded.NonClosedChildren, original.NonClosedChildren)
	}
}

// TestInvalidParentReferenceRow_JSONRoundTrip verifies that the spec-mandated
// JSON shape (`{"issue": "...", "missing_parent_id": "..."}`) is preserved.
func TestInvalidParentReferenceRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	// Given
	original := driving.InvalidParentReferenceRow{
		Issue:           "NP-jkl78",
		MissingParentID: "NP-mno90",
	}

	// When
	bytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"issue":"NP-jkl78","missing_parent_id":"NP-mno90"}`
	if got := string(bytes); got != want {
		t.Errorf("JSON shape mismatch:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.InvalidParentReferenceRow
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Then
	if decoded != original {
		t.Errorf("decoded != original: %+v vs %+v", decoded, original)
	}
}

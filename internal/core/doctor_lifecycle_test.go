package core

import (
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- closable-parent-issues ---

// TestRunClosableParentIssues_EpicAllChildrenClosed_ReturnsFinding verifies
// that an open epic whose only child is closed produces one finding row.
func TestRunClosableParentIssues_EpicAllChildrenClosed_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — an open epic with one closed child task.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	child := buildTask(t, "NP-bbbbb", domain.StateClosed, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding for the closable parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.ClosableParentIssueRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want ClosableParentIssueRow", result.Affected[0])
	}
	if row.Issue != parentID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, parentID)
	}
}

// TestRunClosableParentIssues_EpicOneOpenChild_Passes verifies that an open
// epic with at least one open child produces no finding.
func TestRunClosableParentIssues_EpicOneOpenChild_Passes(t *testing.T) {
	t.Parallel()

	// Given — an open epic with one open child and one closed child.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	openChild := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID)
	closedChild := buildTask(t, "NP-ccccc", domain.StateClosed, parentID)
	svc := newGraphSvc([]domain.Issue{parent, openChild, closedChild})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunClosableParentIssues_EpicOneDeferredChild_Passes verifies that an
// open epic with a deferred child (not closed) produces no finding — deferred
// is not closed, so the parent is not eligible for close-completed.
func TestRunClosableParentIssues_EpicOneDeferredChild_Passes(t *testing.T) {
	t.Parallel()

	// Given — an open epic with one deferred child and one closed child.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	deferredChild := buildTask(t, "NP-bbbbb", domain.StateDeferred, parentID)
	closedChild := buildTask(t, "NP-ccccc", domain.StateClosed, parentID)
	svc := newGraphSvc([]domain.Issue{parent, deferredChild, closedChild})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding (deferred is not closed).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunClosableParentIssues_ParentTaskAllChildrenClosed_ReturnsFinding verifies
// that a parent task (role=task, not epic) with all closed children is still
// detected as closable — the check is role-agnostic.
func TestRunClosableParentIssues_ParentTaskAllChildrenClosed_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — an open task acting as a parent, with one closed child task.
	parentID := mustParseID("NP-aaaaa")
	parent := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	child := buildTask(t, "NP-bbbbb", domain.StateClosed, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding for the parent task.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding for parent task with all-closed children, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.ClosableParentIssueRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want ClosableParentIssueRow", result.Affected[0])
	}
	if row.Issue != parentID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, parentID)
	}
}

// TestRunClosableParentIssues_EpicNoChildren_Passes verifies that an open epic
// with no children at all produces no finding — the gate "at least one child"
// prevents leaf issues from appearing as closable.
func TestRunClosableParentIssues_EpicNoChildren_Passes(t *testing.T) {
	t.Parallel()

	// Given — an open epic with zero children.
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	svc := newGraphSvc([]domain.Issue{parent})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunClosableParentIssues_MultipleClosableEpics_RowsSortedAscending verifies
// that when multiple parents are closable, they appear sorted ascending by ID.
func TestRunClosableParentIssues_MultipleClosableEpics_RowsSortedAscending(t *testing.T) {
	t.Parallel()

	// Given — two open epics (IDs NP-bbbbb and NP-aaaaa), each with a closed child.
	// NP-bbbbb sorts after NP-aaaaa, so rows should be [NP-aaaaa, NP-bbbbb].
	epicAID := mustParseID("NP-aaaaa")
	epicBID := mustParseID("NP-bbbbb")
	epicA := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	epicB := buildEpic(t, "NP-bbbbb", domain.StateOpen)
	childA := buildTask(t, "NP-ccccc", domain.StateClosed, epicAID)
	childB := buildTask(t, "NP-ddddd", domain.StateClosed, epicBID)
	svc := newGraphSvc([]domain.Issue{epicA, epicB, childA, childB})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — two rows sorted ascending by issue ID.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected findings, got nil")
	}
	if got := len(result.Affected); got != 2 {
		t.Fatalf("affected rows: got %d, want 2", got)
	}
	r0 := result.Affected[0].(driving.ClosableParentIssueRow)
	r1 := result.Affected[1].(driving.ClosableParentIssueRow)
	if r0.Issue != epicAID.String() {
		t.Errorf("row[0].Issue: got %q, want %q (ascending order)", r0.Issue, epicAID)
	}
	if r1.Issue != epicBID.String() {
		t.Errorf("row[1].Issue: got %q, want %q (ascending order)", r1.Issue, epicBID)
	}
}

// TestRunClosableParentIssues_ParentTaskClosable_FixIncludesTasks verifies that
// when any closable parent is a task, the registry FixFn produces the
// --include-tasks variant of the fix command.
func TestRunClosableParentIssues_ParentTaskClosable_FixIncludesTasks(t *testing.T) {
	t.Parallel()

	// Given — an open task (role=task) acting as a parent with a closed child,
	// and an open epic also closable. Because at least one closable parent is a
	// task, the fix must include --include-tasks.
	epicID := mustParseID("NP-aaaaa")
	taskParentID := mustParseID("NP-bbbbb")
	epicChild := buildTask(t, "NP-ccccc", domain.StateClosed, epicID)
	taskChild := buildTask(t, "NP-ddddd", domain.StateClosed, taskParentID)
	epic := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	taskParent := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	svc := newGraphSvc([]domain.Issue{epic, taskParent, epicChild, taskChild})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — two findings and Fix includes --include-tasks.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected findings, got nil")
	}
	if got := len(result.Affected); got != 2 {
		t.Fatalf("affected rows: got %d, want 2", got)
	}
	entry := findRegistryEntry(t, "closable-parent-issues")
	if entry.FixFn == nil {
		t.Fatal("closable-parent-issues FixFn is nil; expected a dynamic fix function")
	}
	fix := entry.FixFn(result)
	const want = "np epic close-completed --include-tasks"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// TestRunClosableParentIssues_OnlyEpicClosable_FixNoIncludeTasks verifies that
// when all closable parents are epics, the fix omits --include-tasks.
func TestRunClosableParentIssues_OnlyEpicClosable_FixNoIncludeTasks(t *testing.T) {
	t.Parallel()

	// Given — one open epic with a closed child; no task parent.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	child := buildTask(t, "NP-bbbbb", domain.StateClosed, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runClosableParentIssues(t.Context(), svc, driving.DoctorInput{})
	// Then — Fix omits --include-tasks.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	entry := findRegistryEntry(t, "closable-parent-issues")
	if entry.FixFn == nil {
		t.Fatal("closable-parent-issues FixFn is nil; expected a dynamic fix function")
	}
	fix := entry.FixFn(result)
	const want = "np epic close-completed"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// TestClosableParentIssueRow_JSONRoundTrip verifies the spec-mandated JSON shape.
func TestClosableParentIssueRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := driving.ClosableParentIssueRow{Issue: "NP-abc12"}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"issue":"NP-abc12"}`
	if got := string(b); got != want {
		t.Errorf("JSON shape:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.ClosableParentIssueRow
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("decoded != original: %+v vs %+v", decoded, original)
	}
}

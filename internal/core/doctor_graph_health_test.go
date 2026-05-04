package core

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- blocked-by-ancestor ---

// TestRunBlockedByAncestor_BlockedByParent_ReturnsFinding verifies that an issue
// blocked by its direct parent emits one finding row.
func TestRunBlockedByAncestor_BlockedByParent_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a child task that is blocked by its own parent.
	parentID := mustParseID("NP-aaaaa")
	childID := mustParseID("NP-bbbbb")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID)
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{parent, child},
		map[domain.ID][]domain.ID{childID: {parentID}},
	)

	// When
	result, err := runBlockedByAncestor(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding: child blocked by parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.BlockedByAncestorRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want BlockedByAncestorRow", result.Affected[0])
	}
	if row.Issue != childID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, childID)
	}
	if row.BlockingAncestor != parentID.String() {
		t.Errorf("row.BlockingAncestor: got %q, want %q", row.BlockingAncestor, parentID)
	}
}

// TestRunBlockedByAncestor_BlockedByGrandparent_TwoRows verifies that an issue
// blocked by both its parent and grandparent emits two separate rows that
// match the actual (issue, ancestor) pairs — not duplicate rows for the same
// ancestor.
func TestRunBlockedByAncestor_BlockedByGrandparent_TwoRows(t *testing.T) {
	t.Parallel()

	// Given — grandparent → parent → child; child is blocked by both grandparent
	// and parent.
	grandparentID := mustParseID("NP-aaaaa")
	parentID := mustParseID("NP-bbbbb")
	childID := mustParseID("NP-ccccc")
	grandparent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	parent := buildTask(t, "NP-bbbbb", domain.StateOpen, grandparentID)
	child := buildTask(t, "NP-ccccc", domain.StateOpen, parentID)
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{grandparent, parent, child},
		map[domain.ID][]domain.ID{
			childID: {grandparentID, parentID},
		},
	)

	// When
	result, err := runBlockedByAncestor(t.Context(), svc, driving.DoctorInput{})
	// Then — two rows for (child, grandparent) and (child, parent), sorted
	// ascending by BlockingAncestor (since Issue is identical).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected findings, got nil")
	}
	if got := len(result.Affected); got != 2 {
		t.Fatalf("affected rows: got %d, want 2", got)
	}
	r0 := result.Affected[0].(driving.BlockedByAncestorRow)
	r1 := result.Affected[1].(driving.BlockedByAncestorRow)
	if r0.Issue != childID.String() || r1.Issue != childID.String() {
		t.Errorf("rows must both reference child %q; got %q and %q", childID, r0.Issue, r1.Issue)
	}
	// grandparent (NP-aaaaa) sorts before parent (NP-bbbbb).
	if r0.BlockingAncestor != grandparentID.String() {
		t.Errorf("row[0].BlockingAncestor: got %q, want %q", r0.BlockingAncestor, grandparentID)
	}
	if r1.BlockingAncestor != parentID.String() {
		t.Errorf("row[1].BlockingAncestor: got %q, want %q", r1.BlockingAncestor, parentID)
	}
}

// TestRunBlockedByAncestor_BlockerNotAncestor_Passes verifies that an issue
// blocked by an unrelated issue emits no finding.
func TestRunBlockedByAncestor_BlockerNotAncestor_Passes(t *testing.T) {
	t.Parallel()

	// Given — child is blocked by an unrelated issue (not in its ancestor chain).
	unrelatedID := mustParseID("NP-aaaaa")
	parentID := mustParseID("NP-bbbbb")
	childID := mustParseID("NP-ccccc")
	unrelated := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	parent := buildEpic(t, "NP-bbbbb", domain.StateOpen)
	child := buildTask(t, "NP-ccccc", domain.StateOpen, parentID)
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{unrelated, parent, child},
		map[domain.ID][]domain.ID{childID: {unrelatedID}},
	)

	// When
	result, err := runBlockedByAncestor(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunBlockedByAncestor_NoBlockers_Passes verifies the no-blockers case.
func TestRunBlockedByAncestor_NoBlockers_Passes(t *testing.T) {
	t.Parallel()

	// Given — issues with parent relationships but no blocked_by.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runBlockedByAncestor(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunBlockedByAncestor_ParentCycle_DoesNotInfiniteLoop verifies that a
// corrupt parent chain (A's parent is B, B's parent is A) does not cause an
// infinite loop. The check should terminate gracefully even though the input
// data is invalid in another dimension.
func TestRunBlockedByAncestor_ParentCycle_DoesNotInfiniteLoop(t *testing.T) {
	t.Parallel()

	// Given — A and B are each other's parent (corrupt data); C is blocked by A.
	aID := mustParseID("NP-aaaaa")
	bID := mustParseID("NP-bbbbb")
	cID := mustParseID("NP-ccccc")
	a := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	a = a.WithParentID(bID)
	b := buildEpic(t, "NP-bbbbb", domain.StateOpen)
	b = b.WithParentID(aID)
	c := buildTask(t, "NP-ccccc", domain.StateOpen, aID)
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{a, b, c},
		map[domain.ID][]domain.ID{cID: {aID}},
	)

	// When — must terminate.
	result, err := runBlockedByAncestor(t.Context(), svc, driving.DoctorInput{})
	// Then — at least one finding (C blocked by ancestor A); must not hang.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (C blocked by ancestor A), got nil")
	}
}

// --- blocked-by-closable-issue ---

// TestRunBlockedByClosableIssue_ClosableEpicBlocker_ReturnsFinding verifies that
// an issue blocked by an epic whose children are all closed emits a finding.
func TestRunBlockedByClosableIssue_ClosableEpicBlocker_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — blockerEpic is open with one closed child; issue is blocked by it.
	blockerID := mustParseID("NP-aaaaa")
	blockedIssueID := mustParseID("NP-ccccc")
	blockerEpic := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	closedChild := buildTask(t, "NP-bbbbb", domain.StateClosed, blockerID)
	blockedIssue := buildTask(t, "NP-ccccc", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{blockerEpic, closedChild, blockedIssue},
		map[domain.ID][]domain.ID{blockedIssueID: {blockerID}},
	)

	// When
	result, err := runBlockedByClosableIssue(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.BlockedByClosableIssueRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want BlockedByClosableIssueRow", result.Affected[0])
	}
	if row.Issue != blockedIssueID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, blockedIssueID)
	}
	if row.ClosableBlocker != blockerID.String() {
		t.Errorf("row.ClosableBlocker: got %q, want %q", row.ClosableBlocker, blockerID)
	}
}

// TestRunBlockedByClosableIssue_ClosableTaskBlocker_FixIncludesTasks verifies
// that when any closable blocker is a task, the computed Fix includes --include-tasks.
func TestRunBlockedByClosableIssue_ClosableTaskBlocker_FixIncludesTasks(t *testing.T) {
	t.Parallel()

	// Given — blockerTask is an open task (role=task) with one closed child;
	// blockerEpic is an open epic with one closed child; blockedIssue is
	// blocked by both. Because at least one closable blocker is a task, the
	// fix must include --include-tasks.
	epicBlockerID := mustParseID("NP-aaaaa")
	taskBlockerID := mustParseID("NP-bbbbb")
	blockedIssueID := mustParseID("NP-eeeee")

	epicBlocker := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	taskBlocker := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	epicChild := buildTask(t, "NP-ccccc", domain.StateClosed, epicBlockerID)
	taskChild := buildTask(t, "NP-ddddd", domain.StateClosed, taskBlockerID)
	blockedIssue := buildTask(t, "NP-eeeee", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{epicBlocker, taskBlocker, epicChild, taskChild, blockedIssue},
		map[domain.ID][]domain.ID{
			blockedIssueID: {epicBlockerID, taskBlockerID},
		},
	)

	// When — run the check and build the registry Fix via FixFn.
	result, err := runBlockedByClosableIssue(t.Context(), svc, driving.DoctorInput{})
	// Then — two findings (one per closable blocker) and Fix includes --include-tasks.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected findings, got nil")
	}
	if got := len(result.Affected); got != 2 {
		t.Fatalf("affected rows: got %d, want 2 (one per closable blocker)", got)
	}
	// The registry entry's FixFn should produce --include-tasks.
	entry := findRegistryEntry(t, "blocked-by-closable-issue")
	if entry.FixFn == nil {
		t.Fatal("blocked-by-closable-issue FixFn is nil; expected a dynamic fix function")
	}
	fix := entry.FixFn(result)
	const want = "np epic close-completed --include-tasks"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// TestRunBlockedByClosableIssue_OnlyEpicBlockers_FixNoIncludeTasks verifies that
// when all closable blockers are epics, the fix omits --include-tasks.
func TestRunBlockedByClosableIssue_OnlyEpicBlockers_FixNoIncludeTasks(t *testing.T) {
	t.Parallel()

	// Given — blockerEpic is open with a closed child; no task blocker.
	epicBlockerID := mustParseID("NP-aaaaa")
	blockedIssueID := mustParseID("NP-ccccc")
	epicBlocker := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	epicChild := buildTask(t, "NP-bbbbb", domain.StateClosed, epicBlockerID)
	blockedIssue := buildTask(t, "NP-ccccc", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{epicBlocker, epicChild, blockedIssue},
		map[domain.ID][]domain.ID{blockedIssueID: {epicBlockerID}},
	)

	// When
	result, err := runBlockedByClosableIssue(t.Context(), svc, driving.DoctorInput{})
	// Then — Fix omits --include-tasks.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	entry := findRegistryEntry(t, "blocked-by-closable-issue")
	fix := entry.FixFn(result)
	const want = "np epic close-completed"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// TestRunBlockedByClosableIssue_BlockerHasOpenChild_Passes verifies that a
// blocker whose children are not all closed is not treated as closable.
func TestRunBlockedByClosableIssue_BlockerHasOpenChild_Passes(t *testing.T) {
	t.Parallel()

	// Given — blocker is open with one closed and one open child; not closable.
	blockerID := mustParseID("NP-aaaaa")
	blockedIssueID := mustParseID("NP-ddddd")
	blocker := buildEpic(t, "NP-aaaaa", domain.StateOpen)
	closedChild := buildTask(t, "NP-bbbbb", domain.StateClosed, blockerID)
	openChild := buildTask(t, "NP-ccccc", domain.StateOpen, blockerID)
	blockedIssue := buildTask(t, "NP-ddddd", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{blocker, closedChild, openChild, blockedIssue},
		map[domain.ID][]domain.ID{blockedIssueID: {blockerID}},
	)

	// When
	result, err := runBlockedByClosableIssue(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding (blocker has an open child, not closable).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// --- blocked-by-deferred-issue ---

// TestRunBlockedByDeferredIssue_DeferredBlocker_ReturnsFinding verifies that
// an issue blocked by a deferred issue emits one finding.
func TestRunBlockedByDeferredIssue_DeferredBlocker_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — deferredIssue is deferred; blockedIssue is blocked by it.
	deferredID := mustParseID("NP-aaaaa")
	blockedIssueID := mustParseID("NP-bbbbb")
	deferredIssue := buildTask(t, "NP-aaaaa", domain.StateDeferred, domain.ID{})
	blockedIssue := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{deferredIssue, blockedIssue},
		map[domain.ID][]domain.ID{blockedIssueID: {deferredID}},
	)

	// When
	result, err := runBlockedByDeferredIssue(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.BlockedByDeferredIssueRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want BlockedByDeferredIssueRow", result.Affected[0])
	}
	if row.Issue != blockedIssueID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, blockedIssueID)
	}
	if row.Blocker != deferredID.String() {
		t.Errorf("row.Blocker: got %q, want %q", row.Blocker, deferredID)
	}
}

// TestRunBlockedByDeferredIssue_OpenBlocker_Passes verifies that an issue
// blocked by an open (non-deferred) issue emits no finding.
func TestRunBlockedByDeferredIssue_OpenBlocker_Passes(t *testing.T) {
	t.Parallel()

	// Given — openBlocker is open; blockedIssue is blocked by it.
	openBlockerID := mustParseID("NP-aaaaa")
	blockedIssueID := mustParseID("NP-bbbbb")
	openBlocker := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	blockedIssue := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{openBlocker, blockedIssue},
		map[domain.ID][]domain.ID{blockedIssueID: {openBlockerID}},
	)

	// When
	result, err := runBlockedByDeferredIssue(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunBlockedByDeferredIssue_MultipleBlockers_OneRow verifies that one row
// is emitted per (issue, deferred-blocker) pair.
func TestRunBlockedByDeferredIssue_MultipleBlockers_OneRow(t *testing.T) {
	t.Parallel()

	// Given — issue has two blockers: one deferred, one open. Only the
	// deferred one should produce a finding.
	deferredID := mustParseID("NP-aaaaa")
	openBlockerID := mustParseID("NP-bbbbb")
	issueID := mustParseID("NP-ccccc")
	deferred := buildTask(t, "NP-aaaaa", domain.StateDeferred, domain.ID{})
	openBlocker := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	issue := buildTask(t, "NP-ccccc", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{deferred, openBlocker, issue},
		map[domain.ID][]domain.ID{issueID: {deferredID, openBlockerID}},
	)

	// When
	result, err := runBlockedByDeferredIssue(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding for the deferred blocker only.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if got := len(result.Affected); got != 1 {
		t.Fatalf("affected rows: got %d, want 1", got)
	}
	row := result.Affected[0].(driving.BlockedByDeferredIssueRow)
	if row.Blocker != deferredID.String() {
		t.Errorf("row.Blocker: got %q, want %q", row.Blocker, deferredID)
	}
}

// --- blocker-cycles ---

// TestRunBlockerCycles_ThreeCycle_ReturnsCanonicalOrder verifies that a 3-cycle
// produces one finding with the minimum-ID issue first and edges followed in
// blocked_by direction.
func TestRunBlockerCycles_ThreeCycle_ReturnsCanonicalOrder(t *testing.T) {
	t.Parallel()

	// Given — A blocked_by B, B blocked_by C, C blocked_by A.
	// Alphabetical order: NP-aaaaa < NP-bbbbb < NP-ccccc.
	aID := mustParseID("NP-aaaaa")
	bID := mustParseID("NP-bbbbb")
	cID := mustParseID("NP-ccccc")
	a := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	b := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	c := buildTask(t, "NP-ccccc", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{a, b, c},
		map[domain.ID][]domain.ID{
			aID: {bID}, // A blocked_by B
			bID: {cID}, // B blocked_by C
			cID: {aID}, // C blocked_by A
		},
	)

	// When
	result, err := runBlockerCycles(t.Context(), svc, driving.DoctorInput{})
	// Then — one cycle [A, B, C] with A first (minimum ID) and edges followed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a cycle finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.BlockerCycleRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want BlockerCycleRow", result.Affected[0])
	}
	want := []string{aID.String(), bID.String(), cID.String()}
	if !slices.Equal(row.Cycle, want) {
		t.Errorf("cycle order: got %v, want %v", row.Cycle, want)
	}
}

// TestRunBlockerCycles_SelfLoop_ReturnsOneMemberCycle verifies that an issue
// blocked by itself is reported as a single-element cycle.
func TestRunBlockerCycles_SelfLoop_ReturnsOneMemberCycle(t *testing.T) {
	t.Parallel()

	// Given — X has itself in BlockerIDs (self-loop; corrupt data).
	xID := mustParseID("NP-aaaaa")
	x := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{x},
		map[domain.ID][]domain.ID{xID: {xID}},
	)

	// When
	result, err := runBlockerCycles(t.Context(), svc, driving.DoctorInput{})
	// Then — one single-element cycle [X].
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a cycle finding (self-loop), got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row := result.Affected[0].(driving.BlockerCycleRow)
	if !slices.Equal(row.Cycle, []string{xID.String()}) {
		t.Errorf("cycle: got %v, want [%s]", row.Cycle, xID)
	}
}

// TestRunBlockerCycles_NonSimpleSCC_AllMembersReported verifies that a
// strongly connected component that is not a simple cycle still reports
// every member of the SCC. The greedy "follow blocked_by" walk would
// otherwise truncate, silently dropping entangled issues.
//
// Graph: A blocked_by B, A blocked_by C, B blocked_by A, C blocked_by A.
// SCC = {A, B, C} (all reach each other). The greedy walk from A picks B
// (sorted-min unvisited neighbor), then from B can only reach A (visited),
// so it stops with [A, B]. The completeness fallback must append C.
func TestRunBlockerCycles_NonSimpleSCC_AllMembersReported(t *testing.T) {
	t.Parallel()

	// Given — the non-simple SCC described above.
	aID := mustParseID("NP-aaaaa")
	bID := mustParseID("NP-bbbbb")
	cID := mustParseID("NP-ccccc")
	a := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	b := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	c := buildTask(t, "NP-ccccc", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{a, b, c},
		map[domain.ID][]domain.ID{
			aID: {bID, cID}, // A blocked_by B and C
			bID: {aID},      // B blocked_by A
			cID: {aID},      // C blocked_by A
		},
	)

	// When
	result, err := runBlockerCycles(t.Context(), svc, driving.DoctorInput{})
	// Then — one cycle finding containing all three IDs (A first as min).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a cycle finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row := result.Affected[0].(driving.BlockerCycleRow)
	if len(row.Cycle) != 3 {
		t.Fatalf("cycle members: got %d, want 3 — full SCC must be reported, got %v", len(row.Cycle), row.Cycle)
	}
	if row.Cycle[0] != aID.String() {
		t.Errorf("cycle[0]: got %q, want %q (lowest-ID member must lead)", row.Cycle[0], aID)
	}
	// The greedy walk must follow a real blocked_by edge before falling back
	// to sorted-order append. From A, sorted neighbors are [B, C]; the walk
	// picks B (smallest unvisited). So the second slot must be B — pinning
	// the contract that the algorithm prefers edge-following over plain
	// sort-by-ID. (A degenerate "always sort by ID" implementation would
	// produce [A, B, C] too, so the assertion alone isn't unique to a real
	// edge walk; the value here is making the contract explicit.)
	if row.Cycle[1] != bID.String() {
		t.Errorf("cycle[1]: got %q, want %q (greedy walk must follow A→B before fallback)", row.Cycle[1], bID)
	}
	want := map[string]bool{aID.String(): false, bID.String(): false, cID.String(): false}
	for _, id := range row.Cycle {
		if _, expected := want[id]; !expected {
			t.Errorf("unexpected cycle member: %q", id)
			continue
		}
		want[id] = true
	}
	for id, found := range want {
		if !found {
			t.Errorf("missing cycle member: %q", id)
		}
	}
}

// TestRunBlockerCycles_NoCycles_Passes verifies that a DAG (no cycles) passes.
func TestRunBlockerCycles_NoCycles_Passes(t *testing.T) {
	t.Parallel()

	// Given — A blocked_by B (no cycle).
	aID := mustParseID("NP-aaaaa")
	bID := mustParseID("NP-bbbbb")
	a := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	b := buildTask(t, "NP-bbbbb", domain.StateOpen, domain.ID{})
	svc := newGraphSvcWithBlockers(
		[]domain.Issue{a, b},
		map[domain.ID][]domain.ID{aID: {bID}},
	)

	// When
	result, err := runBlockerCycles(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %v", result.Summary)
	}
}

// --- priority-inversions ---

// TestRunPriorityInversions_P0ChildP2Parent_ReturnsFinding verifies that a P0
// child with a P2 parent emits one finding.
func TestRunPriorityInversions_P0ChildP2Parent_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — parent is P2, child is P0 (higher priority than parent).
	parentID := mustParseID("NP-aaaaa")
	childID := mustParseID("NP-bbbbb")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen).WithPriority(domain.P2)
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID).WithPriority(domain.P0)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runPriorityInversions(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.PriorityInversionRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want PriorityInversionRow", result.Affected[0])
	}
	if row.Issue != childID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, childID)
	}
	if row.Parent != parentID.String() {
		t.Errorf("row.Parent: got %q, want %q", row.Parent, parentID)
	}
	if row.ChildPriority != "P0" {
		t.Errorf("row.ChildPriority: got %q, want %q", row.ChildPriority, "P0")
	}
	if row.ParentPriority != "P2" {
		t.Errorf("row.ParentPriority: got %q, want %q", row.ParentPriority, "P2")
	}
}

// TestRunPriorityInversions_SamePriority_Passes verifies that equal priorities
// do not produce a finding (only strict inequality triggers).
func TestRunPriorityInversions_SamePriority_Passes(t *testing.T) {
	t.Parallel()

	// Given — parent and child both P1.
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen).WithPriority(domain.P1)
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID).WithPriority(domain.P1)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runPriorityInversions(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunPriorityInversions_LowerPriorityChild_Passes verifies that a child
// with lower priority than the parent does not produce a finding.
func TestRunPriorityInversions_LowerPriorityChild_Passes(t *testing.T) {
	t.Parallel()

	// Given — parent is P0, child is P3 (lower priority).
	parentID := mustParseID("NP-aaaaa")
	parent := buildEpic(t, "NP-aaaaa", domain.StateOpen).WithPriority(domain.P0)
	child := buildTask(t, "NP-bbbbb", domain.StateOpen, parentID).WithPriority(domain.P3)
	svc := newGraphSvc([]domain.Issue{parent, child})

	// When
	result, err := runPriorityInversions(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunPriorityInversions_NoParent_Passes verifies that root issues (no
// parent) never produce a finding.
func TestRunPriorityInversions_NoParent_Passes(t *testing.T) {
	t.Parallel()

	// Given — a root issue with P0 (no parent to compare against).
	issue := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{}).WithPriority(domain.P0)
	svc := newGraphSvc([]domain.Issue{issue})

	// When
	result, err := runPriorityInversions(t.Context(), svc, driving.DoctorInput{})
	// Then — no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// --- JSON shape round-trips ---

// TestBlockedByAncestorRow_JSONRoundTrip verifies the spec-mandated JSON shape.
func TestBlockedByAncestorRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := driving.BlockedByAncestorRow{
		Issue:            "NP-abc12",
		BlockingAncestor: "NP-xyz98",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"issue":"NP-abc12","blocking_ancestor":"NP-xyz98"}`
	if got := string(b); got != want {
		t.Errorf("JSON shape:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.BlockedByAncestorRow
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("decoded != original: %+v vs %+v", decoded, original)
	}
}

// TestBlockedByClosableIssueRow_JSONRoundTrip verifies the spec-mandated JSON shape.
func TestBlockedByClosableIssueRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := driving.BlockedByClosableIssueRow{
		Issue:           "NP-abc12",
		ClosableBlocker: "NP-xyz98",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"issue":"NP-abc12","closable_blocker":"NP-xyz98"}`
	if got := string(b); got != want {
		t.Errorf("JSON shape:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.BlockedByClosableIssueRow
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("decoded != original: %+v vs %+v", decoded, original)
	}
}

// TestBlockedByDeferredIssueRow_JSONRoundTrip verifies the spec-mandated JSON shape.
func TestBlockedByDeferredIssueRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := driving.BlockedByDeferredIssueRow{
		Issue:   "NP-abc12",
		Blocker: "NP-xyz98",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"issue":"NP-abc12","blocker":"NP-xyz98"}`
	if got := string(b); got != want {
		t.Errorf("JSON shape:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.BlockedByDeferredIssueRow
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("decoded != original: %+v vs %+v", decoded, original)
	}
}

// TestBlockerCycleRow_JSONRoundTrip verifies the spec-mandated JSON shape.
func TestBlockerCycleRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := driving.BlockerCycleRow{
		Cycle: []string{"NP-aaa", "NP-bbb", "NP-ccc"},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"cycle":["NP-aaa","NP-bbb","NP-ccc"]}`
	if got := string(b); got != want {
		t.Errorf("JSON shape:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.BlockerCycleRow
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !slices.Equal(decoded.Cycle, original.Cycle) {
		t.Errorf("decoded.Cycle != original.Cycle: %v vs %v", decoded.Cycle, original.Cycle)
	}
}

// TestPriorityInversionRow_JSONRoundTrip verifies the spec-mandated JSON shape.
func TestPriorityInversionRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := driving.PriorityInversionRow{
		Issue:          "NP-child",
		Parent:         "NP-parent",
		ChildPriority:  "P0",
		ParentPriority: "P3",
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"issue":"NP-child","parent":"NP-parent","child_priority":"P0","parent_priority":"P3"}`
	if got := string(b); got != want {
		t.Errorf("JSON shape:\ngot:  %s\nwant: %s", got, want)
	}
	var decoded driving.PriorityInversionRow
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("decoded != original: %+v vs %+v", decoded, original)
	}
}

// --- helpers ---

// findRegistryEntry returns the registry entry for slug. Fails the test if not found.
func findRegistryEntry(t *testing.T, slug string) doctorCheckEntry {
	t.Helper()
	for _, e := range doctorRegistry() {
		if e.Slug == slug {
			return e
		}
	}
	t.Fatalf("registry entry not found: %q", slug)
	panic("unreachable")
}

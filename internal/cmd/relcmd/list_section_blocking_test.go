package relcmd_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

// addBlockedBy records that issueID is blocked by blockerID.
func addBlockedBy(t *testing.T, svc driving.Service, issueID, blockerID, author string) {
	t.Helper()
	err := svc.AddRelationship(t.Context(), issueID, driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: blockerID,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add blocked_by %q → %q: %v", issueID, blockerID, err)
	}
}

// setPriority claims an issue, updates its priority, and releases the claim.
func setPriority(t *testing.T, svc driving.Service, id domain.ID, p domain.Priority) {
	t.Helper()
	ctx := t.Context()
	claimID := claimIssue(t, svc, id)
	if err := svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID:  id.String(),
		ClaimID:  claimID,
		Priority: &p,
	}); err != nil {
		t.Fatalf("precondition: set priority %v on %s: %v", p, id, err)
	}
	if err := svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: id.String(),
		ClaimID: claimID,
		Action:  driving.ActionRelease,
	}); err != nil {
		t.Fatalf("precondition: release claim on %s: %v", id, err)
	}
}

// renderBlocking invokes RenderBlockingSection and returns the output as a string.
func renderBlocking(t *testing.T, svc driving.Service, tty bool) string {
	t.Helper()

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(tty)

	if err := relcmd.RenderBlockingSection(t.Context(), svc, ios); err != nil {
		t.Fatalf("RenderBlockingSection failed: %v", err)
	}
	return out.String()
}

// --- Tests ---

// TestRenderBlockingSection_EmptyGraph_ShowsZeroCounts verifies that an empty
// database renders the section header with zero counts and no tree rows.
func TestRenderBlockingSection_EmptyGraph_ShowsZeroCounts(t *testing.T) {
	t.Parallel()

	// Given: an empty database.
	svc := setupService(t)

	// When: rendering the blocking section.
	output := renderBlocking(t, svc, false)

	// Then: the header shows zero chains, edges, and cycles.
	if !strings.Contains(output, "Blocking (0 chains, 0 edges, 0 cycles)") {
		t.Errorf("expected zero-count header; output:\n%s", output)
	}
	// Then: no tree rows (header only).
	lines := nonEmptyLines(output)
	if len(lines) != 1 {
		t.Errorf("line count: got %d, want 1 (header only); output:\n%s", len(lines), output)
	}
}

// TestRenderBlockingSection_NoBlockingEdges_ShowsZeroCounts verifies that a
// database with issues but no blocking relationships produces zero counts.
func TestRenderBlockingSection_NoBlockingEdges_ShowsZeroCounts(t *testing.T) {
	t.Parallel()

	// Given: two unrelated tasks with no blocking relationship.
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")
	_ = createTask(t, svc, "Task B")

	// When: rendering the blocking section.
	output := renderBlocking(t, svc, false)

	// Then: the header still shows zero counts.
	if !strings.Contains(output, "Blocking (0 chains, 0 edges, 0 cycles)") {
		t.Errorf("expected zero-count header with no blocking edges; output:\n%s", output)
	}
}

// TestRenderBlockingSection_SingleChain_RendersHeaderAndChain verifies that a
// single A→B blocking chain produces the correct header and tree rows.
func TestRenderBlockingSection_SingleChain_RendersHeaderAndChain(t *testing.T) {
	t.Parallel()

	// Given: task A blocks task B (B is blocked_by A).
	svc := setupService(t)
	a := createTask(t, svc, "Task A")
	b := createTask(t, svc, "Task B")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")

	// When: rendering the blocking section.
	output := renderBlocking(t, svc, false)

	// Then: the header shows 1 chain, 1 edge, 0 cycles.
	if !strings.Contains(output, "Blocking (1 chains, 1 edges, 0 cycles)") {
		t.Errorf("expected 1-chain header; output:\n%s", output)
	}
	// Then: both issue IDs appear in the output.
	if !strings.Contains(output, a.String()) {
		t.Errorf("blocker ID %q missing from output:\n%s", a.String(), output)
	}
	if !strings.Contains(output, b.String()) {
		t.Errorf("blocked ID %q missing from output:\n%s", b.String(), output)
	}
}

// TestRenderBlockingSection_SingleChain_BlockerIsRoot verifies that the blocker
// appears before the blocked issue in the output.
func TestRenderBlockingSection_SingleChain_BlockerIsRoot(t *testing.T) {
	t.Parallel()

	// Given: A blocks B.
	svc := setupService(t)
	a := createTask(t, svc, "Blocker A")
	b := createTask(t, svc, "Blocked B")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: A appears before B in the output.
	posA := strings.Index(output, a.String())
	posB := strings.Index(output, b.String())
	if posA < 0 || posB < 0 {
		t.Fatalf("expected both IDs in output; got:\n%s", output)
	}
	if posA >= posB {
		t.Errorf("blocker %q should appear before blocked %q; output:\n%s", a.String(), b.String(), output)
	}
}

// TestRenderBlockingSection_DiamondGraph_DeduplicatesBackRef verifies that a
// diamond DAG (A→B, A→C, B→D, C→D) renders D only once in full, with a
// back-reference marker at the second occurrence.
func TestRenderBlockingSection_DiamondGraph_DeduplicatesBackRef(t *testing.T) {
	t.Parallel()

	// Given: A→B, A→C, B→D, C→D (diamond).
	svc := setupService(t)
	a := createTask(t, svc, "A (root)")
	b := createTask(t, svc, "B")
	c := createTask(t, svc, "C")
	d := createTask(t, svc, "D (shared)")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent") // B blocked_by A
	addBlockedBy(t, svc, c.String(), a.String(), "test-agent") // C blocked_by A
	addBlockedBy(t, svc, d.String(), b.String(), "test-agent") // D blocked_by B
	addBlockedBy(t, svc, d.String(), c.String(), "test-agent") // D blocked_by C

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: D's ID appears exactly twice (once in full, once as back-ref).
	count := strings.Count(output, d.String())
	if count != 2 {
		t.Errorf("D %q should appear exactly 2 times (full + back-ref); got %d; output:\n%s",
			d.String(), count, output)
	}
	// Then: the back-reference marker "shown above" appears in the output.
	if !strings.Contains(output, "shown above") {
		t.Errorf("expected back-reference marker 'shown above'; output:\n%s", output)
	}
	// Then: the header shows 4 edges (A→B, A→C, B→D, C→D).
	if !strings.Contains(output, "4 edges") {
		t.Errorf("expected 4 edges in header; output:\n%s", output)
	}
}

// TestRenderBlockingSection_MultiRoot_RendersAllChains verifies that multiple
// independent blocking chains each appear in the output.
func TestRenderBlockingSection_MultiRoot_RendersAllChains(t *testing.T) {
	t.Parallel()

	// Given: chain 1 (A→B) and chain 2 (C→D) — two independent roots.
	svc := setupService(t)
	a := createTask(t, svc, "Chain1-Blocker")
	b := createTask(t, svc, "Chain1-Blocked")
	c := createTask(t, svc, "Chain2-Blocker")
	d := createTask(t, svc, "Chain2-Blocked")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")
	addBlockedBy(t, svc, d.String(), c.String(), "test-agent")

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: 2 chains, 2 edges, 0 cycles.
	if !strings.Contains(output, "2 chains") {
		t.Errorf("expected 2 chains in header; output:\n%s", output)
	}
	if !strings.Contains(output, "2 edges") {
		t.Errorf("expected 2 edges in header; output:\n%s", output)
	}
	// Then: all four issue IDs appear.
	for _, id := range []string{a.String(), b.String(), c.String(), d.String()} {
		if !strings.Contains(output, id) {
			t.Errorf("ID %q missing from output:\n%s", id, output)
		}
	}
}

// TestRenderBlockingSection_Cycle_BannerAndCycleCount verifies that a cycle
// (A→B→A) is detected, the cycle count in the header is non-zero, and a
// "Cycles detected" banner appears.
func TestRenderBlockingSection_Cycle_BannerAndCycleCount(t *testing.T) {
	t.Parallel()

	// Given: A blocks B, B blocks A (a 2-cycle).
	svc := setupService(t)
	a := createTask(t, svc, "Cycle-A")
	b := createTask(t, svc, "Cycle-B")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent") // B blocked_by A
	addBlockedBy(t, svc, a.String(), b.String(), "test-agent") // A blocked_by B

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: the cycle count in the header is non-zero.
	if strings.Contains(output, "0 cycles") {
		t.Errorf("expected non-zero cycle count in header; output:\n%s", output)
	}
	// Then: a "Cycles detected" banner appears.
	if !strings.Contains(output, "Cycles detected") {
		t.Errorf("expected 'Cycles detected' banner; output:\n%s", output)
	}
	// Then: both cycle-node IDs appear in the output.
	if !strings.Contains(output, a.String()) {
		t.Errorf("cycle node %q missing from output:\n%s", a.String(), output)
	}
	if !strings.Contains(output, b.String()) {
		t.Errorf("cycle node %q missing from output:\n%s", b.String(), output)
	}
}

// TestRenderBlockingSection_StandaloneCycle_CycleNodesTreatedAsRoots verifies
// that a standalone cycle with no external entry point has its nodes treated as
// roots so the output lists them all.
func TestRenderBlockingSection_StandaloneCycle_CycleNodesTreatedAsRoots(t *testing.T) {
	t.Parallel()

	// Given: A→B→C→A — a standalone 3-cycle with no external root.
	svc := setupService(t)
	a := createTask(t, svc, "Cycle-A")
	b := createTask(t, svc, "Cycle-B")
	c := createTask(t, svc, "Cycle-C")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent") // B blocked_by A
	addBlockedBy(t, svc, c.String(), b.String(), "test-agent") // C blocked_by B
	addBlockedBy(t, svc, a.String(), c.String(), "test-agent") // A blocked_by C

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: all three cycle nodes appear in the output.
	for _, id := range []string{a.String(), b.String(), c.String()} {
		if !strings.Contains(output, id) {
			t.Errorf("cycle node %q missing from output:\n%s", id, output)
		}
	}
	// Then: a non-zero cycle count appears in the header.
	if strings.Contains(output, "0 cycles") {
		t.Errorf("expected non-zero cycle count; output:\n%s", output)
	}
}

// TestRenderBlockingSection_PriorityOrdering_RootsAreSortedHighToLow verifies
// that roots are sorted by priority ascending (P0 before P3).
func TestRenderBlockingSection_PriorityOrdering_RootsAreSortedHighToLow(t *testing.T) {
	t.Parallel()

	// Given: P3 blocker and P0 blocker, each blocking a separate issue.
	svc := setupService(t)

	p3blocker := createTask(t, svc, "P3 blocker")
	p3blocked := createTask(t, svc, "P3 blocked")
	setPriority(t, svc, p3blocker, domain.P3)
	addBlockedBy(t, svc, p3blocked.String(), p3blocker.String(), "test-agent")

	p0blocker := createTask(t, svc, "P0 blocker")
	p0blocked := createTask(t, svc, "P0 blocked")
	setPriority(t, svc, p0blocker, domain.P0)
	addBlockedBy(t, svc, p0blocked.String(), p0blocker.String(), "test-agent")

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: the P0 blocker appears before the P3 blocker in the output.
	posP0 := strings.Index(output, p0blocker.String())
	posP3 := strings.Index(output, p3blocker.String())
	if posP0 < 0 || posP3 < 0 {
		t.Fatalf("expected both blocker IDs in output; got:\n%s", output)
	}
	if posP0 >= posP3 {
		t.Errorf("P0 blocker %q should appear before P3 blocker %q; output:\n%s",
			p0blocker.String(), p3blocker.String(), output)
	}
}

// TestRenderBlockingSection_ClosedIssueEdgeSuppressed verifies that edges
// involving a closed blocker are excluded from the blocking section.
func TestRenderBlockingSection_ClosedIssueEdgeSuppressed(t *testing.T) {
	t.Parallel()

	// Given: A blocks B, but A is closed.
	svc := setupService(t)
	a := createTask(t, svc, "Closed blocker A")
	b := createTask(t, svc, "B blocked by closed A")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")
	closeTestIssue(t, svc, a.String())

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: zero counts (A is closed, so no active blocking edges).
	if !strings.Contains(output, "Blocking (0 chains, 0 edges, 0 cycles)") {
		t.Errorf("expected zero-count header when all blockers are closed; output:\n%s", output)
	}
	// Then: the closed issue's ID does not appear.
	if strings.Contains(output, a.String()) {
		t.Errorf("closed blocker %q should not appear in output:\n%s", a.String(), output)
	}
}

// TestRenderBlockingSection_ClosedBlockedIssueSuppressed verifies that when the
// blocked endpoint of an edge is closed, the edge is suppressed and the blocker
// does not appear as a root with active blocking edges.
func TestRenderBlockingSection_ClosedBlockedIssueSuppressed(t *testing.T) {
	t.Parallel()

	// Given: A blocks B, but B is closed.
	svc := setupService(t)
	a := createTask(t, svc, "Blocker A")
	b := createTask(t, svc, "Closed blocked B")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")
	closeTestIssue(t, svc, b.String())

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: zero counts — B is closed, so A has no active blocking edges.
	if !strings.Contains(output, "Blocking (0 chains, 0 edges, 0 cycles)") {
		t.Errorf("expected zero-count header when blocked issue is closed; output:\n%s", output)
	}
	// Then: neither issue appears in the output.
	if strings.Contains(output, b.String()) {
		t.Errorf("closed blocked issue %q should not appear in output:\n%s", b.String(), output)
	}
}

// TestRenderBlockingSection_CycleReachableFromRoot_NotInflatingChainCount
// verifies that cycle nodes reachable from a regular root are not redundantly
// promoted to additional roots. The chain count should reflect only the regular
// root, not the cycle nodes within that root's subtree.
func TestRenderBlockingSection_CycleReachableFromRoot_NotInflatingChainCount(t *testing.T) {
	t.Parallel()

	// Given: A→B, B→C, C→B  (A is a regular root; B→C→B is a cycle within it).
	svc := setupService(t)
	a := createTask(t, svc, "Regular root A")
	b := createTask(t, svc, "Cycle node B")
	c := createTask(t, svc, "Cycle node C")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent") // B blocked_by A
	addBlockedBy(t, svc, c.String(), b.String(), "test-agent") // C blocked_by B
	addBlockedBy(t, svc, b.String(), c.String(), "test-agent") // B blocked_by C (cycle)

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: only 1 chain (A is the sole root; B and C are in its subtree).
	if !strings.Contains(output, "1 chains") {
		t.Errorf("expected 1 chain when cycle is reachable from regular root; output:\n%s", output)
	}
	// Then: all three issue IDs appear in the output.
	for _, id := range []string{a.String(), b.String(), c.String()} {
		if !strings.Contains(output, id) {
			t.Errorf("ID %q missing from output:\n%s", id, output)
		}
	}
}

// TestRenderBlockingSection_HeaderAlwaysPresent verifies that the section header
// is always rendered, even when there are no blocking relationships.
func TestRenderBlockingSection_HeaderAlwaysPresent(t *testing.T) {
	t.Parallel()

	// Given: a non-empty database with no blocking relationships.
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: the section header is always present.
	if !strings.Contains(output, "Blocking (") {
		t.Errorf("section header missing; output:\n%s", output)
	}
}

// TestRenderBlockingSection_TableColumns_HeaderPresent verifies that the table
// header (TREE, P, ROLE, STATE, TITLE) is present when blocking edges exist.
func TestRenderBlockingSection_TableColumns_HeaderPresent(t *testing.T) {
	t.Parallel()

	// Given: a single blocking chain.
	svc := setupService(t)
	a := createTask(t, svc, "A")
	b := createTask(t, svc, "B")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")

	// When: rendering.
	output := renderBlocking(t, svc, false)

	// Then: the table header columns are present.
	for _, col := range []string{"TREE", "P", "ROLE", "STATE", "TITLE"} {
		if !strings.Contains(output, col) {
			t.Errorf("expected column %q in output; output:\n%s", col, output)
		}
	}
}

// TestRenderBlockingSection_NonTTY_FullTitles verifies that non-TTY output does
// not truncate titles.
func TestRenderBlockingSection_NonTTY_FullTitles(t *testing.T) {
	t.Parallel()

	// Given: a blocking chain with a very long title on the blocked issue.
	svc := setupService(t)
	longTitle := strings.Repeat("A", 120)
	a := createTask(t, svc, "Blocker")
	b := createTask(t, svc, longTitle)
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")

	// When: rendering in non-TTY mode.
	output := renderBlocking(t, svc, false)

	// Then: the full title appears unchanged (no ellipsis truncation).
	if !strings.Contains(output, longTitle) {
		t.Errorf("expected full title in non-TTY output; output:\n%s", output)
	}
	if strings.Contains(output, "…") {
		t.Errorf("unexpected truncation ellipsis in non-TTY output; output:\n%s", output)
	}
}

// TestRenderBlockingSection_TTY_TruncatesLongTitles verifies that TTY output
// truncates titles that exceed the available width.
func TestRenderBlockingSection_TTY_TruncatesLongTitles(t *testing.T) {
	t.Parallel()

	// Given: a narrow terminal (80 cols) and a very long title.
	svc := setupService(t)
	longTitle := strings.Repeat("X", 200)
	a := createTask(t, svc, "Blocker")
	b := createTask(t, svc, longTitle)
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetTerminalWidth(80)

	// When: rendering with a narrow TTY.
	if err := relcmd.RenderBlockingSection(t.Context(), svc, ios); err != nil {
		t.Fatalf("RenderBlockingSection failed: %v", err)
	}
	output := out.String()

	// Then: the ellipsis appears (title was truncated).
	if !strings.Contains(output, "…") {
		t.Errorf("expected truncation ellipsis in narrow TTY output; output:\n%s", output)
	}
	// Then: the full 200-char title does NOT appear.
	if strings.Contains(output, longTitle) {
		t.Errorf("full 200-char title should be truncated; output:\n%s", output)
	}
}

// TestRenderBlockingSection_NoFilter_BlockingSectionIncluded verifies that
// running RunList with no --rel filter still includes the blocking section in
// the combined output.
func TestRenderBlockingSection_NoFilter_BlockingSectionIncluded(t *testing.T) {
	t.Parallel()

	// Given: a service with one blocking edge.
	svc := setupService(t)
	a := createTask(t, svc, "Blocker")
	b := createTask(t, svc, "Blocked")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")
	ios, _, out, _ := iostreams.Test()

	noop := func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error { return nil }

	// When: running list with no filter (all sections).
	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RenderParentChild: noop,
		RenderBlocking:    relcmd.RenderBlockingSection,
		RenderRefs:        noop,
	}
	if err := relcmd.RunList(t.Context(), input); err != nil {
		t.Fatalf("RunList failed: %v", err)
	}

	// Then: the blocking section header appears in combined output.
	if !strings.Contains(out.String(), "Blocking (") {
		t.Errorf("expected blocking section header in no-filter output; got:\n%s", out.String())
	}
}

// TestRenderBlockingSection_RunList_DispatchedCorrectly verifies that the
// blocking section is dispatched correctly from RunList when the blocking filter
// is active.
func TestRenderBlockingSection_RunList_DispatchedCorrectly(t *testing.T) {
	t.Parallel()

	// Given: a service with one blocking chain.
	svc := setupService(t)
	a := createTask(t, svc, "Blocker")
	b := createTask(t, svc, "Blocked")
	addBlockedBy(t, svc, b.String(), a.String(), "test-agent")
	ios, _, out, _ := iostreams.Test()

	noop := func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error { return nil }

	// When: running list with blocking filter.
	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RelFilter:         relcmd.RelListCategoryBlocking,
		RenderParentChild: noop,
		RenderBlocking:    relcmd.RenderBlockingSection,
		RenderRefs:        noop,
	}
	if err := relcmd.RunList(t.Context(), input); err != nil {
		t.Fatalf("RunList failed: %v", err)
	}

	// Then: the blocking section header appears in output.
	if !strings.Contains(out.String(), "Blocking") {
		t.Errorf("expected blocking section in output; got:\n%s", out.String())
	}
}

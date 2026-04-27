package relcmd_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// closeTestIssue claims and immediately closes an issue for test setup.
func closeTestIssue(t *testing.T, svc driving.Service, issueID string) {
	t.Helper()

	ctx := t.Context()
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: issueID,
		Author:  "test-closer",
	})
	if err != nil {
		t.Fatalf("precondition: claim issue %q: %v", issueID, err)
	}
	if err := svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: issueID,
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
	}); err != nil {
		t.Fatalf("precondition: close issue %q: %v", issueID, err)
	}
}

// renderParentChild invokes renderParentChildSection and returns the output as
// a string.
func renderParentChild(t *testing.T, svc driving.Service, tty bool) string {
	t.Helper()

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(tty)

	err := relcmd.RenderParentChildSection(t.Context(), svc, ios)
	if err != nil {
		t.Fatalf("renderParentChildSection failed: %v", err)
	}
	return out.String()
}

// --- Tests ---

// TestRenderParentChildSection_EmptyDatabase_ShowsZeroCounts verifies that an
// empty database renders the section header with zero counts and no tree rows.
func TestRenderParentChildSection_EmptyDatabase_ShowsZeroCounts(t *testing.T) {
	t.Parallel()

	// Given: an empty database.
	svc := setupService(t)

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: the header line shows zero roots and zero issues.
	if !strings.Contains(output, "Parent-child (0 roots, 0 issues)") {
		t.Errorf("expected zero-count header; output:\n%s", output)
	}
	// Then: no issue rows (only the header line).
	lines := nonEmptyLines(output)
	if len(lines) != 1 {
		t.Errorf("line count: got %d, want 1 (header only); output:\n%s", len(lines), output)
	}
}

// TestRenderParentChildSection_SingleRoot_RendersHeaderAndTree verifies that a
// single root with one non-closed child renders the correct header and tree.
func TestRenderParentChildSection_SingleRoot_RendersHeaderAndTree(t *testing.T) {
	t.Parallel()

	// Given: one root epic with one task child.
	svc := setupService(t)
	rootID := createTreeEpic(t, svc, "Root epic", "")
	childID := createTreeTask(t, svc, "Child task", rootID.String())

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: header shows 1 root and 2 issues (root + child).
	if !strings.Contains(output, "Parent-child (1 roots, 2 issues)") {
		t.Errorf("expected header with 1 root, 2 issues; output:\n%s", output)
	}
	// Then: both issue IDs appear in the output.
	if !strings.Contains(output, rootID.String()) {
		t.Errorf("root ID %q missing from output:\n%s", rootID.String(), output)
	}
	if !strings.Contains(output, childID.String()) {
		t.Errorf("child ID %q missing from output:\n%s", childID.String(), output)
	}
}

// TestRenderParentChildSection_MultipleRoots_RendersAllTrees verifies that
// multiple qualifying roots each produce a separate tree in the output.
func TestRenderParentChildSection_MultipleRoots_RendersAllTrees(t *testing.T) {
	t.Parallel()

	// Given: two independent root epics, each with one child.
	svc := setupService(t)
	root1 := createTreeEpic(t, svc, "Root 1", "")
	child1 := createTreeTask(t, svc, "Child of root 1", root1.String())
	root2 := createTreeEpic(t, svc, "Root 2", "")
	child2 := createTreeTask(t, svc, "Child of root 2", root2.String())

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: header shows 2 roots and 4 issues.
	if !strings.Contains(output, "Parent-child (2 roots, 4 issues)") {
		t.Errorf("expected header with 2 roots, 4 issues; output:\n%s", output)
	}
	// Then: all four IDs appear.
	for _, id := range []string{root1.String(), child1.String(), root2.String(), child2.String()} {
		if !strings.Contains(output, id) {
			t.Errorf("ID %q missing from output:\n%s", id, output)
		}
	}
}

// TestRenderParentChildSection_ClosedChildren_EdgeSuppressed verifies that
// closed children are excluded and their subtrees do not appear. A root whose
// only child is closed must not appear as a qualifying root.
func TestRenderParentChildSection_ClosedChildren_EdgeSuppressed(t *testing.T) {
	t.Parallel()

	// Given: a root epic with two children — one open, one closed.
	svc := setupService(t)
	root := createTreeEpic(t, svc, "Root", "")
	openChild := createTreeTask(t, svc, "Open child", root.String())
	closedChild := createTreeTask(t, svc, "Closed child", root.String())
	closeTestIssue(t, svc, closedChild.String())

	// Given: a second root whose only child is closed — it must be excluded.
	root2 := createTreeEpic(t, svc, "Root with only closed child", "")
	onlyClosedChild := createTreeTask(t, svc, "Only closed child", root2.String())
	closeTestIssue(t, svc, onlyClosedChild.String())

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: header shows 1 root and 2 issues (root + open child only).
	if !strings.Contains(output, "Parent-child (1 roots, 2 issues)") {
		t.Errorf("expected header with 1 root, 2 issues; output:\n%s", output)
	}
	// Then: the open child appears.
	if !strings.Contains(output, openChild.String()) {
		t.Errorf("open child ID %q missing from output:\n%s", openChild.String(), output)
	}
	// Then: the closed child and root2 do NOT appear.
	if strings.Contains(output, closedChild.String()) {
		t.Errorf("closed child ID %q should not appear in output:\n%s", closedChild.String(), output)
	}
	if strings.Contains(output, root2.String()) {
		t.Errorf("root2 %q (all children closed) should not appear in output:\n%s", root2.String(), output)
	}
}

// TestRenderParentChildSection_OrphanIssue_Omitted verifies that a non-closed
// issue with no parent and no non-closed children does not appear.
func TestRenderParentChildSection_OrphanIssue_Omitted(t *testing.T) {
	t.Parallel()

	// Given: a standalone task with no parent and no children.
	svc := setupService(t)
	orphan := createTreeTask(t, svc, "Lone orphan", "")

	// Given: a qualifying root so the section is non-empty.
	root := createTreeEpic(t, svc, "Root", "")
	_ = createTreeTask(t, svc, "Child", root.String())

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: the orphan ID does not appear.
	if strings.Contains(output, orphan.String()) {
		t.Errorf("orphan ID %q should not appear in output:\n%s", orphan.String(), output)
	}
	// Then: the root and its child appear.
	if !strings.Contains(output, root.String()) {
		t.Errorf("root ID %q missing from output:\n%s", root.String(), output)
	}
}

// TestRenderParentChildSection_HeaderAlwaysPresent verifies that the section
// header is always rendered, even when there are no qualifying roots.
func TestRenderParentChildSection_HeaderAlwaysPresent(t *testing.T) {
	t.Parallel()

	// Given: a non-empty database with only orphan issues.
	svc := setupService(t)
	_ = createTreeTask(t, svc, "Lone task A", "")
	_ = createTreeTask(t, svc, "Lone task B", "")

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: the section header is present with zero counts.
	if !strings.Contains(output, "Parent-child (") {
		t.Errorf("section header missing; output:\n%s", output)
	}
}

// TestRenderParentChildSection_ClosedRootBecomesVirtualRoot verifies that a
// non-closed issue whose parent is closed appears as a root in the forest (its
// parent link is treated as non-existent because the parent is closed).
func TestRenderParentChildSection_ClosedParent_ChildBecomesRoot(t *testing.T) {
	t.Parallel()

	// Given: parent → child → grandchild. The domain requires bottom-up
	// closing (children first). We close all three, then reopen child and
	// grandchild so that parent remains closed while child and grandchild
	// are open — the target scenario for virtual-root promotion.
	svc := setupService(t)
	parent := createTreeEpic(t, svc, "Closed parent", "")
	child := createTreeEpic(t, svc, "Open child (virtual root)", parent.String())
	grandchild := createTreeTask(t, svc, "Open grandchild", child.String())

	// Close bottom-up.
	closeTestIssue(t, svc, grandchild.String())
	closeTestIssue(t, svc, child.String())
	closeTestIssue(t, svc, parent.String())

	// Reopen from bottom so only the parent stays closed.
	ctx := t.Context()
	if err := svc.ReopenIssue(ctx, driving.ReopenInput{IssueID: grandchild.String(), Author: "test"}); err != nil {
		t.Fatalf("precondition: reopen grandchild: %v", err)
	}
	if err := svc.ReopenIssue(ctx, driving.ReopenInput{IssueID: child.String(), Author: "test"}); err != nil {
		t.Fatalf("precondition: reopen child: %v", err)
	}

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: the child becomes a root (1 root, 2 issues: child + grandchild).
	if !strings.Contains(output, "Parent-child (1 roots, 2 issues)") {
		t.Errorf("expected 1 root (virtual root from closed parent); output:\n%s", output)
	}
	// Then: the closed parent does NOT appear.
	if strings.Contains(output, parent.String()) {
		t.Errorf("closed parent %q should not appear; output:\n%s", parent.String(), output)
	}
	// Then: the virtual root and grandchild DO appear.
	if !strings.Contains(output, child.String()) {
		t.Errorf("virtual root %q missing from output:\n%s", child.String(), output)
	}
	if !strings.Contains(output, grandchild.String()) {
		t.Errorf("grandchild %q missing from output:\n%s", grandchild.String(), output)
	}
}

// TestRenderParentChildSection_SectionHeader_CountsIncludeRootIssues verifies
// that M in "N roots, M issues" counts all issues in rendered subtrees,
// including roots themselves.
func TestRenderParentChildSection_SectionHeader_CountsIncludeRoots(t *testing.T) {
	t.Parallel()

	// Given: root → child1 → grandchild. Total: 3 non-closed issues.
	svc := setupService(t)
	root := createTreeEpic(t, svc, "Root", "")
	child := createTreeEpic(t, svc, "Child", root.String())
	grandchild := createTreeTask(t, svc, "Grandchild", child.String())
	_ = grandchild

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: 1 root, 3 issues (root + child + grandchild).
	if !strings.Contains(output, "Parent-child (1 roots, 3 issues)") {
		t.Errorf("expected 1 root, 3 issues; output:\n%s", output)
	}
}

// TestRenderParentChildSection_TreeUsesRenderTreeText_TableHeader verifies
// that the output uses the existing RenderTreeText table format (TREE, P,
// ROLE, STATE, TITLE columns).
func TestRenderParentChildSection_TreeUsesTableFormat(t *testing.T) {
	t.Parallel()

	// Given: a simple root with a child.
	svc := setupService(t)
	root := createTreeEpic(t, svc, "Root", "")
	_ = createTreeTask(t, svc, "Child", root.String())

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: the table header (TREE, P, ROLE, STATE, TITLE) is present.
	for _, col := range []string{"TREE", "P", "ROLE", "STATE", "TITLE"} {
		if !strings.Contains(output, col) {
			t.Errorf("expected column %q in output; output:\n%s", col, output)
		}
	}
}

// TestRenderParentChildSection_NonTTY_FullTitles verifies that non-TTY output
// does not truncate titles.
func TestRenderParentChildSection_NonTTY_FullTitles(t *testing.T) {
	t.Parallel()

	// Given: a root with a child having a very long title.
	svc := setupService(t)
	longTitle := strings.Repeat("A", 120)
	root := createTreeEpic(t, svc, "Root", "")
	_ = createTreeTask(t, svc, longTitle, root.String())

	// When: rendering in non-TTY mode.
	output := renderParentChild(t, svc, false)

	// Then: the full title appears unchanged (no ellipsis truncation).
	if !strings.Contains(output, longTitle) {
		t.Errorf("expected full title in non-TTY output; output:\n%s", output)
	}
	if strings.Contains(output, "…") {
		t.Errorf("unexpected truncation ellipsis in non-TTY output; output:\n%s", output)
	}
}

// TestRenderParentChildSection_TTY_TruncatesLongTitles verifies that TTY output
// truncates titles that exceed the available width.
func TestRenderParentChildSection_TTY_TruncatesLongTitles(t *testing.T) {
	t.Parallel()

	// Given: a narrow terminal (80 cols) and a very long title.
	svc := setupService(t)
	longTitle := strings.Repeat("X", 200)
	root := createTreeEpic(t, svc, "Root", "")
	_ = createTreeTask(t, svc, longTitle, root.String())

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetTerminalWidth(80)

	// When: rendering the parent-child section.
	err := relcmd.RenderParentChildSection(t.Context(), svc, ios)
	if err != nil {
		t.Fatalf("renderParentChildSection failed: %v", err)
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

// TestRenderParentChildSection_MultipleRoots_SingleTableHeader verifies that
// when multiple roots exist, the tree table header (TREE P ROLE STATE TITLE)
// appears exactly once — all roots are rendered in a single aligned table.
func TestRenderParentChildSection_MultipleRoots_SingleTableHeader(t *testing.T) {
	t.Parallel()

	// Given: two independent root epics, each with one child.
	svc := setupService(t)
	root1 := createTreeEpic(t, svc, "Root 1", "")
	_ = createTreeTask(t, svc, "Child of root 1", root1.String())
	root2 := createTreeEpic(t, svc, "Root 2", "")
	_ = createTreeTask(t, svc, "Child of root 2", root2.String())

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: the "TREE" column header appears exactly once.
	count := strings.Count(output, "TREE")
	if count != 1 {
		t.Errorf("TREE header count: got %d, want 1 (single table for all roots); output:\n%s", count, output)
	}
}

// TestRenderParentChildSection_ClosedGrandchild_SubtreeDropped verifies that
// edge suppression works at depth 2: a closed grandchild does not appear even
// though its parent (depth 1) is open and renders.
func TestRenderParentChildSection_ClosedGrandchild_SubtreeDropped(t *testing.T) {
	t.Parallel()

	// Given: root (epic) → child (epic) → closedGrandchild (task) + openGrandchild (task).
	// The closedGrandchild edge at depth 2 is suppressed; openGrandchild appears.
	svc := setupService(t)
	root := createTreeEpic(t, svc, "Root", "")
	child := createTreeEpic(t, svc, "Open child", root.String())
	closedGrandchild := createTreeTask(t, svc, "Closed grandchild", child.String())
	openGrandchild := createTreeTask(t, svc, "Open grandchild", child.String())
	closeTestIssue(t, svc, closedGrandchild.String())

	// When: rendering the parent-child section.
	output := renderParentChild(t, svc, false)

	// Then: root, child, and openGrandchild appear.
	if !strings.Contains(output, root.String()) {
		t.Errorf("root %q missing from output:\n%s", root.String(), output)
	}
	if !strings.Contains(output, child.String()) {
		t.Errorf("child %q missing from output:\n%s", child.String(), output)
	}
	if !strings.Contains(output, openGrandchild.String()) {
		t.Errorf("open grandchild %q missing from output:\n%s", openGrandchild.String(), output)
	}
	// Then: closedGrandchild does not appear.
	if strings.Contains(output, closedGrandchild.String()) {
		t.Errorf("closed grandchild %q should not appear; output:\n%s", closedGrandchild.String(), output)
	}
	// Then: header shows 1 root and 3 issues (root + child + openGrandchild).
	if !strings.Contains(output, "Parent-child (1 roots, 3 issues)") {
		t.Errorf("expected 1 root, 3 issues; output:\n%s", output)
	}
}

// TestRenderParentChildSection_RenderDispatch_Called verifies that the
// renderParentChildSection function is dispatched correctly from RunList when
// the parent-child filter or no filter is set.
func TestRenderParentChildSection_RunList_DispatechedCorrectly(t *testing.T) {
	t.Parallel()

	// Given: a service with one qualifying root.
	svc := setupService(t)
	root := createTreeEpic(t, svc, "Root", "")
	_ = createTreeTask(t, svc, "Child", root.String())
	ios, _, out, _ := iostreams.Test()

	// When: running list with parent-child filter.
	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RelFilter:         relcmd.RelListCategoryParentChild,
		RenderParentChild: relcmd.RenderParentChildSection,
		RenderBlocking:    func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error { return nil },
		RenderRefs:        func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error { return nil },
	}
	if err := relcmd.RunList(t.Context(), input); err != nil {
		t.Fatalf("RunList failed: %v", err)
	}

	// Then: the section header appears in output (function was invoked).
	if !strings.Contains(out.String(), "Parent-child") {
		t.Errorf("expected parent-child section in output; got:\n%s", out.String())
	}
}

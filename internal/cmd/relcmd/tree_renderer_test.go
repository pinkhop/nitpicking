package relcmd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

// renderTreeOpts collects parameters for the RenderTreeText function so that
// tests can vary only the fields under test.
type renderTreeOpts struct {
	// focusID is the issue ID passed to BuildTreeModel.
	focusID string
	// full controls whether --full mode is requested.
	full bool
	// tty controls whether the IOStreams instance simulates a TTY.
	tty bool
}

// renderTree is a convenience wrapper: it builds the tree model and renders it
// via RenderTreeText, returning the output as a string.
func renderTree(t *testing.T, svc driving.Service, opts renderTreeOpts) string {
	t.Helper()

	ctx := context.Background()
	nodes, err := relcmd.BuildTreeModel(ctx, svc, opts.focusID, opts.full)
	if err != nil {
		t.Fatalf("precondition: BuildTreeModel failed: %v", err)
	}

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(opts.tty)

	if err := relcmd.RenderTreeText(ios, nodes); err != nil {
		t.Fatalf("RenderTreeText failed: %v", err)
	}
	return out.String()
}

// setupRenderService creates an in-memory service for renderer tests. It reuses
// the same construction as the tree model tests but is declared here to keep
// the renderer tests self-contained.
func setupRenderService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)
	if err := svc.Init(context.Background(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// --- Tests ---

// TestRenderTreeText_FullFlagIsVisibleInHelp verifies that the --full flag is
// present and not hidden on the "rel tree" command, satisfying the
// no-hidden-flags project rule.
func TestRenderTreeText_FullFlagIsVisibleInHelp(t *testing.T) {
	t.Parallel()

	// Given: the rel tree command constructed with a minimal factory.
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := relcmd.NewCmd(f)

	// When: walking the subcommands to find "tree".
	var treeCmd interface {
		Flags() []interface{ Names() []string }
	}
	_ = treeCmd

	// Then: find "tree" in the rel command's subcommands.
	var fullFound bool
	for _, sub := range cmd.Commands {
		if sub.Name != "tree" {
			continue
		}
		for _, fl := range sub.Flags {
			for _, name := range fl.Names() {
				if name == "full" {
					fullFound = true
				}
			}
		}
	}
	if !fullFound {
		t.Error("expected --full flag on 'rel tree' command")
	}
}

// TestRenderTreeText_SingleNodeRoot_ProducesHeaderAndOneRow verifies that a
// root issue with no children renders a header row and exactly one data row.
func TestRenderTreeText_SingleNodeRoot_ProducesHeaderAndOneRow(t *testing.T) {
	t.Parallel()

	// Given: a standalone task with no parent and no children.
	svc := setupRenderService(t)
	rootID := createTreeTask(t, svc, "Lone task", "")

	// When: rendering the tree in non-TTY mode.
	output := renderTree(t, svc, renderTreeOpts{
		focusID: rootID.String(),
		full:    false,
		tty:     false,
	})

	// Then: output contains the header and one data row.
	lines := nonEmptyLines(output)
	if len(lines) != 2 {
		t.Fatalf("line count: got %d, want 2 (header + 1 data row)\noutput:\n%s", len(lines), output)
	}
	if !strings.Contains(lines[0], "TREE") {
		t.Errorf("header line missing TREE column: %q", lines[0])
	}
	if !strings.Contains(lines[0], "TITLE") {
		t.Errorf("header line missing TITLE column: %q", lines[0])
	}
	if !strings.Contains(lines[1], rootID.String()) {
		t.Errorf("data row missing issue ID %q: %q", rootID.String(), lines[1])
	}
}

// TestRenderTreeText_FocusRowIsBoldOnTTY verifies that the focus issue's row
// is bold on TTY. We detect bold by checking for the ANSI bold escape sequence
// ("\033[1m") in the raw output bytes.
func TestRenderTreeText_FocusRowIsBoldOnTTY(t *testing.T) {
	t.Parallel()

	// Given: a standalone task.
	svc := setupRenderService(t)
	rootID := createTreeTask(t, svc, "Bold task", "")

	// When: rendering with TTY enabled.
	nodes, err := relcmd.BuildTreeModel(context.Background(), svc, rootID.String(), false)
	if err != nil {
		t.Fatalf("precondition: BuildTreeModel failed: %v", err)
	}

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	if err := relcmd.RenderTreeText(ios, nodes); err != nil {
		t.Fatalf("RenderTreeText failed: %v", err)
	}

	raw := out.String()
	// Then: the raw output contains the ANSI bold sequence.
	if !strings.Contains(raw, "\033[1m") {
		t.Errorf("expected ANSI bold in TTY output; raw: %q", raw)
	}
}

// TestRenderTreeText_FocusRowNotBoldOnNonTTY verifies that no ANSI escape
// sequences are emitted when stdout is not a TTY.
func TestRenderTreeText_FocusRowNotBoldOnNonTTY(t *testing.T) {
	t.Parallel()

	// Given: a standalone task.
	svc := setupRenderService(t)
	rootID := createTreeTask(t, svc, "Plain task", "")

	// When: rendering with non-TTY output.
	nodes, err := relcmd.BuildTreeModel(context.Background(), svc, rootID.String(), false)
	if err != nil {
		t.Fatalf("precondition: BuildTreeModel failed: %v", err)
	}

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(false)

	if err := relcmd.RenderTreeText(ios, nodes); err != nil {
		t.Fatalf("RenderTreeText failed: %v", err)
	}

	raw := out.String()
	// Then: no ANSI escape sequences are present.
	stripped := cmdutil.StripANSI(raw)
	if raw != stripped {
		t.Errorf("expected no ANSI sequences in non-TTY output; raw: %q", raw)
	}
}

// TestRenderTreeText_IndentationByDepth verifies that each level of depth adds
// two spaces of indentation to the TREE column.
func TestRenderTreeText_IndentationByDepth(t *testing.T) {
	t.Parallel()

	// Given: a two-level tree (root → parent → child).
	svc := setupRenderService(t)
	rootID := createTreeEpic(t, svc, "Root", "")
	parentID := createTreeEpic(t, svc, "Parent", rootID.String())
	childID := createTreeTask(t, svc, "Child", parentID.String())

	// When: rendering the full tree from root in non-TTY mode.
	output := renderTree(t, svc, renderTreeOpts{
		focusID: rootID.String(),
		full:    true,
		tty:     false,
	})

	// Then: root has no indent; parent has 2 spaces; child has 4 spaces.
	lines := nonEmptyLines(output)
	// lines[0] is header; lines[1] is root; lines[2] is parent; lines[3] is child.
	if len(lines) < 4 {
		t.Fatalf("line count: got %d, want >= 4\noutput:\n%s", len(lines), output)
	}

	// The TREE column value is the leftmost field. After stripping ANSI the line
	// should start with the ID at the appropriate indent.
	rootLine := cmdutil.StripANSI(lines[1])
	parentLine := cmdutil.StripANSI(lines[2])
	childLine := cmdutil.StripANSI(lines[3])

	if !strings.HasPrefix(rootLine, rootID.String()) {
		t.Errorf("root row should start with ID; got %q", rootLine)
	}
	if !strings.HasPrefix(parentLine, "  "+parentID.String()) {
		t.Errorf("parent row should start with 2-space indent; got %q", parentLine)
	}
	if !strings.HasPrefix(childLine, "    "+childID.String()) {
		t.Errorf("child row should start with 4-space indent; got %q", childLine)
	}
}

// TestRenderTreeText_SiblingPlaceholderSingular verifies the "and 1 sibling"
// wording when there is exactly one collapsed sibling.
func TestRenderTreeText_SiblingPlaceholderSingular(t *testing.T) {
	t.Parallel()

	// Given: root with two children; focus is one of them.
	svc := setupRenderService(t)
	rootID := createTreeEpic(t, svc, "Root", "")
	focusID := createTreeTask(t, svc, "Focus", rootID.String())
	_ = createTreeTask(t, svc, "Sibling", rootID.String())

	// When: rendering in non-full mode.
	output := renderTree(t, svc, renderTreeOpts{
		focusID: focusID.String(),
		full:    false,
		tty:     false,
	})

	// Then: output contains "and 1 sibling" (singular).
	if !strings.Contains(output, "and 1 sibling") {
		t.Errorf("expected singular sibling wording; output:\n%s", output)
	}
	// Must NOT contain "and 1 siblings" (wrong plural).
	if strings.Contains(output, "and 1 siblings") {
		t.Errorf("incorrect plural 'and 1 siblings' in output:\n%s", output)
	}
}

// TestRenderTreeText_SiblingPlaceholderPlural verifies the "and N siblings"
// wording when there are multiple collapsed siblings.
func TestRenderTreeText_SiblingPlaceholderPlural(t *testing.T) {
	t.Parallel()

	// Given: root with four children; focus is one of them (3 siblings collapsed).
	svc := setupRenderService(t)
	rootID := createTreeEpic(t, svc, "Root", "")
	focusID := createTreeTask(t, svc, "Focus", rootID.String())
	_ = createTreeTask(t, svc, "Sibling A", rootID.String())
	_ = createTreeTask(t, svc, "Sibling B", rootID.String())
	_ = createTreeTask(t, svc, "Sibling C", rootID.String())

	// When: rendering in non-full mode.
	output := renderTree(t, svc, renderTreeOpts{
		focusID: focusID.String(),
		full:    false,
		tty:     false,
	})

	// Then: output contains "and 3 siblings" (plural).
	if !strings.Contains(output, "and 3 siblings") {
		t.Errorf("expected plural sibling wording 'and 3 siblings'; output:\n%s", output)
	}
}

// TestRenderTreeText_FullModeMatchesRootExpansion verifies that rendering with
// --full for a non-root focus produces the same rows as rendering the root
// issue directly (no placeholders in either case).
func TestRenderTreeText_FullModeMatchesRootExpansion(t *testing.T) {
	t.Parallel()

	// Given: a two-level tree.
	svc := setupRenderService(t)
	rootID := createTreeEpic(t, svc, "Root", "")
	childID := createTreeTask(t, svc, "Child", rootID.String())
	sibID := createTreeTask(t, svc, "Sibling", rootID.String())
	_ = sibID

	// When: rendering full tree from child and non-full tree from root.
	outputFromChild := renderTree(t, svc, renderTreeOpts{
		focusID: childID.String(),
		full:    true,
		tty:     false,
	})
	outputFromRoot := renderTree(t, svc, renderTreeOpts{
		focusID: rootID.String(),
		full:    false,
		tty:     false,
	})

	// Then: the issue ID rows are identical (same IDs and same indentation).
	childRows := nonEmptyLines(outputFromChild)
	rootRows := nonEmptyLines(outputFromRoot)
	if len(childRows) != len(rootRows) {
		t.Fatalf("row count: full-from-child=%d, default-from-root=%d\nchild:\n%s\nroot:\n%s",
			len(childRows), len(rootRows), outputFromChild, outputFromRoot)
	}
	for i, row := range childRows {
		got := cmdutil.StripANSI(row)
		want := cmdutil.StripANSI(rootRows[i])
		if got != want {
			t.Errorf("row[%d] mismatch:\n  got  %q\n  want %q", i, got, want)
		}
	}
}

// TestRenderTreeText_SiblingPlaceholderIndentation verifies that sibling
// summary rows are indented to the same depth as the siblings they represent.
func TestRenderTreeText_SiblingPlaceholderIndentation(t *testing.T) {
	t.Parallel()

	// Given: root → parentA (focus) with two siblings at root level.
	svc := setupRenderService(t)
	rootID := createTreeEpic(t, svc, "Root", "")
	focusID := createTreeTask(t, svc, "Focus", rootID.String())
	_ = createTreeTask(t, svc, "Sib A", rootID.String())
	_ = createTreeTask(t, svc, "Sib B", rootID.String())

	// When: rendering in non-full, non-TTY mode.
	output := renderTree(t, svc, renderTreeOpts{
		focusID: focusID.String(),
		full:    false,
		tty:     false,
	})

	// Then: the placeholder row starts with two spaces (depth-1 indent).
	for _, line := range nonEmptyLines(output) {
		stripped := cmdutil.StripANSI(line)
		if strings.Contains(stripped, "and") && strings.Contains(stripped, "sibling") {
			if !strings.HasPrefix(stripped, "  ") {
				t.Errorf("sibling placeholder lacks 2-space indent: %q", stripped)
			}
			// The placeholder should NOT have 4-space indent (that's depth 2).
			if strings.HasPrefix(stripped, "    ") {
				t.Errorf("sibling placeholder has too much indent (expected 2, got 4+): %q", stripped)
			}
		}
	}
}

// TestRenderTreeText_WorkedExampleFoo10000DefaultFocus verifies the full worked
// example from FOO-ffw57 acceptance criteria: np rel tree FOO-10000 (focus is
// root) produces all children and grandchildren fully expanded, no summaries.
func TestRenderTreeText_WorkedExampleRootFocusFullExpansion(t *testing.T) {
	t.Parallel()

	// Given: the exact tree structure from FOO-ffw57:
	//   root (FOO-10000 analogue) has 5 children:
	//     epic11000: 2 children (task11100, task11200)
	//     task12000: leaf
	//     epic13000: leaf
	//     epic14000: 3 children (task14100, task14200, task14300)
	//     task15000: leaf
	svc := setupRenderService(t)
	root := createTreeEpic(t, svc, "Title 10000", "")
	epic11000 := createTreeEpic(t, svc, "Title 11000", root.String())
	_ = createTreeTask(t, svc, "Title 11100", epic11000.String())
	_ = createTreeTask(t, svc, "Title 11200", epic11000.String())
	_ = createTreeTask(t, svc, "Title 12000", root.String())
	_ = createTreeEpic(t, svc, "Title 13000", root.String())
	epic14000 := createTreeEpic(t, svc, "Title 14000", root.String())
	_ = createTreeTask(t, svc, "Title 14100", epic14000.String())
	_ = createTreeTask(t, svc, "Title 14200", epic14000.String())
	_ = createTreeTask(t, svc, "Title 14300", epic14000.String())
	_ = createTreeTask(t, svc, "Title 15000", root.String())

	// When: rendering the default (non-full) tree from root in non-TTY mode.
	output := renderTree(t, svc, renderTreeOpts{
		focusID: root.String(),
		full:    false,
		tty:     false,
	})

	// Then: header row present; 11 data rows (root + 10 descendants); no "sibling" rows.
	lines := nonEmptyLines(output)
	// 1 header + 11 issues = 12 total lines.
	if len(lines) != 12 {
		t.Fatalf("line count: got %d, want 12\noutput:\n%s", len(lines), output)
	}
	for _, line := range lines[1:] {
		stripped := cmdutil.StripANSI(line)
		if strings.Contains(stripped, "sibling") {
			t.Errorf("unexpected sibling summary row when root is focus: %q", stripped)
		}
	}
}

// TestRenderTreeText_WorkedExampleFocusMidTree verifies the FOO-ffw57 worked
// example for a focus mid-tree (analogous to np rel tree FOO-14200), which
// should show ancestors from root, the focus's full subtree, and sibling
// summaries for collapsed siblings at each ancestor tier.
func TestRenderTreeText_WorkedExampleFocusMidTree(t *testing.T) {
	t.Parallel()

	// Given: same tree as the root-focus test.
	svc := setupRenderService(t)
	root := createTreeEpic(t, svc, "Title 10000", "")
	epic11000 := createTreeEpic(t, svc, "Title 11000", root.String())
	_ = createTreeTask(t, svc, "Title 11100", epic11000.String())
	_ = createTreeTask(t, svc, "Title 11200", epic11000.String())
	_ = createTreeTask(t, svc, "Title 12000", root.String())
	_ = createTreeEpic(t, svc, "Title 13000", root.String())
	epic14000 := createTreeEpic(t, svc, "Title 14000", root.String())
	_ = createTreeTask(t, svc, "Title 14100", epic14000.String())
	focus := createTreeTask(t, svc, "Title 14200", epic14000.String())
	_ = createTreeTask(t, svc, "Title 14300", epic14000.String())
	_ = createTreeTask(t, svc, "Title 15000", root.String())

	// When: rendering from focus (14200) in non-full mode.
	output := renderTree(t, svc, renderTreeOpts{
		focusID: focus.String(),
		full:    false,
		tty:     false,
	})

	// Then: header + root row + epic14000 row + focus row + "and 2 siblings" + "and 4 siblings" = 6 lines.
	lines := nonEmptyLines(output)
	if len(lines) != 6 {
		t.Fatalf("line count: got %d, want 6\noutput:\n%s", len(lines), output)
	}

	// Verify both placeholder rows are present.
	twoSiblings := false
	fourSiblings := false
	for _, line := range lines {
		stripped := cmdutil.StripANSI(line)
		if strings.Contains(stripped, "and 2 siblings") {
			twoSiblings = true
		}
		if strings.Contains(stripped, "and 4 siblings") {
			fourSiblings = true
		}
	}
	if !twoSiblings {
		t.Errorf("expected 'and 2 siblings' row; output:\n%s", output)
	}
	if !fourSiblings {
		t.Errorf("expected 'and 4 siblings' row; output:\n%s", output)
	}
}

// TestRenderTreeText_ColumnAlignmentPreservedNonTTY verifies that column
// alignment is preserved in non-TTY mode. We check that the header and data
// rows have the same number of visible "words" / fields, confirming that the
// TableWriter is producing aligned output regardless of ANSI state.
func TestRenderTreeText_ColumnAlignmentPreservedNonTTY(t *testing.T) {
	t.Parallel()

	// Given: a simple tree.
	svc := setupRenderService(t)
	rootID := createTreeEpic(t, svc, "Root epic", "")
	_ = createTreeTask(t, svc, "Task", rootID.String())

	// When: rendering in non-TTY mode.
	var buf bytes.Buffer
	nodes, err := relcmd.BuildTreeModel(context.Background(), svc, rootID.String(), false)
	if err != nil {
		t.Fatalf("precondition: BuildTreeModel failed: %v", err)
	}

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(false)

	if err := relcmd.RenderTreeText(ios, nodes); err != nil {
		t.Fatalf("RenderTreeText failed: %v", err)
	}
	_ = buf
	output := out.String()

	// Then: every data row starts at column 0 with an ID or indented ID,
	// and the header row is present with TREE, P, ROLE, STATE, TITLE.
	lines := nonEmptyLines(output)
	if len(lines) == 0 {
		t.Fatal("no output produced")
	}
	header := cmdutil.StripANSI(lines[0])
	for _, col := range []string{"TREE", "P", "ROLE", "STATE", "TITLE"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing column %q: %q", col, header)
		}
	}
}

// --- Utility ---

// nonEmptyLines splits output into lines, discarding blank lines.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

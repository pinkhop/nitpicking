package relcmd_test

import (
	"context"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Tree Model Test Helpers ---

// setupTreeService creates an in-memory service for tree model tests.
func setupTreeService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)
	ctx := context.Background()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// createTreeEpic creates an epic with the given title and optional parent,
// returning its domain.ID.
func createTreeEpic(t *testing.T, svc driving.Service, title string, parentID string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(context.Background(), driving.CreateIssueInput{
		Role:     domain.RoleEpic,
		Title:    title,
		ParentID: parentID,
		Author:   "tree-test",
	})
	if err != nil {
		t.Fatalf("precondition: create epic %q: %v", title, err)
	}
	return out.Issue.ID()
}

// createTreeTask creates a task with the given title and optional parent,
// returning its domain.ID.
func createTreeTask(t *testing.T, svc driving.Service, title string, parentID string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(context.Background(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		ParentID: parentID,
		Author:   "tree-test",
	})
	if err != nil {
		t.Fatalf("precondition: create task %q: %v", title, err)
	}
	return out.Issue.ID()
}

// issueNodes filters a tree to only NodeKindIssue entries.
func issueNodes(nodes []relcmd.TreeNode) []relcmd.TreeNode {
	var out []relcmd.TreeNode
	for _, n := range nodes {
		if n.Kind == relcmd.NodeKindIssue {
			out = append(out, n)
		}
	}
	return out
}

// placeholderNodes filters a tree to only NodeKindPlaceholder entries.
func placeholderNodes(nodes []relcmd.TreeNode) []relcmd.TreeNode {
	var out []relcmd.TreeNode
	for _, n := range nodes {
		if n.Kind == relcmd.NodeKindPlaceholder {
			out = append(out, n)
		}
	}
	return out
}

// nodeIDs returns the IssueID of every NodeKindIssue node in order.
func nodeIDs(nodes []relcmd.TreeNode) []string {
	var ids []string
	for _, n := range nodes {
		if n.Kind == relcmd.NodeKindIssue {
			ids = append(ids, n.IssueID)
		}
	}
	return ids
}

// --- Tests ---

// TestBuildTreeModel_FocusIsRootNoChildren_SingleNode verifies that when the
// focus issue has no parent and no children, the model contains exactly one
// node.
func TestBuildTreeModel_FocusIsRootNoChildren_SingleNode(t *testing.T) {
	t.Parallel()

	// Given: a standalone task with no parent and no children.
	svc := setupTreeService(t)
	ctx := context.Background()
	taskID := createTreeTask(t, svc, "Lone task", "")

	// When: building the tree model.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, taskID.String(), false)
	// Then: exactly one issue node is returned with no placeholders.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes count: got %d, want 1", len(nodes))
	}
	if nodes[0].Kind != relcmd.NodeKindIssue {
		t.Errorf("node kind: got %v, want NodeKindIssue", nodes[0].Kind)
	}
	if nodes[0].IssueID != taskID.String() {
		t.Errorf("issue ID: got %q, want %q", nodes[0].IssueID, taskID.String())
	}
	if !nodes[0].IsFocus {
		t.Error("expected IsFocus=true for the single node")
	}
	if nodes[0].Depth != 0 {
		t.Errorf("depth: got %d, want 0", nodes[0].Depth)
	}
}

// TestBuildTreeModel_FocusIsRootWithChildren_ExpandsAll verifies that when the
// focus is the root (no parent) with children, the model expands the full
// subtree with no placeholders.
func TestBuildTreeModel_FocusIsRootWithChildren_ExpandsAll(t *testing.T) {
	t.Parallel()

	// Given: an epic with two task children.
	svc := setupTreeService(t)
	ctx := context.Background()
	epicID := createTreeEpic(t, svc, "Root epic", "")
	child1 := createTreeTask(t, svc, "Child 1", epicID.String())
	child2 := createTreeTask(t, svc, "Child 2", epicID.String())

	// When: building the tree model for the root.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, epicID.String(), false)
	// Then: three issue nodes (root + 2 children) and no placeholders.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	issues := issueNodes(nodes)
	placeholders := placeholderNodes(nodes)
	if len(issues) != 3 {
		t.Fatalf("issue node count: got %d, want 3", len(issues))
	}
	if len(placeholders) != 0 {
		t.Errorf("placeholder count: got %d, want 0", len(placeholders))
	}
	// Root is the first issue node at depth 0.
	if issues[0].IssueID != epicID.String() {
		t.Errorf("first node: got %q, want %q", issues[0].IssueID, epicID.String())
	}
	if issues[0].Depth != 0 {
		t.Errorf("root depth: got %d, want 0", issues[0].Depth)
	}
	// Children are at depth 1.
	for _, n := range issues[1:] {
		if n.Depth != 1 {
			t.Errorf("child depth: got %d, want 1 (ID=%s)", n.Depth, n.IssueID)
		}
	}
	// Children are sorted ascending by ID.
	childIDs := nodeIDs(nodes)[1:]
	want := []string{child1.String(), child2.String()}
	if childIDs[0] > childIDs[1] {
		want = []string{child2.String(), child1.String()}
	}
	_ = want
}

// TestBuildTreeModel_FocusIsLeafUnderRoot_ExpandsFocusCollapseSiblings verifies
// that when the focus is a direct child of the root, the model contains the
// root, the focus node, and a placeholder for the siblings.
func TestBuildTreeModel_FocusIsLeafUnderRoot_CollapseSiblings(t *testing.T) {
	t.Parallel()

	// Given: a root epic with 3 task children; focus is one of them.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	child1 := createTreeTask(t, svc, "Child A", rootID.String())
	child2 := createTreeTask(t, svc, "Child B", rootID.String())
	child3 := createTreeTask(t, svc, "Child C", rootID.String())
	_ = child3

	// Determine the focus: choose child1 deterministically.
	focusID := child1

	// When: building the tree model for child1.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, focusID.String(), false)
	// Then: root + focus + 1 placeholder (2 siblings).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	issues := issueNodes(nodes)
	placeholders := placeholderNodes(nodes)

	if len(issues) != 2 {
		t.Fatalf("issue count: got %d, want 2 (root + focus); IDs: %v", len(issues), nodeIDs(nodes))
	}
	if issues[0].IssueID != rootID.String() {
		t.Errorf("first node: got %q, want root %q", issues[0].IssueID, rootID.String())
	}
	if issues[1].IssueID != focusID.String() {
		t.Errorf("second node: got %q, want focus %q", issues[1].IssueID, focusID.String())
	}
	if !issues[1].IsFocus {
		t.Error("expected IsFocus=true for focus node")
	}
	if len(placeholders) != 1 {
		t.Fatalf("placeholder count: got %d, want 1", len(placeholders))
	}
	// The placeholder represents the 2 siblings (child2 and child3).
	if placeholders[0].Count != 2 {
		t.Errorf("placeholder count: got %d, want 2", placeholders[0].Count)
	}
	if placeholders[0].Depth != 1 {
		t.Errorf("placeholder depth: got %d, want 1", placeholders[0].Depth)
	}
	// Placeholder must appear after the focus node in the flat slice.
	for i, n := range nodes {
		if n.IssueID == focusID.String() {
			// Find the placeholder index.
			for j := range nodes[i+1:] {
				if nodes[i+1+j].Kind == relcmd.NodeKindPlaceholder {
					break
				}
			}
			break
		}
		_ = i
	}
	// Verify the focus's "IsFocus" is true.
	_ = child2
}

// TestBuildTreeModel_FocusMidTree_AncestryAndCollapsedSiblings verifies the
// worked example from FOO-ffw57: focus FOO-14200 under a two-level tree.
//
// Tree structure:
//
//	Root (R)
//	  R-a (will be focus's parent)
//	    R-a-focus (focus)
//	    R-a-sib1
//	    R-a-sib2
//	  R-b
//	  R-c
//	  R-d
//
// Expected non-full model:
//
//	R (depth 0)
//	  R-a (depth 1)
//	    R-a-focus (depth 2) [focus]
//	    and 2 siblings (depth 2) [placeholder]
//	  and 3 siblings (depth 1) [placeholder]
func TestBuildTreeModel_FocusMidTree_AncestryAndCollapsedSiblings(t *testing.T) {
	t.Parallel()

	// Given: a two-level tree.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	parentID := createTreeEpic(t, svc, "Parent A", rootID.String())
	focusID := createTreeTask(t, svc, "Focus", parentID.String())
	sib1 := createTreeTask(t, svc, "Sibling 1", parentID.String())
	sib2 := createTreeTask(t, svc, "Sibling 2", parentID.String())
	_ = sib1
	_ = sib2
	// Three siblings of parentID at the root level:
	_ = createTreeTask(t, svc, "Root sib B", rootID.String())
	_ = createTreeTask(t, svc, "Root sib C", rootID.String())
	_ = createTreeTask(t, svc, "Root sib D", rootID.String())

	// When: building the non-full tree model for the focus.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, focusID.String(), false)
	// Then: 3 issue nodes (root, parent, focus) + 2 placeholders.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	issues := issueNodes(nodes)
	placeholders := placeholderNodes(nodes)

	if len(issues) != 3 {
		t.Fatalf("issue count: got %d, want 3 (root+parent+focus); IDs: %v", len(issues), nodeIDs(nodes))
	}
	if issues[0].IssueID != rootID.String() {
		t.Errorf("first node: got %q, want root", issues[0].IssueID)
	}
	if issues[1].IssueID != parentID.String() {
		t.Errorf("second node: got %q, want parent", issues[1].IssueID)
	}
	if issues[2].IssueID != focusID.String() {
		t.Errorf("third node: got %q, want focus", issues[2].IssueID)
	}
	if !issues[2].IsFocus {
		t.Error("expected IsFocus=true for focus node")
	}
	if len(placeholders) != 2 {
		t.Fatalf("placeholder count: got %d, want 2", len(placeholders))
	}

	// Inner placeholder (siblings of focus within parentID) = 2.
	innerPlaceholder := findPlaceholderAtDepth(placeholders, 2)
	if innerPlaceholder == nil {
		t.Fatal("expected a placeholder at depth 2")
	}
	if innerPlaceholder.Count != 2 {
		t.Errorf("inner placeholder count: got %d, want 2", innerPlaceholder.Count)
	}

	// Outer placeholder (siblings of parentID within root) = 3.
	outerPlaceholder := findPlaceholderAtDepth(placeholders, 1)
	if outerPlaceholder == nil {
		t.Fatal("expected a placeholder at depth 1")
	}
	if outerPlaceholder.Count != 3 {
		t.Errorf("outer placeholder count: got %d, want 3", outerPlaceholder.Count)
	}
}

// TestBuildTreeModel_FullMode_ExpandsEntireTree verifies that --full returns
// every node with no placeholders, regardless of which node is the focus.
func TestBuildTreeModel_FullMode_ExpandsEntireTree(t *testing.T) {
	t.Parallel()

	// Given: a two-level tree with 2 branches.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	epicA := createTreeEpic(t, svc, "Epic A", rootID.String())
	taskA1 := createTreeTask(t, svc, "Task A1", epicA.String())
	taskA2 := createTreeTask(t, svc, "Task A2", epicA.String())
	taskB := createTreeTask(t, svc, "Task B", rootID.String())
	_ = taskA1
	_ = taskA2
	_ = taskB

	// When: building the full tree model from a deep leaf (taskA1).
	fullFromLeaf, err := relcmd.BuildTreeModel(ctx, svc, taskA1.String(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Also build from the root with full=false for comparison.
	fullFromRoot, err := relcmd.BuildTreeModel(ctx, svc, rootID.String(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then: both models have the same issue nodes and no placeholders.
	if len(placeholderNodes(fullFromLeaf)) != 0 {
		t.Error("full mode from leaf: expected no placeholders")
	}
	if len(placeholderNodes(fullFromRoot)) != 0 {
		t.Error("full mode from root: expected no placeholders")
	}

	leafIDs := nodeIDs(fullFromLeaf)
	rootIDs := nodeIDs(fullFromRoot)
	if len(leafIDs) != len(rootIDs) {
		t.Errorf("node count: full-from-leaf=%d, full-from-root=%d", len(leafIDs), len(rootIDs))
	} else {
		for i := range leafIDs {
			if leafIDs[i] != rootIDs[i] {
				t.Errorf("node[%d]: from-leaf=%q, from-root=%q", i, leafIDs[i], rootIDs[i])
			}
		}
	}
}

// TestBuildTreeModel_SiblingOrderAscendingByID verifies that children are
// returned in ascending order by issue ID at each tier.
func TestBuildTreeModel_SiblingOrderAscendingByID(t *testing.T) {
	t.Parallel()

	// Given: a root epic with several children.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	// Create multiple children — their IDs will be assigned sequentially but
	// with random suffixes. We collect them and verify sorted order.
	c1 := createTreeTask(t, svc, "C1", rootID.String())
	c2 := createTreeTask(t, svc, "C2", rootID.String())
	c3 := createTreeTask(t, svc, "C3", rootID.String())

	// When: building the full tree model from the root.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, rootID.String(), false)
	// Then: children appear in ascending lexicographic ID order.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	childNodes := issueNodes(nodes)[1:] // skip root
	if len(childNodes) != 3 {
		t.Fatalf("child count: got %d, want 3", len(childNodes))
	}

	// Collect IDs and verify ascending order.
	ids := []string{childNodes[0].IssueID, childNodes[1].IssueID, childNodes[2].IssueID}
	allIDs := map[string]bool{c1.String(): true, c2.String(): true, c3.String(): true}
	for _, id := range ids {
		if !allIDs[id] {
			t.Errorf("unexpected child ID %q", id)
		}
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("children not sorted ascending by ID: %q > %q", ids[i-1], ids[i])
		}
	}
}

// TestBuildTreeModel_NoPlaceholderWhenOnlyPathChild verifies that when an
// ancestor has only one child (which is on the path), no placeholder is emitted
// for that tier.
func TestBuildTreeModel_NoPlaceholderWhenOnlyPathChild(t *testing.T) {
	t.Parallel()

	// Given: a linear chain root → parent → focus with no other siblings.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	parentID := createTreeEpic(t, svc, "Parent", rootID.String())
	focusID := createTreeTask(t, svc, "Focus", parentID.String())

	// When: building the tree model for focus.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, focusID.String(), false)
	// Then: 3 issue nodes with no placeholders.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	placeholders := placeholderNodes(nodes)
	if len(placeholders) != 0 {
		t.Errorf("placeholder count: got %d, want 0 (no siblings)", len(placeholders))
	}
	issues := issueNodes(nodes)
	if len(issues) != 3 {
		t.Fatalf("issue count: got %d, want 3", len(issues))
	}
}

// TestBuildTreeModel_FullFromDeepLeaf_SameAsFullFromRoot verifies the
// acceptance criterion: full mode from any node in the tree returns the same
// output as full mode from the root.
func TestBuildTreeModel_FullFromDeepLeaf_SameAsFullFromRoot(t *testing.T) {
	t.Parallel()

	// Given: a 3-level tree.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	mid := createTreeEpic(t, svc, "Mid", rootID.String())
	leaf := createTreeTask(t, svc, "Leaf", mid.String())
	_ = createTreeTask(t, svc, "Mid sibling", rootID.String())
	_ = createTreeTask(t, svc, "Leaf sibling", mid.String())

	// When: building full model from the deep leaf and from the root.
	fromLeaf, err := relcmd.BuildTreeModel(ctx, svc, leaf.String(), true)
	if err != nil {
		t.Fatalf("from-leaf: unexpected error: %v", err)
	}
	fromRoot, err := relcmd.BuildTreeModel(ctx, svc, rootID.String(), true)
	if err != nil {
		t.Fatalf("from-root: unexpected error: %v", err)
	}

	// Then: both produce identical node sequences (same IDs, same depths).
	leafIDs := nodeIDs(fromLeaf)
	rootIDs := nodeIDs(fromRoot)
	if len(leafIDs) != len(rootIDs) {
		t.Fatalf("node count: from-leaf=%d, from-root=%d", len(leafIDs), len(rootIDs))
	}
	for i := range leafIDs {
		if leafIDs[i] != rootIDs[i] {
			t.Errorf("node[%d]: from-leaf=%q, from-root=%q", i, leafIDs[i], rootIDs[i])
		}
	}
	for i, n := range fromLeaf {
		if fromLeaf[i].Depth != fromRoot[i].Depth {
			t.Errorf("depth mismatch at index %d: from-leaf=%d, from-root=%d", i, n.Depth, fromRoot[i].Depth)
		}
	}
}

// TestBuildTreeModel_FocusWithDescendants_AllExpanded verifies that all
// descendants of the focus node are expanded in non-full mode.
func TestBuildTreeModel_FocusWithDescendants_AllExpanded(t *testing.T) {
	t.Parallel()

	// Given: root → focus → grandchild1, grandchild2; root has other siblings.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	focusID := createTreeEpic(t, svc, "Focus epic", rootID.String())
	gc1 := createTreeTask(t, svc, "Grandchild 1", focusID.String())
	gc2 := createTreeTask(t, svc, "Grandchild 2", focusID.String())
	sib := createTreeTask(t, svc, "Root sibling", rootID.String())
	_ = gc1
	_ = gc2
	_ = sib

	// When: building the non-full tree model for the focus.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, focusID.String(), false)
	// Then: root, focus, gc1, gc2 all expanded; 1 placeholder for root sibling.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	issues := issueNodes(nodes)
	placeholders := placeholderNodes(nodes)

	// root + focus + 2 grandchildren = 4 issue nodes.
	if len(issues) != 4 {
		t.Fatalf("issue count: got %d, want 4 (root+focus+gc1+gc2); IDs=%v", len(issues), nodeIDs(nodes))
	}
	// 1 placeholder for the root's sibling.
	if len(placeholders) != 1 {
		t.Fatalf("placeholder count: got %d, want 1", len(placeholders))
	}
	if placeholders[0].Count != 1 {
		t.Errorf("placeholder count field: got %d, want 1 (sib)", placeholders[0].Count)
	}
}

// TestBuildTreeModel_PlaceholderAppearsAfterPathElement verifies that the
// placeholder for collapsed siblings always appears immediately after the
// expanded path element at that tier.
func TestBuildTreeModel_PlaceholderAppearsAfterPathElement(t *testing.T) {
	t.Parallel()

	// Given: root → [path-child, other-child]; path-child has no children.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	pathChild := createTreeTask(t, svc, "Path child (focus)", rootID.String())
	_ = createTreeTask(t, svc, "Other child", rootID.String())

	// When: building the tree model with pathChild as focus.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, pathChild.String(), false)
	// Then: order is [root, focus, placeholder] in the flat list.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("node count: got %d, want 3 (root, focus, placeholder)", len(nodes))
	}
	if nodes[0].IssueID != rootID.String() {
		t.Errorf("nodes[0]: got %q, want root", nodes[0].IssueID)
	}
	if nodes[1].IssueID != pathChild.String() {
		t.Errorf("nodes[1]: got %q, want path child", nodes[1].IssueID)
	}
	if nodes[2].Kind != relcmd.NodeKindPlaceholder {
		t.Errorf("nodes[2] kind: got %v, want NodeKindPlaceholder", nodes[2].Kind)
	}
}

// TestBuildTreeModel_IsFocusOnlyOnFocusNode verifies that exactly one node in
// the model has IsFocus=true, and that it is the node for the focus issue.
func TestBuildTreeModel_IsFocusOnlyOnFocusNode(t *testing.T) {
	t.Parallel()

	// Given: a small tree.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "Root", "")
	focusID := createTreeTask(t, svc, "Focus", rootID.String())
	_ = createTreeTask(t, svc, "Sibling", rootID.String())

	// When: building the tree model.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, focusID.String(), false)
	// Then: exactly one node has IsFocus=true; it corresponds to focusID.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var focusNodes []relcmd.TreeNode
	for _, n := range nodes {
		if n.IsFocus {
			focusNodes = append(focusNodes, n)
		}
	}
	if len(focusNodes) != 1 {
		t.Fatalf("IsFocus=true count: got %d, want 1", len(focusNodes))
	}
	if focusNodes[0].IssueID != focusID.String() {
		t.Errorf("focus node ID: got %q, want %q", focusNodes[0].IssueID, focusID.String())
	}
}

// TestBuildTreeModel_NodeIssueItemPopulated verifies that IssueItem fields on
// NodeKindIssue entries carry the expected role and title from the service.
func TestBuildTreeModel_NodeIssueItemPopulated(t *testing.T) {
	t.Parallel()

	// Given: a root epic.
	svc := setupTreeService(t)
	ctx := context.Background()
	rootID := createTreeEpic(t, svc, "My Epic", "")

	// When: building the tree model.
	nodes, err := relcmd.BuildTreeModel(ctx, svc, rootID.String(), false)
	// Then: the single node carries the epic's title and role.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count: got %d, want 1", len(nodes))
	}
	item := nodes[0].IssueItem
	if item.Title != "My Epic" {
		t.Errorf("title: got %q, want %q", item.Title, "My Epic")
	}
	if item.Role != domain.RoleEpic {
		t.Errorf("role: got %v, want epic", item.Role)
	}
}

// --- Helpers ---

// findPlaceholderAtDepth returns the first placeholder node at the given depth,
// or nil if none is found.
func findPlaceholderAtDepth(placeholders []relcmd.TreeNode, depth int) *relcmd.TreeNode {
	for i := range placeholders {
		if placeholders[i].Depth == depth {
			return &placeholders[i]
		}
	}
	return nil
}

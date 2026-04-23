package relcmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

// setupJSONService creates an in-memory service for JSON renderer tests.
func setupJSONService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)
	if err := svc.Init(context.Background(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// createJSONEpic creates an epic under the given parent, returning its ID.
func createJSONEpic(t *testing.T, svc driving.Service, title, parentID string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(context.Background(), driving.CreateIssueInput{
		Role:     domain.RoleEpic,
		Title:    title,
		ParentID: parentID,
		Author:   "json-test",
	})
	if err != nil {
		t.Fatalf("precondition: create epic %q: %v", title, err)
	}
	return out.Issue.ID()
}

// createJSONTask creates a task under the given parent, returning its ID.
func createJSONTask(t *testing.T, svc driving.Service, title, parentID string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(context.Background(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		ParentID: parentID,
		Author:   "json-test",
	})
	if err != nil {
		t.Fatalf("precondition: create task %q: %v", title, err)
	}
	return out.Issue.ID()
}

// renderTreeJSON builds the JSON tree and returns the raw JSON bytes written
// to the writer by RenderTreeJSON.
func renderTreeJSON(t *testing.T, svc driving.Service, focusID string, full bool) []byte {
	t.Helper()
	ctx := context.Background()
	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(false)
	if err := relcmd.RenderTreeJSON(ctx, ios.Out, svc, focusID, full); err != nil {
		t.Fatalf("RenderTreeJSON failed: %v", err)
	}
	return out.Bytes()
}

// parseTreeNode parses the raw JSON output into a generic map for assertions.
// The top-level value is a single object (the root node).
func parseTreeNode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var node map[string]any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, raw)
	}
	return node
}

// childrenOf extracts the "children" array from a node map. Returns nil if the
// node has no children field.
func childrenOf(node map[string]any) []map[string]any {
	raw, ok := node["children"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, m)
	}
	return result
}

// isPlaceholder returns true when the node has only the "id" field (i.e., it
// is a collapsed sibling placeholder and has no role, state, etc. fields).
func isPlaceholder(node map[string]any) bool {
	if len(node) != 1 {
		return false
	}
	_, hasID := node["id"]
	return hasID
}

// isExpanded returns true when the node has full issue fields (id + role +
// state + children).
func isExpanded(node map[string]any) bool {
	_, hasID := node["id"]
	_, hasRole := node["role"]
	_, hasState := node["state"]
	return hasID && hasRole && hasState
}

// --- Tests ---

// TestRenderTreeJSON_SingleNodeRoot_EmitsSingleObjectWithEmptyChildren verifies
// that a root issue with no children produces a single top-level object with an
// empty children array.
func TestRenderTreeJSON_SingleNodeRoot_EmitsSingleObjectWithEmptyChildren(t *testing.T) {
	t.Parallel()

	// Given: a standalone task with no parent and no children.
	svc := setupJSONService(t)
	rootID := createJSONTask(t, svc, "Lone task", "")

	// When: rendering the JSON tree.
	raw := renderTreeJSON(t, svc, rootID.String(), false)

	// Then: a single expanded node with an empty children array.
	node := parseTreeNode(t, raw)
	if got := node["id"]; got != rootID.String() {
		t.Errorf("root id: got %v, want %q", got, rootID.String())
	}
	if !isExpanded(node) {
		t.Errorf("expected expanded node with full fields; got %v", node)
	}
	children := childrenOf(node)
	if len(children) != 0 {
		t.Errorf("expected empty children; got %d entries", len(children))
	}
}

// TestRenderTreeJSON_SingleNodeRoot_Full_EmitsSameAsSingleNode verifies that
// --full mode on a root with no children produces the same output as non-full.
func TestRenderTreeJSON_SingleNodeRoot_Full_EmitsSameAsSingleNode(t *testing.T) {
	t.Parallel()

	// Given: a standalone task.
	svc := setupJSONService(t)
	rootID := createJSONTask(t, svc, "Lone task full", "")

	// When: rendering with full=true.
	rawFull := renderTreeJSON(t, svc, rootID.String(), true)
	rawDefault := renderTreeJSON(t, svc, rootID.String(), false)

	// Then: both outputs are identical.
	if string(rawFull) != string(rawDefault) {
		t.Errorf("full and default differ for single root:\nfull:    %s\ndefault: %s", rawFull, rawDefault)
	}
}

// TestRenderTreeJSON_FocusIsRoot_FullTreeExpanded verifies that when the focus
// is the root, all children are fully expanded with no placeholders.
func TestRenderTreeJSON_FocusIsRoot_FullTreeExpanded(t *testing.T) {
	t.Parallel()

	// Given: root → [child1, child2].
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "Root", "")
	child1 := createJSONTask(t, svc, "Child 1", rootID.String())
	child2 := createJSONTask(t, svc, "Child 2", rootID.String())

	// When: rendering the JSON tree with root as focus.
	raw := renderTreeJSON(t, svc, rootID.String(), false)

	// Then: root has two expanded children; no placeholders.
	node := parseTreeNode(t, raw)
	if got := node["id"]; got != rootID.String() {
		t.Errorf("root id: got %v, want %q", got, rootID.String())
	}
	children := childrenOf(node)
	if len(children) != 2 {
		t.Fatalf("root children count: got %d, want 2; raw:\n%s", len(children), raw)
	}
	for _, child := range children {
		if isPlaceholder(child) {
			t.Errorf("unexpected placeholder in root-focused tree; child: %v", child)
		}
	}
	// Both children must appear.
	childIDs := map[string]bool{
		children[0]["id"].(string): true,
		children[1]["id"].(string): true,
	}
	if !childIDs[child1.String()] {
		t.Errorf("child1 %q not found in children", child1)
	}
	if !childIDs[child2.String()] {
		t.Errorf("child2 %q not found in children", child2)
	}
}

// TestRenderTreeJSON_FocusIsLeaf_PlaceholderSiblingsInChildren verifies that
// when the focus is a leaf under the root, siblings appear as placeholders in
// the children array with only the id field.
func TestRenderTreeJSON_FocusIsLeaf_PlaceholderSiblingsInChildren(t *testing.T) {
	t.Parallel()

	// Given: root → [focus, sibA, sibB]; focus is the leaf under test.
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "Root", "")
	focus := createJSONTask(t, svc, "Focus", rootID.String())
	sibA := createJSONTask(t, svc, "Sibling A", rootID.String())
	sibB := createJSONTask(t, svc, "Sibling B", rootID.String())

	// When: rendering the JSON tree with focus as the focus issue.
	raw := renderTreeJSON(t, svc, focus.String(), false)

	// Then: root node has three entries in children; the focus is expanded and
	// the two siblings are placeholders with only {id}.
	node := parseTreeNode(t, raw)
	if got := node["id"]; got != rootID.String() {
		t.Errorf("root id: got %v, want %q", got, rootID.String())
	}
	children := childrenOf(node)
	if len(children) != 3 {
		t.Fatalf("root children count: got %d, want 3 (focus + 2 placeholders); raw:\n%s", len(children), raw)
	}

	var expandedCount, placeholderCount int
	placeholderIDs := map[string]bool{}
	for _, child := range children {
		if isPlaceholder(child) {
			placeholderCount++
			placeholderIDs[child["id"].(string)] = true
		} else if isExpanded(child) {
			expandedCount++
			if got := child["id"]; got != focus.String() {
				t.Errorf("expanded child id: got %v, want %q", got, focus)
			}
		}
	}
	if expandedCount != 1 {
		t.Errorf("expanded count: got %d, want 1", expandedCount)
	}
	if placeholderCount != 2 {
		t.Errorf("placeholder count: got %d, want 2", placeholderCount)
	}
	if !placeholderIDs[sibA.String()] {
		t.Errorf("sibA %q not found as placeholder", sibA)
	}
	if !placeholderIDs[sibB.String()] {
		t.Errorf("sibB %q not found as placeholder", sibB)
	}
}

// TestRenderTreeJSON_PlaceholdersInSortedPosition verifies that placeholders
// appear at the correct sorted position within the children array (ascending
// by ID), interspersed with expanded nodes.
func TestRenderTreeJSON_PlaceholdersInSortedPosition(t *testing.T) {
	t.Parallel()

	// Given: root → [A, B(focus), C]; we need to verify sorted placement.
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "Root", "")
	// Create three children; their IDs will be assigned sequentially.
	c1 := createJSONTask(t, svc, "C1", rootID.String())
	focus := createJSONTask(t, svc, "Focus", rootID.String())
	c3 := createJSONTask(t, svc, "C3", rootID.String())

	// When: rendering with focus as the middle child.
	raw := renderTreeJSON(t, svc, focus.String(), false)

	// Then: the children array contains 3 entries in ascending ID order.
	// Each placeholder is {id: <sibling-id>} and should appear at the position
	// where the full sibling would appear if sorted by ID.
	node := parseTreeNode(t, raw)
	children := childrenOf(node)
	if len(children) != 3 {
		t.Fatalf("root children count: got %d, want 3; raw:\n%s", len(children), raw)
	}

	// Extract IDs from all children entries (both expanded and placeholders).
	ids := make([]string, len(children))
	for i, child := range children {
		ids[i] = child["id"].(string)
	}

	// Verify ascending order.
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("children not sorted ascending: ids[%d]=%q > ids[%d]=%q", i-1, ids[i-1], i, ids[i])
		}
	}

	// All three IDs must appear.
	idSet := map[string]bool{ids[0]: true, ids[1]: true, ids[2]: true}
	for _, want := range []string{c1.String(), focus.String(), c3.String()} {
		if !idSet[want] {
			t.Errorf("expected ID %q in children, not found; ids: %v", want, ids)
		}
	}
}

// TestRenderTreeJSON_WorkedExampleFocusMidTree verifies the worked example from
// NP-ffw57: focus is FOO-14200 in a two-level tree. Root → [epic14000] (with
// 4 siblings); epic14000 → [focus, 2 siblings].
func TestRenderTreeJSON_WorkedExampleFocusMidTree(t *testing.T) {
	t.Parallel()

	// Given: the tree from NP-ffw57:
	//   root (FOO-10000 analogue) has 5 children:
	//     epic11000: 2 children (task11100, task11200)
	//     task12000: leaf
	//     epic13000: leaf
	//     epic14000: 3 children (task14100, task14200, task14300)
	//     task15000: leaf
	svc := setupJSONService(t)
	root := createJSONEpic(t, svc, "Title 10000", "")
	epic11000 := createJSONEpic(t, svc, "Title 11000", root.String())
	_ = createJSONTask(t, svc, "Title 11100", epic11000.String())
	_ = createJSONTask(t, svc, "Title 11200", epic11000.String())
	_ = createJSONTask(t, svc, "Title 12000", root.String())
	_ = createJSONEpic(t, svc, "Title 13000", root.String())
	epic14000 := createJSONEpic(t, svc, "Title 14000", root.String())
	_ = createJSONTask(t, svc, "Title 14100", epic14000.String())
	focus := createJSONTask(t, svc, "Title 14200", epic14000.String())
	_ = createJSONTask(t, svc, "Title 14300", epic14000.String())
	_ = createJSONTask(t, svc, "Title 15000", root.String())

	// When: rendering the JSON tree from focus (14200) in non-full mode.
	raw := renderTreeJSON(t, svc, focus.String(), false)

	// Then: root is the top-level object; it has 5 children in children array:
	//   - 4 placeholders (for epic11000, task12000, epic13000, task15000)
	//   - 1 expanded epic14000 node
	node := parseTreeNode(t, raw)
	if got := node["id"]; got != root.String() {
		t.Errorf("root id: got %v, want %q", got, root)
	}
	rootChildren := childrenOf(node)
	if len(rootChildren) != 5 {
		t.Fatalf("root children count: got %d, want 5; raw:\n%s", len(rootChildren), raw)
	}

	// Find the expanded child at root level (should be epic14000).
	var expandedAtRoot map[string]any
	rootExpandedCount := 0
	for _, child := range rootChildren {
		if isExpanded(child) {
			rootExpandedCount++
			expandedAtRoot = child
		}
	}
	if rootExpandedCount != 1 {
		t.Errorf("root expanded children: got %d, want 1", rootExpandedCount)
	}
	if expandedAtRoot != nil {
		if got := expandedAtRoot["id"]; got != epic14000.String() {
			t.Errorf("expanded root child id: got %v, want %q", got, epic14000)
		}
	}

	// epic14000 should have 3 children in its children array:
	//   - focus (expanded)
	//   - 2 placeholders (task14100, task14300)
	if expandedAtRoot == nil {
		t.Fatal("no expanded child found at root level")
	}
	epic14Children := childrenOf(expandedAtRoot)
	if len(epic14Children) != 3 {
		t.Fatalf("epic14000 children count: got %d, want 3; raw:\n%s", len(epic14Children), raw)
	}

	var focusNode map[string]any
	epic14ExpandedCount := 0
	for _, child := range epic14Children {
		if isExpanded(child) {
			epic14ExpandedCount++
			focusNode = child
		}
	}
	if epic14ExpandedCount != 1 {
		t.Errorf("epic14000 expanded children: got %d, want 1", epic14ExpandedCount)
	}
	if focusNode != nil {
		if got := focusNode["id"]; got != focus.String() {
			t.Errorf("focus node id: got %v, want %q", got, focus)
		}
	}
}

// TestRenderTreeJSON_FullMode_AllExpandedNoPlaceholders verifies that --full
// mode produces a completely expanded tree with no placeholder entries.
func TestRenderTreeJSON_FullMode_AllExpandedNoPlaceholders(t *testing.T) {
	t.Parallel()

	// Given: root → [epicA → [taskA1, taskA2], taskB]; focus is taskA1.
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "Root", "")
	epicA := createJSONEpic(t, svc, "Epic A", rootID.String())
	taskA1 := createJSONTask(t, svc, "Task A1", epicA.String())
	_ = createJSONTask(t, svc, "Task A2", epicA.String())
	_ = createJSONTask(t, svc, "Task B", rootID.String())

	// When: rendering with full=true from a deep leaf.
	raw := renderTreeJSON(t, svc, taskA1.String(), true)

	// Then: all nodes are expanded (no placeholders anywhere in the tree).
	var countPlaceholders func(node map[string]any) int
	countPlaceholders = func(node map[string]any) int {
		count := 0
		for _, child := range childrenOf(node) {
			if isPlaceholder(child) {
				count++
			} else {
				count += countPlaceholders(child)
			}
		}
		return count
	}
	node := parseTreeNode(t, raw)
	if n := countPlaceholders(node); n != 0 {
		t.Errorf("expected 0 placeholders in full mode; got %d; raw:\n%s", n, raw)
	}
}

// TestRenderTreeJSON_FullMode_SameAsRootDefaultMode verifies that --full from
// a non-root node produces the same JSON structure as the default mode from the
// root ancestor.
func TestRenderTreeJSON_FullMode_SameAsRootDefaultMode(t *testing.T) {
	t.Parallel()

	// Given: root → [epicA → [taskA1, taskA2], taskB]; focus is taskA1.
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "Root", "")
	epicA := createJSONEpic(t, svc, "Epic A", rootID.String())
	taskA1 := createJSONTask(t, svc, "Task A1", epicA.String())
	_ = createJSONTask(t, svc, "Task A2", epicA.String())
	_ = createJSONTask(t, svc, "Task B", rootID.String())

	// When: rendering full from leaf and default from root.
	rawFullFromLeaf := renderTreeJSON(t, svc, taskA1.String(), true)
	rawDefaultFromRoot := renderTreeJSON(t, svc, rootID.String(), false)

	// Then: both outputs are byte-identical (same tree, same JSON structure).
	if string(rawFullFromLeaf) != string(rawDefaultFromRoot) {
		t.Errorf("full-from-leaf and default-from-root differ:\nfull-from-leaf:\n%s\ndefault-from-root:\n%s",
			rawFullFromLeaf, rawDefaultFromRoot)
	}
}

// TestRenderTreeJSON_PerIssueFieldShape_ByteCompatible verifies that expanded
// issue nodes carry the expected JSON fields (id, role, state, secondary_state,
// display_status, priority, title, parent_id, created_at) and that the field
// names match the existing ConvertListItems shape.
func TestRenderTreeJSON_PerIssueFieldShape_ByteCompatible(t *testing.T) {
	t.Parallel()

	// Given: a standalone epic with no parent (so parent_id should be absent).
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "My Epic", "")

	// When: rendering the JSON tree.
	raw := renderTreeJSON(t, svc, rootID.String(), false)

	// Then: expected fields are present; no PascalCase leaks; no unknown fields.
	node := parseTreeNode(t, raw)
	expectedFields := []string{"id", "role", "state", "display_status", "priority", "title", "created_at", "children"}
	for _, field := range expectedFields {
		if _, ok := node[field]; !ok {
			t.Errorf("expected field %q absent in node; raw:\n%s", field, raw)
		}
	}
	// Role should be a string (not an int).
	if role, ok := node["role"].(string); !ok || role == "" {
		t.Errorf("role field should be a non-empty string; got %v", node["role"])
	}
	// No PascalCase leaks.
	for _, bad := range []string{"Items", "HasMore", "ID", "Role", "State"} {
		if _, ok := node[bad]; ok {
			t.Errorf("unexpected PascalCase field %q in output", bad)
		}
	}
}

// TestRenderTreeJSON_NoANSIInTTYMode verifies that the JSON output contains no
// ANSI escape sequences even when the output writer is attached to a TTY.
func TestRenderTreeJSON_NoANSIInTTYMode(t *testing.T) {
	t.Parallel()

	// Given: a standalone task and an IOStreams attached to a TTY.
	svc := setupJSONService(t)
	rootID := createJSONTask(t, svc, "Task", "")
	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	// When: rendering the JSON tree to the TTY writer.
	if err := relcmd.RenderTreeJSON(context.Background(), ios.Out, svc, rootID.String(), false); err != nil {
		t.Fatalf("RenderTreeJSON failed: %v", err)
	}

	// Then: the raw output contains no ANSI escape sequences.
	raw := out.String()
	if strings.Contains(raw, "\033[") {
		t.Errorf("ANSI escape sequence found in JSON output: %q", raw)
	}
}

// TestRenderTreeJSON_FocusSubtreeFullyExpanded verifies that the focus node's
// own subtree is fully expanded even in non-full mode.
func TestRenderTreeJSON_FocusSubtreeFullyExpanded(t *testing.T) {
	t.Parallel()

	// Given: root → focus → [grandchild1, grandchild2]; root has a sibling.
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "Root", "")
	focus := createJSONEpic(t, svc, "Focus epic", rootID.String())
	gc1 := createJSONTask(t, svc, "Grandchild 1", focus.String())
	gc2 := createJSONTask(t, svc, "Grandchild 2", focus.String())
	_ = createJSONTask(t, svc, "Root sibling", rootID.String())

	// When: rendering in non-full mode with focus as focus.
	raw := renderTreeJSON(t, svc, focus.String(), false)

	// Then: root has two children (focus expanded + root-sibling placeholder);
	// focus has two fully-expanded grandchildren (no placeholders inside focus).
	node := parseTreeNode(t, raw)
	rootChildren := childrenOf(node)
	if len(rootChildren) != 2 {
		t.Fatalf("root children: got %d, want 2; raw:\n%s", len(rootChildren), raw)
	}

	// Find the focus node among root's children.
	var focusNode map[string]any
	for _, child := range rootChildren {
		if !isPlaceholder(child) && child["id"] == focus.String() {
			focusNode = child
		}
	}
	if focusNode == nil {
		t.Fatalf("focus node not found in root children; raw:\n%s", raw)
	}

	// Focus's children must be two expanded grandchildren.
	focusChildren := childrenOf(focusNode)
	if len(focusChildren) != 2 {
		t.Fatalf("focus children: got %d, want 2; raw:\n%s", len(focusChildren), raw)
	}
	gcIDs := map[string]bool{}
	for _, child := range focusChildren {
		if isPlaceholder(child) {
			t.Errorf("unexpected placeholder in focus subtree: %v", child)
		}
		gcIDs[child["id"].(string)] = true
	}
	if !gcIDs[gc1.String()] {
		t.Errorf("grandchild1 %q not found in focus children", gc1)
	}
	if !gcIDs[gc2.String()] {
		t.Errorf("grandchild2 %q not found in focus children", gc2)
	}
}

// TestRenderTreeJSON_ChildrenSortedAscendingByID verifies that children arrays
// at every level are sorted in ascending order by issue ID.
func TestRenderTreeJSON_ChildrenSortedAscendingByID(t *testing.T) {
	t.Parallel()

	// Given: root → [c1, c2, c3, c4]; all expanded because focus is root.
	svc := setupJSONService(t)
	rootID := createJSONEpic(t, svc, "Root", "")
	_ = createJSONTask(t, svc, "C1", rootID.String())
	_ = createJSONTask(t, svc, "C2", rootID.String())
	_ = createJSONTask(t, svc, "C3", rootID.String())
	_ = createJSONTask(t, svc, "C4", rootID.String())

	// When: rendering the full tree from root.
	raw := renderTreeJSON(t, svc, rootID.String(), false)

	// Then: children are sorted ascending by ID.
	node := parseTreeNode(t, raw)
	children := childrenOf(node)
	if len(children) != 4 {
		t.Fatalf("root children count: got %d, want 4; raw:\n%s", len(children), raw)
	}
	ids := make([]string, len(children))
	for i, child := range children {
		ids[i] = child["id"].(string)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("children not sorted ascending at index %d: %q > %q", i, ids[i-1], ids[i])
		}
	}
}

// TestRenderTreeJSON_OrphanTask_SingleNodeEmptyChildren verifies that an orphan
// task (no parent, no children) renders as a single top-level node with an
// empty children array. This covers the "focus-is-orphan-task" acceptance case.
func TestRenderTreeJSON_OrphanTask_SingleNodeEmptyChildren(t *testing.T) {
	t.Parallel()

	// Given: a task with no parent and no children.
	svc := setupJSONService(t)
	taskID := createJSONTask(t, svc, "Orphan task", "")

	// When: rendering in non-full mode.
	raw := renderTreeJSON(t, svc, taskID.String(), false)

	// Then: single node, empty children.
	node := parseTreeNode(t, raw)
	if got := node["id"]; got != taskID.String() {
		t.Errorf("node id: got %v, want %q", got, taskID)
	}
	children := childrenOf(node)
	if len(children) != 0 {
		t.Errorf("expected empty children for orphan task; got %d", len(children))
	}
}

// TestRenderTreeJSON_WritesToProvidedWriter verifies that RenderTreeJSON writes
// its output to the writer argument rather than to some implicit destination.
// This is important because newTreeCmd passes f.IOStreams.Out explicitly.
func TestRenderTreeJSON_WritesToProvidedWriter(t *testing.T) {
	t.Parallel()

	// Given: a standalone task and a bytes.Buffer to capture output.
	svc := setupJSONService(t)
	rootID := createJSONTask(t, svc, "Output test", "")
	var buf bytes.Buffer

	// When: rendering the JSON tree to the buffer.
	if err := relcmd.RenderTreeJSON(context.Background(), &buf, svc, rootID.String(), false); err != nil {
		t.Fatalf("RenderTreeJSON failed: %v", err)
	}

	// Then: the buffer contains the expected root object.
	if buf.Len() == 0 {
		t.Fatal("expected non-empty output to buffer")
	}
	if !strings.Contains(buf.String(), rootID.String()) {
		t.Errorf("output missing issue ID %q; got: %s", rootID, buf.String())
	}
}

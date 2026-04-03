package graphcmd_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/graphcmd"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// mustParseGraphID parses an issue ID string for use in graph tests, failing
// the test immediately if the string is not a valid ID.
func mustParseGraphID(t *testing.T, s string) domain.ID {
	t.Helper()
	id, err := domain.ParseID(s)
	if err != nil {
		t.Fatalf("invalid test ID %q: %v", s, err)
	}
	return id
}

func TestRenderGraphDOT_EmptyGraph_ProducesValidDOT(t *testing.T) {
	t.Parallel()

	// When
	result := graphcmd.RenderGraphDOT(nil, nil)

	// Then
	if !strings.Contains(result, "digraph issues") {
		t.Error("expected digraph header")
	}
	if !strings.HasSuffix(result, "}\n") {
		t.Error("expected closing brace")
	}
}

func TestRenderGraphDOT_SingleNode_RendersWithColorAndLabel(t *testing.T) {
	t.Parallel()

	// Given
	nodes := []graphcmd.GraphNode{
		{ID: mustParseGraphID(t, "NP-a3bxr"), Role: domain.RoleTask, State: domain.StateOpen, Title: "Fix login bug"},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, nil)

	// Then
	if !strings.Contains(result, `"NP-a3bxr"`) {
		t.Error("expected node ID in output")
	}
	if !strings.Contains(result, "Fix login bug") {
		t.Error("expected title in label")
	}
	if !strings.Contains(result, `fillcolor="white"`) {
		t.Errorf("expected white fill for open state, got:\n%s", result)
	}
}

func TestRenderGraphDOT_NodeLabel_UsesBackslashNForLineBreaks(t *testing.T) {
	t.Parallel()

	// Given
	nodes := []graphcmd.GraphNode{
		{ID: mustParseGraphID(t, "NP-a3bxr"), Role: domain.RoleTask, State: domain.StateOpen, Title: "Fix login bug"},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, nil)

	// Then: DOT labels should use \n (backslash-n) for line breaks,
	// not \\n (double-escaped).
	if strings.Contains(result, `\\n`) {
		t.Errorf("label contains literal \\\\n instead of \\n:\n%s", result)
	}
	// The label should contain \n for line breaks in DOT format.
	if !strings.Contains(result, `\n`) {
		t.Errorf("label should contain \\n for DOT line breaks:\n%s", result)
	}
}

func TestRenderGraphDOT_StateColors_MatchExpected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state domain.State
		color string
	}{
		{"open", domain.StateOpen, "white"},
		{"claimed", domain.StateClaimed, "yellow"},
		{"closed", domain.StateClosed, "gray"},
		{"deferred", domain.StateDeferred, "lightyellow"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nodes := []graphcmd.GraphNode{
				{ID: mustParseGraphID(t, "NP-a3bxr"), Role: domain.RoleTask, State: tc.state, Title: "Test"},
			}

			// When
			result := graphcmd.RenderGraphDOT(nodes, nil)

			// Then
			if !strings.Contains(result, `fillcolor="`+tc.color+`"`) {
				t.Errorf("expected fillcolor=%q for state %s, got:\n%s", tc.color, tc.state, result)
			}
		})
	}
}

func TestRenderGraphDOT_ParentChild_CreatesClusterAndEdge(t *testing.T) {
	t.Parallel()

	// Given
	epicID := mustParseGraphID(t, "NP-ep1c0")
	taskID := mustParseGraphID(t, "NP-ta5k0")

	nodes := []graphcmd.GraphNode{
		{ID: epicID, Role: domain.RoleEpic, State: domain.StateOpen, Title: "Auth epic"},
		{ID: taskID, Role: domain.RoleTask, State: domain.StateOpen, Title: "Login fix", ParentID: epicID},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, nil)

	// Then: subgraph cluster exists.
	if !strings.Contains(result, "subgraph cluster_ep1c0") {
		t.Error("expected subgraph cluster for epic")
	}
	// Parent-child edge.
	if !strings.Contains(result, `"NP-ep1c0" -> "NP-ta5k0"`) {
		t.Error("expected parent-child edge")
	}
	if !strings.Contains(result, "style=solid") {
		t.Error("expected solid style for parent-child edge")
	}
}

func TestRenderGraphDOT_BlockedByEdge_DashedRed(t *testing.T) {
	t.Parallel()

	// Given
	id1 := mustParseGraphID(t, "NP-a3bxr")
	id2 := mustParseGraphID(t, "NP-b4cys")

	nodes := []graphcmd.GraphNode{
		{ID: id1, Role: domain.RoleTask, State: domain.StateOpen, Title: "Task A"},
		{ID: id2, Role: domain.RoleTask, State: domain.StateOpen, Title: "Task B"},
	}
	edges := []graphcmd.GraphEdge{
		{SourceID: id1, TargetID: id2, Type: domain.RelBlockedBy},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, edges)

	// Then
	if !strings.Contains(result, `"NP-a3bxr" -> "NP-b4cys"`) {
		t.Error("expected blocked_by edge")
	}
	if !strings.Contains(result, "style=dashed") {
		t.Error("expected dashed style")
	}
	if !strings.Contains(result, "color=red") {
		t.Error("expected red color")
	}
}

func TestRenderGraphDOT_CitesEdge_DottedGray(t *testing.T) {
	t.Parallel()

	// Given
	id1 := mustParseGraphID(t, "NP-a3bxr")
	id2 := mustParseGraphID(t, "NP-b4cys")

	nodes := []graphcmd.GraphNode{
		{ID: id1, Role: domain.RoleTask, State: domain.StateOpen, Title: "Task A"},
		{ID: id2, Role: domain.RoleTask, State: domain.StateOpen, Title: "Task B"},
	}
	edges := []graphcmd.GraphEdge{
		{SourceID: id1, TargetID: id2, Type: domain.RelCites},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, edges)

	// Then
	if !strings.Contains(result, `"NP-a3bxr" -> "NP-b4cys"`) {
		t.Error("expected cites edge")
	}
	if !strings.Contains(result, "style=dotted") {
		t.Error("expected dotted style")
	}
	if !strings.Contains(result, "color=gray") {
		t.Error("expected gray color")
	}
}

func TestRenderGraphDOT_OrphanNode_RenderedOutsideCluster(t *testing.T) {
	t.Parallel()

	// Given — a node whose ParentID references an ID not in the graph
	orphanParentID := mustParseGraphID(t, "NP-m1ss0")
	orphanID := mustParseGraphID(t, "NP-0rph0")

	nodes := []graphcmd.GraphNode{
		{ID: orphanID, Role: domain.RoleTask, State: domain.StateOpen, Title: "Orphan task", ParentID: orphanParentID},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, nil)

	// Then — orphan should be rendered (not lost)
	if !strings.Contains(result, `"NP-0rph0"`) {
		t.Errorf("expected orphan node to be rendered, got:\n%s", result)
	}
	// Should not be in a cluster since parent is not in the graph
	if strings.Contains(result, "subgraph cluster_") {
		t.Error("expected no cluster for orphan node")
	}
}

func TestRenderGraphDOT_EdgeWithMissingEndpoint_StillRendered(t *testing.T) {
	t.Parallel()

	// Given — an edge references a node ID not in the nodes list
	existingID := mustParseGraphID(t, "NP-a3bxr")
	missingID := mustParseGraphID(t, "NP-m1ss0")

	nodes := []graphcmd.GraphNode{
		{ID: existingID, Role: domain.RoleTask, State: domain.StateOpen, Title: "Existing"},
	}
	edges := []graphcmd.GraphEdge{
		{SourceID: existingID, TargetID: missingID, Type: domain.RelBlockedBy},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, edges)

	// Then — edge should still appear in output (DOT allows dangling references)
	if !strings.Contains(result, `"NP-a3bxr" -> "NP-m1ss0"`) {
		t.Errorf("expected edge with missing endpoint to render, got:\n%s", result)
	}
}

func TestRenderGraphDOT_SpecialCharactersInTitle_Escaped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		title string
	}{
		{"double quotes", `Fix "auth" bug`},
		{"backslash", `Path C:\Users\test`},
		{"angle brackets", `Compare <old> vs <new>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given
			nodes := []graphcmd.GraphNode{
				{ID: mustParseGraphID(t, "NP-a3bxr"), Role: domain.RoleTask, State: domain.StateOpen, Title: tc.title},
			}

			// When
			result := graphcmd.RenderGraphDOT(nodes, nil)

			// Then — output should be valid DOT (no unescaped special chars)
			// The %q formatting escapes quotes and backslashes automatically.
			if !strings.Contains(result, `"NP-a3bxr"`) {
				t.Errorf("expected node to render for title %q, got:\n%s", tc.title, result)
			}
			// Raw unescaped double quotes inside a DOT string would break parsing.
			// Verify title content appears in escaped form (not raw).
			if tc.title == `Fix "auth" bug` && strings.Contains(result, `Fix "auth" bug`) {
				t.Error("expected quotes in title to be escaped in DOT output")
			}
		})
	}
}

func TestRenderGraphDOT_ShortTitle_RenderedFully(t *testing.T) {
	t.Parallel()

	// Given
	nodes := []graphcmd.GraphNode{
		{ID: mustParseGraphID(t, "NP-a3bxr"), Role: domain.RoleTask, State: domain.StateOpen, Title: "X"},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, nil)

	// Then — short title should not be truncated
	if strings.Contains(result, "...") {
		t.Error("expected short title not to be truncated")
	}
	if !strings.Contains(result, "X") {
		t.Error("expected short title to appear in output")
	}
}

func TestRenderGraphDOT_LongTitle_Truncated(t *testing.T) {
	t.Parallel()

	// Given
	longTitle := "This is a very long title that should be truncated for readability in the graph"
	nodes := []graphcmd.GraphNode{
		{ID: mustParseGraphID(t, "NP-a3bxr"), Role: domain.RoleTask, State: domain.StateOpen, Title: longTitle},
	}

	// When
	result := graphcmd.RenderGraphDOT(nodes, nil)

	// Then: full title should NOT appear; truncated version should.
	if strings.Contains(result, longTitle) {
		t.Error("expected long title to be truncated")
	}
	if !strings.Contains(result, "...") {
		t.Error("expected ellipsis in truncated title")
	}
}

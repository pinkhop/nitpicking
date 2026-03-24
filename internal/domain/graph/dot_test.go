package graph_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/graph"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func mustID(t *testing.T, s string) issue.ID {
	t.Helper()
	id, err := issue.ParseID(s)
	if err != nil {
		t.Fatalf("invalid test ID %q: %v", s, err)
	}
	return id
}

func TestRenderDOT_EmptyGraph_ProducesValidDOT(t *testing.T) {
	t.Parallel()

	// When
	result := graph.RenderDOT(nil, nil)

	// Then
	if !strings.Contains(result, "digraph issues") {
		t.Error("expected digraph header")
	}
	if !strings.HasSuffix(result, "}\n") {
		t.Error("expected closing brace")
	}
}

func TestRenderDOT_SingleNode_RendersWithColorAndLabel(t *testing.T) {
	t.Parallel()

	// Given
	nodes := []graph.Node{
		{ID: mustID(t, "NP-a3bxr"), Role: issue.RoleTask, State: issue.StateOpen, Title: "Fix login bug"},
	}

	// When
	result := graph.RenderDOT(nodes, nil)

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

func TestRenderDOT_StateColors_MatchExpected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state issue.State
		color string
	}{
		{"open", issue.StateOpen, "white"},
		{"claimed", issue.StateClaimed, "yellow"},
		{"closed", issue.StateClosed, "gray"},
		{"deferred", issue.StateDeferred, "lightyellow"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nodes := []graph.Node{
				{ID: mustID(t, "NP-a3bxr"), Role: issue.RoleTask, State: tc.state, Title: "Test"},
			}

			// When
			result := graph.RenderDOT(nodes, nil)

			// Then
			if !strings.Contains(result, `fillcolor="`+tc.color+`"`) {
				t.Errorf("expected fillcolor=%q for state %s, got:\n%s", tc.color, tc.state, result)
			}
		})
	}
}

func TestRenderDOT_ParentChild_CreatesClusterAndEdge(t *testing.T) {
	t.Parallel()

	// Given
	epicID := mustID(t, "NP-ep1c0")
	taskID := mustID(t, "NP-ta5k0")

	nodes := []graph.Node{
		{ID: epicID, Role: issue.RoleEpic, State: issue.StateOpen, Title: "Auth epic"},
		{ID: taskID, Role: issue.RoleTask, State: issue.StateOpen, Title: "Login fix", ParentID: epicID},
	}

	// When
	result := graph.RenderDOT(nodes, nil)

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

func TestRenderDOT_BlockedByEdge_DashedRed(t *testing.T) {
	t.Parallel()

	// Given
	id1 := mustID(t, "NP-a3bxr")
	id2 := mustID(t, "NP-b4cys")

	nodes := []graph.Node{
		{ID: id1, Role: issue.RoleTask, State: issue.StateOpen, Title: "Task A"},
		{ID: id2, Role: issue.RoleTask, State: issue.StateOpen, Title: "Task B"},
	}
	edges := []graph.Edge{
		{SourceID: id1, TargetID: id2, Type: issue.RelBlockedBy},
	}

	// When
	result := graph.RenderDOT(nodes, edges)

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

func TestRenderDOT_CitesEdge_DottedGray(t *testing.T) {
	t.Parallel()

	// Given
	id1 := mustID(t, "NP-a3bxr")
	id2 := mustID(t, "NP-b4cys")

	nodes := []graph.Node{
		{ID: id1, Role: issue.RoleTask, State: issue.StateOpen, Title: "Task A"},
		{ID: id2, Role: issue.RoleTask, State: issue.StateOpen, Title: "Task B"},
	}
	edges := []graph.Edge{
		{SourceID: id1, TargetID: id2, Type: issue.RelCites},
	}

	// When
	result := graph.RenderDOT(nodes, edges)

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

func TestRenderDOT_LongTitle_Truncated(t *testing.T) {
	t.Parallel()

	// Given
	longTitle := "This is a very long title that should be truncated for readability in the graph"
	nodes := []graph.Node{
		{ID: mustID(t, "NP-a3bxr"), Role: issue.RoleTask, State: issue.StateOpen, Title: longTitle},
	}

	// When
	result := graph.RenderDOT(nodes, nil)

	// Then: full title should NOT appear; truncated version should.
	if strings.Contains(result, longTitle) {
		t.Error("expected long title to be truncated")
	}
	if !strings.Contains(result, "...") {
		t.Error("expected ellipsis in truncated title")
	}
}

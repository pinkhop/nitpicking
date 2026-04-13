package graphcmd_test

import (
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd/graphcmd"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// graphJSONIssue mirrors the expected JSON output structure for assertions.
type graphJSONIssue struct {
	ID            string                 `json:"id"`
	Role          string                 `json:"role"`
	State         string                 `json:"state"`
	Title         string                 `json:"title"`
	Relationships graphJSONRelationships `json:"relationships"`
	Children      []graphJSONIssue       `json:"children"`
}

// graphJSONRelationships mirrors the relationship JSON structure for test
// assertions.
type graphJSONRelationships struct {
	Blocks    []string `json:"blocks"`
	BlockedBy []string `json:"blocked_by"`
	Refs      []string `json:"refs"`
}

func TestRenderGraphJSON_EmptyGraph_ReturnsEmptyArray(t *testing.T) {
	t.Parallel()

	// When
	result := graphcmd.RenderGraphJSON(nil, nil)

	// Then
	var issues []graphJSONIssue
	if err := json.Unmarshal([]byte(result), &issues); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, result)
	}
	if len(issues) != 0 {
		t.Errorf("expected empty array, got %d items", len(issues))
	}
}

func TestRenderGraphJSON_SingleRootTask_HasAllFields(t *testing.T) {
	t.Parallel()

	// Given
	id := mustParseGraphID(t, "NP-abc12")
	nodes := []graphcmd.GraphNode{
		{ID: id, Role: domain.RoleTask, State: domain.StateOpen, Title: "My task"},
	}

	// When
	result := graphcmd.RenderGraphJSON(nodes, nil)

	// Then
	var issues []graphJSONIssue
	if err := json.Unmarshal([]byte(result), &issues); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, result)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 root issue, got %d", len(issues))
	}

	got := issues[0]
	if got.ID != "NP-abc12" {
		t.Errorf("id: got %q, want %q", got.ID, "NP-abc12")
	}
	if got.Role != "task" {
		t.Errorf("role: got %q, want %q", got.Role, "task")
	}
	if got.State != "open" {
		t.Errorf("state: got %q, want %q", got.State, "open")
	}
	if got.Title != "My task" {
		t.Errorf("title: got %q, want %q", got.Title, "My task")
	}
	if got.Relationships.Blocks == nil || got.Relationships.BlockedBy == nil || got.Relationships.Refs == nil {
		t.Error("relationship arrays should be non-nil (empty, not null)")
	}
	if got.Children == nil {
		t.Error("children should be non-nil (empty, not null)")
	}
}

func TestRenderGraphJSON_ParentChild_ChildrenNested(t *testing.T) {
	t.Parallel()

	// Given — an epic with two child tasks.
	epicID := mustParseGraphID(t, "NP-epic1")
	child1 := mustParseGraphID(t, "NP-tsk01")
	child2 := mustParseGraphID(t, "NP-tsk02")

	nodes := []graphcmd.GraphNode{
		{ID: epicID, Role: domain.RoleEpic, State: domain.StateOpen, Title: "My Epic"},
		{ID: child1, Role: domain.RoleTask, State: domain.StateOpen, Title: "Task 1", ParentID: epicID},
		{ID: child2, Role: domain.RoleTask, State: domain.StateOpen, Title: "Task 2", ParentID: epicID},
	}

	// When
	result := graphcmd.RenderGraphJSON(nodes, nil)

	// Then — only the epic is at root level; children are nested.
	var issues []graphJSONIssue
	if err := json.Unmarshal([]byte(result), &issues); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, result)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 root issue, got %d", len(issues))
	}
	if len(issues[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(issues[0].Children))
	}
}

func TestRenderGraphJSON_Relationships_BlockedByAndRefs(t *testing.T) {
	t.Parallel()

	// Given — A is blocked_by B, and A cites C.
	idA := mustParseGraphID(t, "NP-aaaaa")
	idB := mustParseGraphID(t, "NP-bbbbb")
	idC := mustParseGraphID(t, "NP-ccccc")

	nodes := []graphcmd.GraphNode{
		{ID: idA, Role: domain.RoleTask, State: domain.StateOpen, Title: "A"},
		{ID: idB, Role: domain.RoleTask, State: domain.StateOpen, Title: "B"},
		{ID: idC, Role: domain.RoleTask, State: domain.StateOpen, Title: "C"},
	}

	edges := []graphcmd.GraphEdge{
		{SourceID: idA, TargetID: idB, Type: domain.RelBlockedBy},
		{SourceID: idA, TargetID: idC, Type: domain.RelCites},
	}

	// When
	result := graphcmd.RenderGraphJSON(nodes, edges)

	// Then — A has blocked_by=[B], refs=[C]; B has blocks=[A]; C has refs=[A].
	var issues []graphJSONIssue
	if err := json.Unmarshal([]byte(result), &issues); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, result)
	}

	issueMap := make(map[string]graphJSONIssue, len(issues))
	for _, iss := range issues {
		issueMap[iss.ID] = iss
	}

	a := issueMap["NP-aaaaa"]
	if len(a.Relationships.BlockedBy) != 1 || a.Relationships.BlockedBy[0] != "NP-bbbbb" {
		t.Errorf("A.blocked_by: got %v, want [NP-bbbbb]", a.Relationships.BlockedBy)
	}
	if len(a.Relationships.Refs) != 1 || a.Relationships.Refs[0] != "NP-ccccc" {
		t.Errorf("A.refs: got %v, want [NP-ccccc]", a.Relationships.Refs)
	}

	b := issueMap["NP-bbbbb"]
	if len(b.Relationships.Blocks) != 1 || b.Relationships.Blocks[0] != "NP-aaaaa" {
		t.Errorf("B.blocks: got %v, want [NP-aaaaa]", b.Relationships.Blocks)
	}

	c := issueMap["NP-ccccc"]
	if len(c.Relationships.Refs) != 1 || c.Relationships.Refs[0] != "NP-aaaaa" {
		t.Errorf("C.refs: got %v, want [NP-aaaaa]", c.Relationships.Refs)
	}
}

func TestRenderGraphJSON_DeepNesting_EpicContainingSubEpic(t *testing.T) {
	t.Parallel()

	// Given — epic → sub-epic → task.
	epicID := mustParseGraphID(t, "NP-epic1")
	subID := mustParseGraphID(t, "NP-epic2")
	taskID := mustParseGraphID(t, "NP-tsk01")

	nodes := []graphcmd.GraphNode{
		{ID: epicID, Role: domain.RoleEpic, State: domain.StateOpen, Title: "Root epic"},
		{ID: subID, Role: domain.RoleEpic, State: domain.StateOpen, Title: "Sub epic", ParentID: epicID},
		{ID: taskID, Role: domain.RoleTask, State: domain.StateOpen, Title: "Leaf task", ParentID: subID},
	}

	// When
	result := graphcmd.RenderGraphJSON(nodes, nil)

	// Then — root[0].children[0].children[0] is the leaf task.
	var issues []graphJSONIssue
	if err := json.Unmarshal([]byte(result), &issues); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, result)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 root, got %d", len(issues))
	}
	if len(issues[0].Children) != 1 {
		t.Fatalf("expected 1 child of root, got %d", len(issues[0].Children))
	}
	if len(issues[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 child of sub-epic, got %d", len(issues[0].Children[0].Children))
	}
	if issues[0].Children[0].Children[0].ID != "NP-tsk01" {
		t.Errorf("leaf task id: got %q, want %q", issues[0].Children[0].Children[0].ID, "NP-tsk01")
	}
}

package graphcmd

import (
	"fmt"
	"strings"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// GraphNode represents an issue in the graph with the minimum attributes needed
// for rendering.
type GraphNode struct {
	ID       domain.ID
	Role     domain.Role
	State    domain.State
	Title    string
	ParentID domain.ID
}

// GraphEdge represents a relationship between two issues in the graph.
type GraphEdge struct {
	SourceID domain.ID
	TargetID domain.ID
	Type     domain.RelationType
}

// graphStateColors maps issue states to Graphviz fill colors.
var graphStateColors = map[domain.State]string{
	domain.StateOpen:     "white",
	domain.StateClosed:   "gray",
	domain.StateDeferred: "lightyellow",
}

// RenderGraphDOT generates a Graphviz DOT string from the given nodes and
// edges. Nodes are color-coded by state. Parent–child hierarchy is represented
// with solid edges, blocked_by with dashed red edges, and cites with dotted
// gray edges. Children are clustered under their parent epic using subgraphs.
func RenderGraphDOT(nodes []GraphNode, edges []GraphEdge) string {
	var b strings.Builder

	b.WriteString("digraph issues {\n")
	b.WriteString("  rankdir=TB;\n")
	b.WriteString("  node [shape=box, style=filled, fontname=\"Helvetica\"];\n\n")

	// Index nodes by ID for parent lookups.
	nodeByID := make(map[string]GraphNode, len(nodes))
	childrenOf := make(map[string][]GraphNode)
	var roots []GraphNode

	for _, n := range nodes {
		nodeByID[n.ID.String()] = n
		if n.ParentID.IsZero() {
			roots = append(roots, n)
		} else {
			childrenOf[n.ParentID.String()] = append(childrenOf[n.ParentID.String()], n)
		}
	}

	// Render clustered subgraphs for epics with children, and standalone
	// nodes for roots without children.
	for _, root := range roots {
		children := childrenOf[root.ID.String()]
		if len(children) > 0 {
			writeGraphCluster(&b, root, children, childrenOf)
		} else {
			writeGraphNode(&b, root, "  ")
		}
	}

	// Render any orphaned nodes (parent not in the graph).
	rendered := make(map[string]bool)
	var markRendered func(n GraphNode)
	markRendered = func(n GraphNode) {
		rendered[n.ID.String()] = true
		for _, child := range childrenOf[n.ID.String()] {
			markRendered(child)
		}
	}
	for _, root := range roots {
		markRendered(root)
	}
	for _, n := range nodes {
		if !rendered[n.ID.String()] {
			writeGraphNode(&b, n, "  ")
		}
	}

	b.WriteString("\n")

	// Render parent–child edges.
	for _, n := range nodes {
		if !n.ParentID.IsZero() {
			fmt.Fprintf(&b, "  %q -> %q [style=solid, color=black];\n",
				n.ParentID.String(), n.ID.String())
		}
	}

	// Render relationship edges.
	for _, e := range edges {
		switch e.Type {
		case domain.RelBlockedBy:
			fmt.Fprintf(&b, "  %q -> %q [style=dashed, color=red, label=\"blocked_by\"];\n",
				e.SourceID.String(), e.TargetID.String())
		case domain.RelCites:
			fmt.Fprintf(&b, "  %q -> %q [style=dotted, color=gray, label=\"cites\"];\n",
				e.SourceID.String(), e.TargetID.String())
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// writeGraphCluster writes a subgraph cluster for an epic and its children.
func writeGraphCluster(b *strings.Builder, epic GraphNode, children []GraphNode, childrenOf map[string][]GraphNode) {
	fmt.Fprintf(b, "  subgraph cluster_%s {\n", epic.ID.Random())
	fmt.Fprintf(b, "    label=%q;\n", epic.ID.String()+" — "+epic.Title)
	b.WriteString("    style=rounded;\n")
	b.WriteString("    color=blue;\n")

	writeGraphNode(b, epic, "    ")
	for _, child := range children {
		grandchildren := childrenOf[child.ID.String()]
		if len(grandchildren) > 0 {
			writeGraphCluster(b, child, grandchildren, childrenOf)
		} else {
			writeGraphNode(b, child, "    ")
		}
	}

	b.WriteString("  }\n")
}

// writeGraphNode writes a single DOT node definition with color and label.
func writeGraphNode(b *strings.Builder, n GraphNode, indent string) {
	color := graphStateColors[n.State]
	if color == "" {
		color = "white"
	}

	// Truncate long titles for readability.
	title := n.Title
	if len(title) > 40 {
		title = title[:37] + "..."
	}

	label := fmt.Sprintf("%s\n[%s] %s\n%s",
		n.ID.String(), n.Role.String(), n.State.String(), title)

	fmt.Fprintf(b, "%s%q [label=%q, fillcolor=%q];\n",
		indent, n.ID.String(), label, color)
}

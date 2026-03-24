// Package graph renders issue hierarchies and relationships as Graphviz DOT
// format text. The renderer is a pure function with no external dependencies —
// it accepts structured input and returns a DOT string.
package graph

import (
	"fmt"
	"strings"

	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// Node represents an issue in the graph with the minimum attributes needed
// for rendering.
type Node struct {
	ID       issue.ID
	Role     issue.Role
	State    issue.State
	Title    string
	ParentID issue.ID
}

// Edge represents a relationship between two issues in the graph.
type Edge struct {
	SourceID issue.ID
	TargetID issue.ID
	Type     issue.RelationType
}

// stateColors maps issue states to Graphviz fill colors.
var stateColors = map[issue.State]string{
	issue.StateOpen:     "white",
	issue.StateClaimed:  "yellow",
	issue.StateClosed:   "gray",
	issue.StateDeferred: "lightyellow",
	issue.StateWaiting:  "orange",
}

// RenderDOT generates a Graphviz DOT string from the given nodes and edges.
// Nodes are color-coded by state. Parent–child hierarchy is represented with
// solid edges, blocked_by with dashed red edges, and cites with dotted gray
// edges. Children are clustered under their parent epic using subgraphs.
func RenderDOT(nodes []Node, edges []Edge) string {
	var b strings.Builder

	b.WriteString("digraph issues {\n")
	b.WriteString("  rankdir=TB;\n")
	b.WriteString("  node [shape=box, style=filled, fontname=\"Helvetica\"];\n\n")

	// Index nodes by ID for parent lookups.
	nodeByID := make(map[string]Node, len(nodes))
	childrenOf := make(map[string][]Node)
	var roots []Node

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
			writeCluster(&b, root, children, childrenOf)
		} else {
			writeNode(&b, root, "  ")
		}
	}

	// Render any orphaned nodes (parent not in the graph).
	rendered := make(map[string]bool)
	var markRendered func(n Node)
	markRendered = func(n Node) {
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
			writeNode(&b, n, "  ")
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
		case issue.RelBlockedBy:
			fmt.Fprintf(&b, "  %q -> %q [style=dashed, color=red, label=\"blocked_by\"];\n",
				e.SourceID.String(), e.TargetID.String())
		case issue.RelCites:
			fmt.Fprintf(&b, "  %q -> %q [style=dotted, color=gray, label=\"cites\"];\n",
				e.SourceID.String(), e.TargetID.String())
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// writeCluster writes a subgraph cluster for an epic and its children.
func writeCluster(b *strings.Builder, epic Node, children []Node, childrenOf map[string][]Node) {
	fmt.Fprintf(b, "  subgraph cluster_%s {\n", epic.ID.Random())
	fmt.Fprintf(b, "    label=%q;\n", epic.ID.String()+" — "+epic.Title)
	b.WriteString("    style=rounded;\n")
	b.WriteString("    color=blue;\n")

	writeNode(b, epic, "    ")
	for _, child := range children {
		grandchildren := childrenOf[child.ID.String()]
		if len(grandchildren) > 0 {
			writeCluster(b, child, grandchildren, childrenOf)
		} else {
			writeNode(b, child, "    ")
		}
	}

	b.WriteString("  }\n")
}

// writeNode writes a single DOT node definition with color and label.
func writeNode(b *strings.Builder, n Node, indent string) {
	color := stateColors[n.State]
	if color == "" {
		color = "white"
	}

	// Truncate long titles for readability.
	title := n.Title
	if len(title) > 40 {
		title = title[:37] + "..."
	}

	label := fmt.Sprintf("%s\\n[%s] %s\\n%s",
		n.ID.String(), n.Role.String(), n.State.String(), title)

	fmt.Fprintf(b, "%s%q [label=%q, fillcolor=%q];\n",
		indent, n.ID.String(), label, color)
}

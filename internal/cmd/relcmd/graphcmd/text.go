package graphcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// runText renders the issue graph as an ASCII tree to the output writer.
// Root issues (no parent) appear at the top level; children are indented
// under their parent using tree-drawing characters. Relationships are
// listed beneath each issue node.
func runText(ctx context.Context, input RunInput) error {
	fg, err := fetchAndFilter(ctx, input)
	if err != nil {
		return err
	}

	// Index nodes by ID for child lookups.
	nodeByID := make(map[string]driving.IssueListItemDTO, len(fg.nodes))
	for _, n := range fg.nodes {
		nodeByID[n.ID] = n
	}

	// Group children by parent ID.
	childrenOf := make(map[string][]driving.IssueListItemDTO)
	var roots []driving.IssueListItemDTO
	for _, n := range fg.nodes {
		if n.ParentID == "" || !fg.visibleSet[n.ParentID] {
			roots = append(roots, n)
		} else {
			key := n.ParentID
			childrenOf[key] = append(childrenOf[key], n)
		}
	}

	// Index relationships by source issue ID, filtering to visible edges.
	relsBySource := make(map[string][]driving.RelationshipDTO)
	for _, rel := range fg.relationships {
		if !fg.visibleSet[rel.SourceID] || !fg.visibleSet[rel.TargetID] {
			continue
		}
		relsBySource[rel.SourceID] = append(relsBySource[rel.SourceID], rel)
	}

	w := input.WriteTo
	for i, root := range roots {
		isLast := i == len(roots)-1
		writeNode(w, root, "", isLast, true, childrenOf, relsBySource)
	}

	return nil
}

// writeNode renders a single node with its relationships and children,
// recursively descending the tree. prefix is the indentation string
// inherited from the parent; isLast indicates whether this is the last
// sibling at its level; isRoot indicates top-level nodes (no tree chars).
func writeNode(
	w io.Writer,
	node driving.IssueListItemDTO,
	prefix string,
	isLast bool,
	isRoot bool,
	childrenOf map[string][]driving.IssueListItemDTO,
	relsBySource map[string][]driving.RelationshipDTO,
) {
	// Choose tree-drawing characters.
	var connector, childPrefix string
	if isRoot {
		connector = ""
		childPrefix = ""
	} else if isLast {
		connector = "└── "
		childPrefix = prefix + "    "
	} else {
		connector = "├── "
		childPrefix = prefix + "│   "
	}

	// Render the issue line.
	_, _ = fmt.Fprintf(w, "%s%s%s  %s  %s  %s\n",
		prefix, connector,
		node.ID,
		node.Role,
		node.State,
		node.Title,
	)

	// Render relationships beneath the issue.
	rels := relsBySource[node.ID]
	for _, rel := range rels {
		_, _ = fmt.Fprintf(w, "%s%s: %s\n",
			childPrefix,
			rel.Type,
			rel.TargetID,
		)
	}

	// Render children recursively.
	children := childrenOf[node.ID]
	for i, child := range children {
		childIsLast := i == len(children)-1
		writeNode(w, child, childPrefix, childIsLast, false, childrenOf, relsBySource)
	}
}

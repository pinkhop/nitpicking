package relcmd

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// refsEdge represents a single normalized undirected refs edge. leftID is
// always lexicographically less than or equal to rightID, ensuring each
// undirected pair appears exactly once.
type refsEdge struct {
	leftID     string
	leftTitle  string
	rightID    string
	rightTitle string
}

// refsComponent groups a set of issues that are transitively connected by
// refs edges (a connected component of the refs undirected graph).
type refsComponent struct {
	// edges is the sorted edge list (by leftID then rightID).
	edges []refsEdge
	// issueIDs is the sorted list of all distinct issue IDs in this component.
	issueIDs []string
}

// refsGraph holds the pre-computed connected-component data for the refs
// section of np rel list.
type refsGraph struct {
	// components is sorted by component size (descending), then smallest ID (ascending).
	components []refsComponent
	// totalEdges is the count of unique undirected refs edges across all components.
	totalEdges int
}

// refsUnionFind provides union-find (disjoint set) operations for computing
// connected components in O(α(n)) per operation via path compression and
// union by rank.
type refsUnionFind struct {
	parent map[string]string
	rank   map[string]int
}

// newRefsUnionFind returns an empty union-find structure.
func newRefsUnionFind() *refsUnionFind {
	return &refsUnionFind{
		parent: make(map[string]string),
		rank:   make(map[string]int),
	}
}

// find returns the canonical representative of the set containing x, with
// path compression so future lookups are faster.
func (uf *refsUnionFind) find(x string) string {
	if uf.parent[x] == "" {
		// x is its own representative (first encounter).
		return x
	}
	if uf.parent[x] != x {
		uf.parent[x] = uf.find(uf.parent[x])
	}
	return uf.parent[x]
}

// union merges the sets containing x and y using union by rank to keep the
// tree shallow.
func (uf *refsUnionFind) union(x, y string) {
	rx, ry := uf.find(x), uf.find(y)
	if rx == ry {
		return
	}
	if uf.rank[rx] < uf.rank[ry] {
		rx, ry = ry, rx
	}
	uf.parent[ry] = rx
	if uf.rank[rx] == uf.rank[ry] {
		uf.rank[rx]++
	}
}

// buildRefsGraph loads all non-deleted issues and their relationships from
// svc via a single GetGraphData call, then computes connected components of
// the refs undirected graph across non-closed issues. Edges where either
// endpoint is closed are suppressed.
func buildRefsGraph(ctx context.Context, svc driving.Service) (*refsGraph, error) {
	data, err := svc.GetGraphData(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading graph data for refs section: %w", err)
	}

	// Index non-closed issues by ID for O(1) membership and title lookup.
	nonClosed := make(map[string]string, len(data.Nodes)) // ID → title
	for _, node := range data.Nodes {
		if node.State != domain.StateClosed {
			nonClosed[node.ID] = node.Title
		}
	}

	// Filter to refs relationships where both endpoints are non-closed.
	// GetGraphData may return both directions of a symmetric refs edge, so
	// normalize to (leftID < rightID) and deduplicate before building the
	// component structure.
	seen := make(map[string]bool)
	var edges []refsEdge
	for _, rel := range data.Relationships {
		if rel.Type != domain.RelRefs.String() {
			continue
		}
		if _, ok := nonClosed[rel.SourceID]; !ok {
			continue
		}
		if _, ok := nonClosed[rel.TargetID]; !ok {
			continue
		}
		leftID, rightID := rel.SourceID, rel.TargetID
		if leftID > rightID {
			leftID, rightID = rightID, leftID
		}
		key := leftID + "\x00" + rightID
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, refsEdge{
			leftID:     leftID,
			leftTitle:  nonClosed[leftID],
			rightID:    rightID,
			rightTitle: nonClosed[rightID],
		})
	}

	if len(edges) == 0 {
		return &refsGraph{}, nil
	}

	// Compute connected components via union-find.
	uf := newRefsUnionFind()
	for _, e := range edges {
		uf.union(e.leftID, e.rightID)
	}

	// Group edges and issue IDs by their component root.
	edgesByRoot := make(map[string][]refsEdge)
	issueSetByRoot := make(map[string]map[string]bool)
	for _, e := range edges {
		root := uf.find(e.leftID)
		edgesByRoot[root] = append(edgesByRoot[root], e)
		if issueSetByRoot[root] == nil {
			issueSetByRoot[root] = make(map[string]bool)
		}
		issueSetByRoot[root][e.leftID] = true
		issueSetByRoot[root][e.rightID] = true
	}

	components := make([]refsComponent, 0, len(edgesByRoot))
	for root, es := range edgesByRoot {
		issueSet := issueSetByRoot[root]
		issueIDs := make([]string, 0, len(issueSet))
		for id := range issueSet {
			issueIDs = append(issueIDs, id)
		}
		slices.Sort(issueIDs)

		// Sort edges within the component by (leftID, rightID) for deterministic output.
		slices.SortFunc(es, func(a, b refsEdge) int {
			if c := cmp.Compare(a.leftID, b.leftID); c != 0 {
				return c
			}
			return cmp.Compare(a.rightID, b.rightID)
		})

		components = append(components, refsComponent{
			edges:    es,
			issueIDs: issueIDs,
		})
	}

	// Sort components: largest first by issue count; ties broken by smallest
	// issue ID in the component (the first element of the sorted issueIDs slice).
	slices.SortFunc(components, func(a, b refsComponent) int {
		if len(a.issueIDs) != len(b.issueIDs) {
			// Descending by issue count — larger component first.
			return cmp.Compare(len(b.issueIDs), len(a.issueIDs))
		}
		// Ascending by smallest issue ID for deterministic tie-breaking.
		return cmp.Compare(a.issueIDs[0], b.issueIDs[0])
	})

	return &refsGraph{
		components: components,
		totalEdges: len(edges),
	}, nil
}

// refsRowOverhead is the number of non-title characters consumed by a refs
// edge row at the terminal. The format is:
//
//	"  {leftID}  {leftTitle}  —  {rightID}  {rightTitle}"
//
// Fixed overhead: 2 (indent) + 10 (left ID) + 2 (between left ID and left
// title) + 5 ("  —  " between the two titles) + 10 (right ID) + 2 (between
// right ID and right title) = 31. The 10-char allowance per ID assumes a
// prefix length up to ~5 chars plus the 5-char random suffix.
const refsRowOverhead = 31

// RenderRefsSection renders the contextual reference section of np rel list.
// It loads all non-deleted issues and their refs relationships via a single
// GetGraphData call, computes connected components of the undirected refs
// graph (ignoring edges where either endpoint is closed), and writes the
// result as a flat edge list grouped by component.
//
// Components are printed largest first (by issue count); ties are broken by
// the lexicographically smallest issue ID in the component. Within each
// component, edges are listed one per line with endpoints sorted
// lexicographically (smaller ID on the left). Each undirected edge appears
// exactly once.
//
// On TTY, each title is truncated to half of the available terminal width.
// On non-TTY, full titles are emitted.
//
// The section header always appears, even when there are no refs edges.
func RenderRefsSection(ctx context.Context, svc driving.Service, ios *iostreams.IOStreams) error {
	g, err := buildRefsGraph(ctx, svc)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(ios.Out, "Refs (%d components, %d edges)\n\n",
		len(g.components), g.totalEdges); err != nil {
		return fmt.Errorf("writing refs header: %w", err)
	}

	if len(g.components) == 0 {
		return nil
	}

	termWidth := ios.TerminalWidth()
	// Split the available title budget evenly between the left and right title.
	perTitle := cmdutil.AvailableTitleWidth(termWidth, refsRowOverhead) / 2

	for i, comp := range g.components {
		if _, err := fmt.Fprintf(ios.Out, "Component %d (%d issues, %d edges)\n",
			i+1, len(comp.issueIDs), len(comp.edges)); err != nil {
			return fmt.Errorf("writing component %d header: %w", i+1, err)
		}
		for _, e := range comp.edges {
			leftTitle := cmdutil.TruncateTitle(e.leftTitle, perTitle)
			rightTitle := cmdutil.TruncateTitle(e.rightTitle, perTitle)
			if _, err := fmt.Fprintf(ios.Out, "  %s  %s  —  %s  %s\n",
				e.leftID, leftTitle, e.rightID, rightTitle); err != nil {
				return fmt.Errorf("writing refs edge: %w", err)
			}
		}
		// Blank line between components but not after the last one.
		if i < len(g.components)-1 {
			if _, err := fmt.Fprintln(ios.Out); err != nil {
				return fmt.Errorf("writing component separator: %w", err)
			}
		}
	}

	return nil
}

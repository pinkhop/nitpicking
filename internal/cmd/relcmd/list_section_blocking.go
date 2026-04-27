package relcmd

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// blockingGraph holds the pre-computed data structures for the blocking section
// of np rel list. All slices are built from non-closed issues and their direct
// blocking edges only.
type blockingGraph struct {
	// byID maps every non-closed issue ID to its list-item DTO.
	byID map[string]driving.IssueListItemDTO
	// blocks maps a blocker ID to the sorted slice of IDs it directly blocks.
	// Sorted by priority (ascending) then ID (ascending) so the walk is deterministic.
	blocks map[string][]string
	// roots is the sorted slice of root IDs: non-closed issues that block at
	// least one non-closed issue but are not themselves blocked by any non-closed
	// issue. Cycle nodes are promoted to roots after cycle detection. Sorted by
	// priority (ascending) then ID (ascending).
	roots []string
	// edges is the total number of directed blocking edges (A blocks B) in the graph.
	edges int
	// cycles holds each detected cycle as a slice of IDs forming the cycle path.
	// The last element equals the first to represent the closed loop.
	cycles [][]string
}

// buildBlockingGraph loads all non-closed issues from svc and constructs the
// blocking DAG. It makes a single ListIssues call and processes results in
// memory. Each issue's BlockerIDs field provides the direct blocking edges.
func buildBlockingGraph(ctx context.Context, svc driving.Service) (*blockingGraph, error) {
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  driving.IssueFilterInput{ExcludeClosed: true},
		OrderBy: driving.OrderByID,
		Limit:   -1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing issues for blocking section: %w", err)
	}

	byID := make(map[string]driving.IssueListItemDTO, len(result.Items))
	for _, item := range result.Items {
		byID[item.ID] = item
	}

	// Build the blocking adjacency map from each issue's BlockerIDs. An edge
	// (blockerID → issueID) is included only when both endpoints are non-closed
	// (both present in byID).
	blocks := make(map[string][]string, len(result.Items))
	blockedSet := make(map[string]bool, len(result.Items)) // IDs blocked by at least one non-closed blocker
	totalEdges := 0

	for _, item := range result.Items {
		for _, blockerID := range item.BlockerIDs {
			if _, ok := byID[blockerID]; !ok {
				// Blocker is not in the non-closed set — skip this edge.
				continue
			}
			blocks[blockerID] = append(blocks[blockerID], item.ID)
			blockedSet[item.ID] = true
			totalEdges++
		}
	}

	// Sort each adjacency slice by priority (ascending = highest priority first)
	// then by ID (ascending) for deterministic traversal.
	for id := range blocks {
		slices.SortFunc(blocks[id], func(a, b string) int {
			ia, ib := byID[a], byID[b]
			if ia.Priority != ib.Priority {
				return cmp.Compare(int(ia.Priority), int(ib.Priority))
			}
			return cmp.Compare(ia.ID, ib.ID)
		})
	}

	// Collect all non-closed issue IDs for the cycle detection pass.
	allIDs := make([]string, 0, len(byID))
	for id := range byID {
		allIDs = append(allIDs, id)
	}
	slices.Sort(allIDs) // deterministic DFS start order

	// Detect cycles. Cycle nodes are promoted to roots below.
	cycles, cycleNodeSet := detectBlockingCycles(allIDs, blocks)

	// Build the initial root set: non-closed issues that block something and
	// are not themselves blocked by any non-closed issue.
	rootSet := make(map[string]bool, len(blocks))
	for id := range blocks {
		if !blockedSet[id] {
			rootSet[id] = true
		}
	}

	// Determine which nodes are already reachable from the regular roots so
	// that cycle nodes reachable from a regular root are not redundantly promoted
	// to additional roots. This prevents inflating the chain count and emitting
	// spurious depth-0 back-reference rows for cycles that a regular root already
	// reaches. The visited check inside markReachable handles the cycle case and
	// prevents infinite recursion.
	reachableFromRoots := make(map[string]bool, len(byID))
	var markReachable func(id string)
	markReachable = func(id string) {
		if reachableFromRoots[id] {
			return
		}
		reachableFromRoots[id] = true
		for _, child := range blocks[id] {
			markReachable(child)
		}
	}
	for id := range rootSet {
		markReachable(id)
	}

	// Promote cycle nodes to roots so the output remains useful even when the
	// graph has cycles. Only promote nodes not already reachable from a regular
	// root — those will be visited naturally during the walk from their root.
	for cycleNode := range cycleNodeSet {
		if !reachableFromRoots[cycleNode] {
			rootSet[cycleNode] = true
		}
	}

	// Build the sorted roots slice.
	roots := make([]string, 0, len(rootSet))
	for id := range rootSet {
		roots = append(roots, id)
	}
	slices.SortFunc(roots, func(a, b string) int {
		ia, ib := byID[a], byID[b]
		if ia.Priority != ib.Priority {
			return cmp.Compare(int(ia.Priority), int(ib.Priority))
		}
		return cmp.Compare(ia.ID, ib.ID)
	})

	return &blockingGraph{
		byID:   byID,
		blocks: blocks,
		roots:  roots,
		edges:  totalEdges,
		cycles: cycles,
	}, nil
}

// detectBlockingCycles performs a DFS over the blocking graph to find directed
// cycles. It returns each cycle as a slice of IDs with the first ID repeated at
// the end to show the closed loop (e.g., [A, B, C, A]), and a set of all IDs
// that participate in at least one cycle.
func detectBlockingCycles(allIDs []string, blocks map[string][]string) (cycles [][]string, cycleNodeSet map[string]bool) {
	// color: 0 = unvisited (white), 1 = on current DFS path (gray), 2 = fully processed (black).
	color := make(map[string]int, len(allIDs))
	// pathIdx maps an ID to its position in the current DFS path for O(1) cycle extraction.
	pathIdx := make(map[string]int, len(allIDs))
	path := make([]string, 0, len(allIDs))
	cycleNodeSet = make(map[string]bool)

	var dfs func(id string)
	dfs = func(id string) {
		color[id] = 1
		pathIdx[id] = len(path)
		path = append(path, id)

		for _, child := range blocks[id] {
			switch color[child] {
			case 0:
				dfs(child)
			case 1:
				// Back edge: child is an ancestor on the current path — we have a cycle.
				start := pathIdx[child]
				cycle := make([]string, len(path)-start+1)
				copy(cycle, path[start:])
				cycle[len(cycle)-1] = child // close the loop
				cycles = append(cycles, cycle)
				for i := start; i < len(path); i++ {
					cycleNodeSet[path[i]] = true
				}
			}
		}

		path = path[:len(path)-1]
		delete(pathIdx, id)
		color[id] = 2
	}

	for _, id := range allIDs {
		if color[id] == 0 {
			dfs(id)
		}
	}

	return cycles, cycleNodeSet
}

// blockingWalkState holds the mutable state for a single DFS traversal of the
// blocking DAG that builds the []TreeNode slice for rendering.
type blockingWalkState struct {
	// byID provides issue data for each non-closed ID.
	byID map[string]driving.IssueListItemDTO
	// blocks is the pre-sorted blocking adjacency map.
	blocks map[string][]string
	// seen maps an issue ID to the parent node ID at its first appearance in the
	// walk. An empty string means the issue appeared as a root (depth 0).
	seen map[string]string
	// nodes accumulates the output in traversal order.
	nodes []TreeNode
	// termWidth drives title truncation (0 = non-TTY, no truncation).
	termWidth int
}

// walk appends a TreeNode (or a back-reference node) for id at the given depth,
// then recurses into its children. parentID is the caller's issue ID, or "" when
// id is a root.
func (w *blockingWalkState) walk(id, parentID string, depth int) {
	if firstParent, already := w.seen[id]; already {
		// This node was already rendered earlier in the walk. Emit a back-reference
		// marker instead of repeating the full row.
		w.nodes = append(w.nodes, TreeNode{
			Kind:            NodeKindBackRef,
			Depth:           depth,
			IssueID:         id,
			BackRefParentID: firstParent,
		})
		return
	}
	w.seen[id] = parentID

	item := w.byID[id]
	overhead := treeBaseOverhead + depth*2
	item.Title = cmdutil.TruncateTitle(item.Title, cmdutil.AvailableTitleWidth(w.termWidth, overhead))

	w.nodes = append(w.nodes, TreeNode{
		Kind:      NodeKindIssue,
		Depth:     depth,
		IssueID:   id,
		IssueItem: item,
	})

	for _, childID := range w.blocks[id] {
		w.walk(childID, id, depth+1)
	}
}

// buildBlockingNodes walks the blocking DAG from the given roots in order and
// returns a flat []TreeNode slice for rendering via RenderTreeText.
func buildBlockingNodes(
	roots []string,
	blocks map[string][]string,
	byID map[string]driving.IssueListItemDTO,
	termWidth int,
) []TreeNode {
	state := &blockingWalkState{
		byID:      byID,
		blocks:    blocks,
		seen:      make(map[string]string, len(byID)),
		termWidth: termWidth,
	}
	for _, rootID := range roots {
		state.walk(rootID, "", 0)
	}
	return state.nodes
}

// RenderBlockingSection renders the blocking dependency section of np rel list.
// It builds a directed acyclic graph (DAG) of blocking relationships across all
// non-closed issues, detects cycles, and writes the result as a columnar table
// using the shared RenderTreeText renderer.
//
// Roots are non-closed issues that block at least one other non-closed issue
// but are not themselves blocked. Cycle nodes are promoted to roots. The walk
// proceeds depth-first from each root sorted by priority (P0 first) then ID.
// Issues reached via multiple paths appear in full at their first occurrence;
// subsequent occurrences emit a back-reference marker showing where they first
// appeared.
//
// The section header always appears even when there are no blocking edges. Its
// format is "Blocking (N chains, M edges, K cycles)". Cycle banners appear
// between the section header and the tree listing.
func RenderBlockingSection(ctx context.Context, svc driving.Service, ios *iostreams.IOStreams) error {
	g, err := buildBlockingGraph(ctx, svc)
	if err != nil {
		return err
	}

	// Section header.
	if _, err := fmt.Fprintf(ios.Out, "Blocking (%d chains, %d edges, %d cycles)\n\n",
		len(g.roots), g.edges, len(g.cycles)); err != nil {
		return fmt.Errorf("writing blocking header: %w", err)
	}

	// Cycle banners — one per detected cycle, placed between the header and the tree.
	for _, cycle := range g.cycles {
		banner := "Cycles detected: " + strings.Join(cycle, "→")
		if _, err := fmt.Fprintln(ios.Out, banner); err != nil {
			return fmt.Errorf("writing cycle banner: %w", err)
		}
	}
	if len(g.cycles) > 0 {
		if _, err := fmt.Fprintln(ios.Out); err != nil {
			return fmt.Errorf("writing post-cycle blank line: %w", err)
		}
	}

	if len(g.roots) == 0 {
		return nil
	}

	termWidth := ios.TerminalWidth()
	nodes := buildBlockingNodes(g.roots, g.blocks, g.byID, termWidth)

	if err := RenderTreeText(ios, nodes); err != nil {
		return fmt.Errorf("rendering blocking section: %w", err)
	}

	return nil
}

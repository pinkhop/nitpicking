package relcmd

import (
	"context"
	"fmt"
	"slices"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// parentChildForest holds the pre-computed data structures for the
// parent-child section of np rel list. It is built from all non-closed issues
// in the database and represents the forest of non-closed parent-child trees.
type parentChildForest struct {
	// byID maps non-closed issue IDs to their list-item DTO.
	byID map[string]driving.IssueListItemDTO
	// childrenOf maps a parent ID to its sorted slice of non-closed child IDs.
	// Only edges where both endpoints are non-closed are stored.
	childrenOf map[string][]string
	// roots is the sorted list of qualifying root IDs: non-closed issues
	// with no non-closed parent and at least one non-closed child.
	roots []string
	// totalIssues is the count of non-closed issues across all rendered
	// subtrees, including the root issues themselves.
	totalIssues int
}

// buildParentChildForest loads all non-closed issues from svc and constructs
// the forest data structures used by RenderParentChildSection. It makes a
// single ListIssues call and processes the results in memory.
func buildParentChildForest(ctx context.Context, svc driving.Service) (*parentChildForest, error) {
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  driving.IssueFilterInput{ExcludeClosed: true},
		OrderBy: driving.OrderByID,
		Limit:   -1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing issues for parent-child section: %w", err)
	}

	byID := make(map[string]driving.IssueListItemDTO, len(result.Items))
	for _, item := range result.Items {
		byID[item.ID] = item
	}

	// Build the children map. An edge A→B is included only when both A and B
	// are non-closed (both present in byID). Issues whose parent is closed
	// (parent ID set but parent not in byID) become virtual roots below.
	//
	// Children are appended in ascending ID order because ListIssues returns
	// items sorted by ID.
	childrenOf := make(map[string][]string, len(result.Items))
	for _, item := range result.Items {
		if item.ParentID == "" {
			continue
		}
		if _, ok := byID[item.ParentID]; ok {
			childrenOf[item.ParentID] = append(childrenOf[item.ParentID], item.ID)
		}
	}

	// Collect qualifying roots: non-closed issues with no non-closed parent
	// and at least one non-closed child. Items are iterated in ascending ID
	// order (from the sorted ListIssues result), so roots is already sorted.
	var roots []string
	for _, item := range result.Items {
		_, parentInSet := byID[item.ParentID]
		isRoot := item.ParentID == "" || !parentInSet
		if isRoot && len(childrenOf[item.ID]) > 0 {
			roots = append(roots, item.ID)
		}
	}
	// Defensive sort — ListIssues ordering guarantees ascending, but make the
	// invariant explicit in case the query plan changes in the future.
	slices.Sort(roots)

	// Count all non-closed issues that appear in rendered subtrees.
	totalIssues := 0
	for _, rootID := range roots {
		totalIssues += countParentChildSubtree(rootID, childrenOf)
	}

	return &parentChildForest{
		byID:        byID,
		childrenOf:  childrenOf,
		roots:       roots,
		totalIssues: totalIssues,
	}, nil
}

// countParentChildSubtree recursively counts all issues in the subtree rooted
// at id, including id itself. Only edges present in childrenOf are traversed,
// so only non-closed issues contribute.
func countParentChildSubtree(id string, childrenOf map[string][]string) int {
	count := 1
	for _, childID := range childrenOf[id] {
		count += countParentChildSubtree(childID, childrenOf)
	}
	return count
}

// maxDepthInSubtree returns the maximum depth of any node in the subtree rooted
// at id, where the root itself is at the given startDepth. It traverses only
// edges present in childrenOf, so closed issues (absent from childrenOf) do not
// contribute. The caller is responsible for ensuring childrenOf describes a
// forest (no cycles); a cycle would cause infinite recursion, identical to the
// precondition already held by countParentChildSubtree above.
func maxDepthInSubtree(id string, childrenOf map[string][]string, startDepth int) int {
	deepest := startDepth
	for _, childID := range childrenOf[id] {
		if d := maxDepthInSubtree(childID, childrenOf, startDepth+1); d > deepest {
			deepest = d
		}
	}
	return deepest
}

// buildParentChildNodes constructs a flat []TreeNode slice for the subtree
// rooted at id in full-expansion mode (no placeholders, no single focus).
// Titles are pre-truncated using titleOverhead, which must be a uniform value
// computed once for the entire table via cmdutil.UniformTreeOverhead. Passing a
// uniform overhead ensures every row gets the same available title space,
// matching the TableWriter's behaviour of padding the TREE column to its
// maximum width across all rows. A zero termWidth signals non-TTY output and
// suppresses truncation via AvailableTitleWidth returning 0.
func buildParentChildNodes(
	id string,
	depth int,
	byID map[string]driving.IssueListItemDTO,
	childrenOf map[string][]string,
	termWidth int,
	titleOverhead int,
) []TreeNode {
	item := byID[id]
	item.Title = cmdutil.TruncateTitle(item.Title, cmdutil.AvailableTitleWidth(termWidth, titleOverhead))

	nodes := []TreeNode{{
		Kind:      NodeKindIssue,
		Depth:     depth,
		IssueID:   id,
		IssueItem: item,
	}}

	for _, childID := range childrenOf[id] {
		nodes = append(nodes, buildParentChildNodes(childID, depth+1, byID, childrenOf, termWidth, titleOverhead)...)
	}

	return nodes
}

// RenderParentChildSection renders the parent-child hierarchy section of
// np rel list. It builds a forest of non-closed issue trees and writes them
// as a single aligned table — one header row covering all trees — using the
// existing RenderTreeText renderer. A root qualifies when it is non-closed,
// has no non-closed parent, and has at least one non-closed child.
//
// The section header always appears, even when no qualifying roots exist. The
// format is "Parent-child (N roots, M issues)" where N is the number of roots
// rendered and M is the total non-closed issue count across all subtrees.
//
// Title truncation is applied on TTY output using the terminal width from ios;
// non-TTY output emits full titles.
func RenderParentChildSection(ctx context.Context, svc driving.Service, ios *iostreams.IOStreams) error {
	forest, err := buildParentChildForest(ctx, svc)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(ios.Out, "Parent-child (%d roots, %d issues)\n\n",
		len(forest.roots), forest.totalIssues); err != nil {
		return fmt.Errorf("writing parent-child header: %w", err)
	}

	if len(forest.roots) == 0 {
		return nil
	}

	termWidth := ios.TerminalWidth()

	// All roots are rendered into a single TableWriter call, so the TREE column
	// width is the maximum across every row in the entire forest — including the
	// deepest child at any root. Pre-truncating each row with its own per-depth
	// overhead would give root rows (depth 0) more title space than child rows
	// (depth 1+), even though the rendered table gives every row the same TREE
	// column width. Use a single uniform overhead based on the global max depth.
	maxDepth := 0
	for _, rootID := range forest.roots {
		if d := maxDepthInSubtree(rootID, forest.childrenOf, 0); d > maxDepth {
			maxDepth = d
		}
	}
	titleOverhead := cmdutil.UniformIssueTreeOverhead(maxDepth)

	// Collect nodes from all roots into one slice so the entire forest
	// renders as a single aligned table with one header row.
	var allNodes []TreeNode
	for _, rootID := range forest.roots {
		allNodes = append(allNodes, buildParentChildNodes(rootID, 0, forest.byID, forest.childrenOf, termWidth, titleOverhead)...)
	}

	if err := RenderTreeText(ios, allNodes); err != nil {
		return fmt.Errorf("rendering parent-child forest: %w", err)
	}

	return nil
}

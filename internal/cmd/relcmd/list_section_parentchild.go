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

// treeBaseOverhead estimates the non-title character overhead for the tree
// table at depth zero. The TREE column is a 10-char ID (12 with 2-char padding),
// P is 2 chars (4 with padding), ROLE is 5 chars (7 with padding), and STATE
// is 14 chars worst-case (16 with padding), summing to 39.
// Depth-based indentation is added separately at 2 chars per depth level.
const treeBaseOverhead = 39

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

// buildParentChildNodes constructs a flat []TreeNode slice for the subtree
// rooted at id in full-expansion mode (no placeholders, no single focus).
// Titles are pre-truncated based on termWidth and the per-depth overhead; a
// zero termWidth signals non-TTY output and suppresses truncation via
// AvailableTitleWidth returning 0.
func buildParentChildNodes(
	id string,
	depth int,
	byID map[string]driving.IssueListItemDTO,
	childrenOf map[string][]string,
	termWidth int,
) []TreeNode {
	item := byID[id]

	// Overhead grows with depth because each level adds 2 spaces of indent
	// in the TREE column.
	overhead := treeBaseOverhead + depth*2
	item.Title = cmdutil.TruncateTitle(item.Title, cmdutil.AvailableTitleWidth(termWidth, overhead))

	nodes := []TreeNode{{
		Kind:      NodeKindIssue,
		Depth:     depth,
		IssueID:   id,
		IssueItem: item,
	}}

	for _, childID := range childrenOf[id] {
		nodes = append(nodes, buildParentChildNodes(childID, depth+1, byID, childrenOf, termWidth)...)
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

	// Collect nodes from all roots into one slice so the entire forest
	// renders as a single aligned table with one header row.
	var allNodes []TreeNode
	for _, rootID := range forest.roots {
		allNodes = append(allNodes, buildParentChildNodes(rootID, 0, forest.byID, forest.childrenOf, termWidth)...)
	}

	if err := RenderTreeText(ios, allNodes); err != nil {
		return fmt.Errorf("rendering parent-child forest: %w", err)
	}

	return nil
}

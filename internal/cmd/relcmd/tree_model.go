package relcmd

import (
	"context"
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// NodeKind distinguishes fully-expanded issue nodes from placeholder entries
// that represent collapsed siblings.
type NodeKind int

const (
	// NodeKindIssue is a fully-expanded node that carries complete issue data.
	NodeKindIssue NodeKind = iota

	// NodeKindPlaceholder represents a group of collapsed siblings at a given
	// tier. The Count field tells renderers how many siblings were collapsed.
	// Placeholder nodes always appear after the expanded path element at their
	// tier.
	NodeKindPlaceholder

	// NodeKindBackRef marks an issue that has already been rendered at an
	// earlier position in the output. Used by the blocking section to deduplicate
	// issues reached via multiple paths in the dependency DAG. The IssueID field
	// holds the deduplicated issue's ID; BackRefParentID holds the parent node ID
	// at the issue's first appearance ("" when it first appeared as a root).
	NodeKindBackRef
)

// TreeNode is a single entry in the in-memory tree model produced by
// BuildTreeModel. It is rendering-agnostic — text and JSON renderers both
// consume this structure and produce their output independently.
//
// When Kind == NodeKindIssue, all Issue* fields are populated and Count is
// zero. When Kind == NodeKindPlaceholder, only Depth and Count are meaningful;
// Issue* fields are empty.
type TreeNode struct {
	// Kind distinguishes expanded issue nodes from sibling placeholder entries.
	Kind NodeKind

	// Depth is the zero-based depth in the tree. The root ancestor is at depth
	// zero; each child tier increments the depth by one.
	Depth int

	// IsFocus is true for the single node that corresponds to the focus issue
	// supplied to BuildTreeModel. Renderers use this to apply emphasis (e.g.,
	// bold on TTY). Only meaningful when Kind == NodeKindIssue.
	IsFocus bool

	// IssueID is the string representation of the issue ID (e.g., "NP-a3bxr").
	// Empty when Kind == NodeKindPlaceholder.
	IssueID string

	// IssueItem carries the full list-item DTO for the issue. The zero value
	// when Kind == NodeKindPlaceholder.
	IssueItem driving.IssueListItemDTO

	// Count is the number of collapsed siblings this placeholder represents.
	// Zero when Kind == NodeKindIssue.
	Count int

	// BackRefParentID is the ID of the node that was the parent of this issue at
	// its first appearance in the output. Empty when the issue first appeared as a
	// root (depth 0). Only meaningful when Kind == NodeKindBackRef.
	BackRefParentID string
}

// treeService is the minimal subset of driving.Service consumed by
// BuildTreeModel. Declaring a narrow interface here keeps the function
// testable without wiring the full service.
type treeService interface {
	// ListIssues returns a filtered, ordered, paginated list of issues.
	ListIssues(ctx context.Context, input driving.ListIssuesInput) (driving.ListIssuesOutput, error)

	// ShowIssue returns the full detail view of an issue, including its
	// ParentID. Used to retrieve the focus issue's parent chain.
	ShowIssue(ctx context.Context, id string) (driving.ShowIssueOutput, error)
}

// treeData bundles the indexed tree information loaded from the service so
// that both the text tree model and the JSON tree model can share a single
// data-loading pass. Fields are immutable once loadTreeData returns.
type treeData struct {
	// rootID is the ID of the tree's root ancestor. Equals focusID when the
	// focus has no parent.
	rootID string

	// byID maps every issue ID in the tree (root inclusive) to its list-item
	// projection.
	byID map[string]driving.IssueListItemDTO

	// children maps each parent ID to its sorted slice of child IDs (ascending
	// by ID).
	children map[string][]string

	// pathSet is the set of IDs on the ancestry path from root to focus,
	// inclusive of both endpoints.
	pathSet map[string]bool

	// focusDescendants is the set of IDs that are descendants of the focus (not
	// including the focus itself). Used to determine which subtrees must be
	// fully expanded in non-full mode.
	focusDescendants map[string]bool
}

// loadTreeData performs the shared service-call sequence used by both the text
// tree model builder and the JSON tree builder. It fetches the ancestry chain,
// every descendant of the root, and the root issue itself, then builds the
// ancillary lookup maps (byID, children, pathSet, focusDescendants). The
// returned treeData contains everything the tree walkers need to render either
// a flat text model or a nested JSON tree without issuing any further service
// calls.
//
// svc must not be nil. focusID must be a valid issue ID present in the
// database.
func loadTreeData(ctx context.Context, svc treeService, focusID string) (*treeData, error) {
	// Resolve the ancestry chain: all ancestors from the root down to the focus
	// issue.
	ancestorsResult, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  driving.IssueFilterInput{AncestorsOf: focusID},
		OrderBy: driving.OrderByID,
		Limit:   -1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing ancestors: %w", err)
	}
	anchorPath := buildAnchorPath(ancestorsResult.Items)

	// Determine the root ancestor.
	rootID := focusID
	if len(anchorPath) > 0 {
		rootID = anchorPath[0]
	}

	// Fetch every descendant of the root so we can build the full tree from a
	// single query. DescendantsOf excludes the root itself, so we also need to
	// fetch the root via ShowIssue.
	allResult, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  driving.IssueFilterInput{DescendantsOf: rootID},
		OrderBy: driving.OrderByID,
		Limit:   -1,
	})
	if err != nil {
		return nil, fmt.Errorf("listing descendants of root: %w", err)
	}

	rootShown, err := svc.ShowIssue(ctx, rootID)
	if err != nil {
		return nil, fmt.Errorf("looking up root issue: %w", err)
	}
	rootItem := showToListItem(rootShown)

	// Index all items by their ID for O(1) lookup.
	byID := make(map[string]driving.IssueListItemDTO, len(allResult.Items)+1)
	byID[rootID] = rootItem
	for _, item := range allResult.Items {
		byID[item.ID] = item
	}

	// Build a children map keyed by parent ID. Child slices are already in
	// ascending ID order because OrderByID produced ascending results.
	children := make(map[string][]string)
	for _, item := range allResult.Items {
		parent := item.ParentID
		if parent == "" {
			parent = rootID
		}
		children[parent] = append(children[parent], item.ID)
	}

	// Build the path set (root → focus, inclusive) for quick membership tests.
	pathSet := make(map[string]bool, len(anchorPath)+1)
	for _, id := range anchorPath {
		pathSet[id] = true
	}
	pathSet[focusID] = true

	// Collect every descendant of the focus so non-full mode knows to fully
	// expand the focus's subtree.
	focusDescendants := make(map[string]bool)
	collectDescendants(focusID, children, focusDescendants)

	return &treeData{
		rootID:           rootID,
		byID:             byID,
		children:         children,
		pathSet:          pathSet,
		focusDescendants: focusDescendants,
	}, nil
}

// BuildTreeModel constructs an ordered, flat slice of TreeNode values that
// represents the tree view for the given focus issue.
//
// When full is false (default mode), the model contains:
//   - every ancestor from the root down to the focus issue (one node each),
//   - the full descendant subtree of the focus issue, and
//   - placeholder entries at each ancestor tier for siblings of the path
//     element that are NOT on the ancestry path (these always appear after the
//     expanded path element at their tier, reflecting siblings sorted after it).
//
// When full is true, the model contains the complete tree rooted at the root
// ancestor with no placeholder entries. The result is identical to calling
// BuildTreeModel with the root ancestor ID and full == false on a tree where
// the root ancestor has no unexpanded siblings.
//
// Children at every tier are sorted ascending by issue ID (lexicographic on
// the string representation). Placeholder entries appear after the expanded
// path element at their tier — they summarize all siblings that sort after the
// expanded element (the implementation counts siblings on both sides, because
// sibling-count for the placeholder is all siblings except the path element).
//
// The svc parameter must not be nil. focusID must be a valid issue ID string
// present in the database.
func BuildTreeModel(ctx context.Context, svc treeService, focusID string, full bool) ([]TreeNode, error) {
	data, err := loadTreeData(ctx, svc, focusID)
	if err != nil {
		return nil, err
	}

	// Walk the tree from the root and emit nodes in pre-order (parent before
	// children), respecting the full/non-full rendering mode.
	var nodes []TreeNode
	nodes = walkTree(nodes, data.rootID, 0, focusID, data.pathSet, data.focusDescendants, data.byID, data.children, full)
	return nodes, nil
}

// buildAnchorPath constructs the anchor path from root to focus (exclusive of
// the focus itself) using the raw AncestorsOf result. The AncestorsOf filter
// returns ancestors in the order determined by the OrderBy parameter; the
// ordering is not guaranteed to be root-first. We reconstruct the root-first
// chain from the ParentID fields: the root ancestor is the item whose parent
// is not itself in the ancestor set.
func buildAnchorPath(ancestors []driving.IssueListItemDTO) []string {
	if len(ancestors) == 0 {
		return nil
	}

	// Index ancestors by ID and mark which IDs are present in the ancestor set.
	ancestorIDs := make(map[string]bool, len(ancestors))
	for _, a := range ancestors {
		ancestorIDs[a.ID] = true
	}

	// The root ancestor is the one whose parent is not in the ancestor set.
	// An ancestor with an empty ParentID or with a parent outside the set is
	// the root.
	var rootID string
	for _, a := range ancestors {
		if !ancestorIDs[a.ParentID] {
			rootID = a.ID
			break
		}
	}
	if rootID == "" {
		// Circular or otherwise unexpected structure — return ancestors in their
		// original order rather than hiding data.
		path := make([]string, len(ancestors))
		for i, a := range ancestors {
			path[i] = a.ID
		}
		return path
	}

	// Build a forward-link map: parentID → childID within the ancestor set.
	// This lets us walk from root toward the focus in O(n).
	forwardLink := make(map[string]string, len(ancestors))
	for _, a := range ancestors {
		if a.ParentID != "" {
			forwardLink[a.ParentID] = a.ID
		}
	}

	path := make([]string, 0, len(ancestors))
	for cur := rootID; cur != ""; cur = forwardLink[cur] {
		path = append(path, cur)
	}
	return path
}

// collectDescendants recursively adds all descendant IDs of root to the given
// set.
func collectDescendants(id string, children map[string][]string, set map[string]bool) {
	for _, child := range children[id] {
		set[child] = true
		collectDescendants(child, children, set)
	}
}

// walkTree appends TreeNode entries to nodes in pre-order for the subtree
// rooted at id. depth is the current zero-based depth. The function applies
// full/non-full visibility rules:
//
//   - In full mode, every node in the entire tree is expanded; no placeholders
//     are emitted.
//   - In non-full mode, only nodes on the path from the root to the focus
//     (pathSet) and the focus's full subtree (focusDescendants) are expanded.
//     At each ancestor tier, siblings of the path element that are not on the
//     path are collapsed into a single placeholder entry.
func walkTree(
	nodes []TreeNode,
	id string,
	depth int,
	focusID string,
	pathSet map[string]bool,
	focusDescendants map[string]bool,
	byID map[string]driving.IssueListItemDTO,
	children map[string][]string,
	full bool,
) []TreeNode {
	item := byID[id]

	// Emit the node for this issue.
	nodes = append(nodes, TreeNode{
		Kind:      NodeKindIssue,
		Depth:     depth,
		IsFocus:   id == focusID,
		IssueID:   id,
		IssueItem: item,
	})

	// Determine which children to expand and whether a placeholder is needed.
	childIDs := children[id]
	if len(childIDs) == 0 {
		return nodes
	}

	if full {
		// Expand every child unconditionally — no placeholders in full mode.
		for _, childID := range childIDs {
			nodes = walkTree(nodes, childID, depth+1, focusID, pathSet, focusDescendants, byID, children, full)
		}
		return nodes
	}

	// Non-full mode: decide which children to expand and how many to collapse.
	//
	// If the current node is on the ancestry path (and is not the focus), we
	// expand only the path child and collapse siblings. If the current node is
	// the focus or a descendant of the focus, we expand all children.
	if id == focusID || focusDescendants[id] {
		// Expand every child of the focus's subtree.
		for _, childID := range childIDs {
			nodes = walkTree(nodes, childID, depth+1, focusID, pathSet, focusDescendants, byID, children, full)
		}
		return nodes
	}

	// Current node is an ancestor (path element) but not the focus. Find
	// which child continues the path.
	var pathChild string
	for _, childID := range childIDs {
		if pathSet[childID] {
			pathChild = childID
			break
		}
	}

	if pathChild == "" {
		// No child is on the path — this should not happen in a consistent tree,
		// but fall back to expanding all children to avoid silently hiding data.
		for _, childID := range childIDs {
			nodes = walkTree(nodes, childID, depth+1, focusID, pathSet, focusDescendants, byID, children, full)
		}
		return nodes
	}

	// Expand only the path child; count all other children as collapsed
	// siblings.
	siblingCount := len(childIDs) - 1
	nodes = walkTree(nodes, pathChild, depth+1, focusID, pathSet, focusDescendants, byID, children, full)

	if siblingCount > 0 {
		nodes = append(nodes, TreeNode{
			Kind:  NodeKindPlaceholder,
			Depth: depth + 1,
			Count: siblingCount,
		})
	}

	return nodes
}

// showToListItem converts a ShowIssueOutput into an IssueListItemDTO for use
// within the tree model. Only the fields relevant to tree rendering are
// populated. The DisplayStatus mirrors the format used by FormatState:
// "primary (secondary)" when a secondary state is present, or just "primary"
// when secondary is SecondaryNone.
func showToListItem(s driving.ShowIssueOutput) driving.IssueListItemDTO {
	var displayStatus string
	if s.SecondaryState == domain.SecondaryNone {
		displayStatus = s.State.String()
	} else {
		displayStatus = fmt.Sprintf("%s (%s)", s.State.String(), s.SecondaryState.String())
	}
	return driving.IssueListItemDTO{
		ID:             s.ID,
		Role:           s.Role,
		State:          s.State,
		Priority:       s.Priority,
		Title:          s.Title,
		ParentID:       s.ParentID,
		CreatedAt:      s.CreatedAt,
		SecondaryState: s.SecondaryState,
		DisplayStatus:  displayStatus,
	}
}

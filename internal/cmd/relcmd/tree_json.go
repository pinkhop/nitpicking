package relcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// jsonIssueNode is the JSON representation of an expanded issue node in the
// tree output. It carries all fields from the standard list-item shape (to
// remain byte-compatible with the ConvertListItems output) plus a Children
// array that holds either expanded child nodes or placeholder entries.
//
// The Children field is always present (even when empty) so that JSON consumers
// can reliably distinguish leaf nodes from unexpanded ones. Entries in the
// Children slice are typed as `any` to support the union of expanded
// jsonIssueNode and placeholder jsonPlaceholderNode shapes.
type jsonIssueNode struct {
	ID             string   `json:"id"`
	Role           string   `json:"role"`
	State          string   `json:"state"`
	SecondaryState string   `json:"secondary_state,omitempty"`
	DisplayStatus  string   `json:"display_status"`
	Priority       string   `json:"priority"`
	Title          string   `json:"title"`
	BlockerIDs     []string `json:"blocker_ids,omitempty"`
	ParentID       string   `json:"parent_id,omitempty"`
	CreatedAt      string   `json:"created_at"`
	Children       []any    `json:"children"`
}

// jsonPlaceholderNode is a minimal JSON object representing a collapsed
// sibling. It carries only the issue ID so that consumers can identify the
// sibling and look it up separately if they need its full detail.
type jsonPlaceholderNode struct {
	ID string `json:"id"`
}

// RenderTreeJSON builds the nested JSON tree for the given focus issue and
// writes it as indented JSON to w.
//
// The output shape is:
//   - A single top-level JSON object representing the root ancestor.
//   - Each expanded node carries a "children" array containing either fully
//     expanded child nodes (with all issue fields) or placeholder entries
//     ({"id": "<sibling-id>"} only) for unexpanded siblings.
//   - Without full: the tree contains the ancestry path from root to focus,
//     the focus's full subtree, and placeholders for unexpanded siblings at
//     each ancestor tier. Placeholders appear at their sorted position within
//     the children array (ascending by issue ID).
//   - With full: all nodes are expanded; no placeholders appear.
//
// The per-issue field shape is byte-compatible with cmdutil.ConvertListItems:
// id, role, state, secondary_state, display_status, priority, title,
// blocker_ids, parent_id, and created_at. The children field is added on top.
//
// JSON output is always plain text; no ANSI escape sequences are emitted
// regardless of whether w is a TTY.
//
// svc must not be nil. focusID must be a valid issue ID present in the
// database.
func RenderTreeJSON(ctx context.Context, w io.Writer, svc treeService, focusID string, full bool) error {
	root, err := buildJSONTree(ctx, svc, focusID, full)
	if err != nil {
		return fmt.Errorf("building JSON tree: %w", err)
	}
	return cmdutil.WriteJSON(w, root)
}

// buildJSONTree constructs the nested JSON tree by loading the shared tree
// data and then walking it recursively to produce a nested structure. The
// service-call sequence is shared with BuildTreeModel via loadTreeData.
func buildJSONTree(ctx context.Context, svc treeService, focusID string, full bool) (*jsonIssueNode, error) {
	data, err := loadTreeData(ctx, svc, focusID)
	if err != nil {
		return nil, err
	}
	return buildJSONNode(data.rootID, focusID, data.pathSet, data.focusDescendants, data.byID, data.children, full), nil
}

// buildJSONNode recursively constructs a jsonIssueNode for the given issue ID.
// It applies the same full/non-full visibility rules as walkTree:
//
//   - In full mode, all children are expanded.
//   - In non-full mode, only the path child is expanded at each ancestor tier;
//     the remaining siblings become placeholder entries.
//   - Descendants of the focus are always fully expanded in non-full mode.
//
// Children are always emitted in ascending ID order (the slice in childrenMap
// is already sorted). Placeholder entries appear at the position in the sorted
// order where the full sibling object would have appeared.
func buildJSONNode(
	id string,
	focusID string,
	pathSet map[string]bool,
	focusDescendants map[string]bool,
	byID map[string]driving.IssueListItemDTO,
	childrenMap map[string][]string,
	full bool,
) *jsonIssueNode {
	item := byID[id]
	node := issueItemToJSONNode(item)

	childIDs := childrenMap[id]
	if len(childIDs) == 0 {
		return node
	}

	if full {
		// Expand every child unconditionally.
		for _, childID := range childIDs {
			child := buildJSONNode(childID, focusID, pathSet, focusDescendants, byID, childrenMap, full)
			node.Children = append(node.Children, child)
		}
		return node
	}

	if id == focusID || focusDescendants[id] {
		// Expand every child of the focus's subtree.
		for _, childID := range childIDs {
			child := buildJSONNode(childID, focusID, pathSet, focusDescendants, byID, childrenMap, full)
			node.Children = append(node.Children, child)
		}
		return node
	}

	// Non-full mode on an ancestor tier: find the path child and expand it;
	// all other children become placeholders at their sorted position.
	var pathChild string
	for _, childID := range childIDs {
		if pathSet[childID] {
			pathChild = childID
			break
		}
	}

	if pathChild == "" {
		// Unexpected: no path child found. Fall back to expanding all children
		// to avoid hiding data silently.
		for _, childID := range childIDs {
			child := buildJSONNode(childID, focusID, pathSet, focusDescendants, byID, childrenMap, full)
			node.Children = append(node.Children, child)
		}
		return node
	}

	// Emit each child in sorted order: expanded for the path child, placeholder
	// for all others. This places each placeholder at its natural sorted position
	// within the children array.
	for _, childID := range childIDs {
		if childID == pathChild {
			child := buildJSONNode(childID, focusID, pathSet, focusDescendants, byID, childrenMap, full)
			node.Children = append(node.Children, child)
		} else {
			node.Children = append(node.Children, jsonPlaceholderNode{ID: childID})
		}
	}

	return node
}

// issueItemToJSONNode converts an IssueListItemDTO into a jsonIssueNode. The
// Children slice is initialized to an empty (non-nil) slice so that JSON output
// always contains "children": [] rather than "children": null for leaf nodes.
// The SecondaryState field is omitted when the issue has no secondary state
// (SecondaryNone.String() returns "").
func issueItemToJSONNode(item driving.IssueListItemDTO) *jsonIssueNode {
	return &jsonIssueNode{
		ID:             item.ID,
		Role:           item.Role.String(),
		State:          item.State.String(),
		SecondaryState: item.SecondaryState.String(),
		DisplayStatus:  item.DisplayStatus,
		Priority:       item.Priority.String(),
		Title:          item.Title,
		BlockerIDs:     item.BlockerIDs,
		ParentID:       item.ParentID,
		CreatedAt:      cmdutil.FormatJSONTimestamp(item.CreatedAt),
		Children:       []any{},
	}
}

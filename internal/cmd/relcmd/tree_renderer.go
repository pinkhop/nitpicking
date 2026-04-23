package relcmd

import (
	"fmt"
	"strings"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// TreeModelService is the subset of driving.Service consumed by BuildTreeModel.
// It is exported so that tests in the _test package can declare their own
// helpers that accept the interface without depending on the driving package
// directly.
type TreeModelService = treeService

// RenderTreeText writes the tree as a columnar text table to ios.Out.
//
// The table has five columns: TREE (issue ID indented two spaces per depth),
// P (priority), ROLE, STATE, and TITLE. The focus issue's row is bold when
// stdout is a TTY. Sibling placeholder rows are indented to the depth of the
// siblings they summarize and read "and N siblings" (or "and 1 sibling" for
// N=1). Column coloration matches np list (cs.Yellow for priority, cs.Dim for
// role, cmdutil.FormatState for state).
//
// When stdout is not a TTY, no ANSI escape sequences are emitted and alignment
// is preserved via cmdutil.TableWriter, which strips ANSI bytes before
// measuring column widths.
func RenderTreeText(ios *iostreams.IOStreams, nodes []TreeNode) error {
	w := ios.Out
	cs := ios.ColorScheme()

	tw := cmdutil.NewTableWriter(w, 2)

	// Header row.
	tw.AddRow("TREE", "P", "ROLE", "STATE", "TITLE")

	for _, node := range nodes {
		switch node.Kind {
		case NodeKindIssue:
			// TREE column: issue ID indented two spaces per depth level.
			indent := strings.Repeat("  ", node.Depth)
			treeCell := indent + node.IssueID

			// Apply bold to the focus row on TTY. Bold wraps the entire row's
			// TREE cell so the alignment measurement remains correct — the bold
			// sequences are stripped by TableWriter before computing widths.
			if node.IsFocus {
				treeCell = cs.Bold(indent + node.IssueID)
			}

			item := node.IssueItem

			// Priority column (yellow, matching np list).
			priorityCell := cs.Yellow(item.Priority.String())

			// Role column (dim, matching np list).
			roleCell := cs.Dim(item.Role.String())

			// State column (FormatState, matching np list).
			stateCell := cmdutil.FormatState(cs, item.State, item.SecondaryState)

			// Title column: plain text; no truncation for tree view.
			titleCell := item.Title
			if node.IsFocus {
				titleCell = cs.Bold(item.Title)
			}

			tw.AddRow(treeCell, priorityCell, roleCell, stateCell, titleCell)

		case NodeKindPlaceholder:
			// Sibling summary row, e.g. "  and 3 siblings" or "  and 1 sibling".
			indent := strings.Repeat("  ", node.Depth)
			var label string
			if node.Count == 1 {
				label = fmt.Sprintf("%sand 1 sibling", indent)
			} else {
				label = fmt.Sprintf("%sand %d siblings", indent, node.Count)
			}

			// Placeholder rows have no meaningful P/ROLE/STATE/TITLE cells.
			// The TREE column carries the summary text; other cells are empty.
			tw.AddRow(label, "", "", "", "")
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("rendering tree: %w", err)
	}

	// Trailing newline to match the output style of other list commands.
	_, _ = fmt.Fprintln(w)

	return nil
}

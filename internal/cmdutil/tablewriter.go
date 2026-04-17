package cmdutil

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ansiEscapePattern matches ANSI SGR (Select Graphic Rendition) escape
// sequences — the \033[...m codes used for colors, bold, dim, and reset.
// Only SGR sequences appear in this codebase's colored output.
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes all ANSI SGR escape sequences from a string, returning
// only the visible text content. Exported for use in test assertions.
func StripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

// visibleLen returns the number of visible runes in a string after stripping
// ANSI escape sequences. Use this when column alignment must ignore invisible
// formatting bytes — e.g., when computing padding for colored table cells.
func visibleLen(s string) int {
	return utf8.RuneCountInString(StripANSI(s))
}

// TableWriter collects rows of cells and writes them as a padded, aligned
// table. Unlike text/tabwriter, it measures visible width by stripping ANSI
// escape sequences, so colored cells align correctly with uncolored headers.
//
// Usage:
//
//	tw := NewTableWriter(w, 2)
//	tw.AddRow("ID", "STATE", "TITLE")
//	tw.AddRow(id, coloredState, title)
//	_ = tw.Flush()
type TableWriter struct {
	w       io.Writer
	rows    [][]string
	padding int
}

// NewTableWriter creates a TableWriter that writes aligned output to w. The
// padding parameter controls the minimum number of spaces between columns.
func NewTableWriter(w io.Writer, padding int) *TableWriter {
	return &TableWriter{w: w, padding: padding}
}

// AddRow appends a row of cell values. Every row must have the same number of
// cells as the first row added to this writer. Flush returns an error if any
// row violates this constraint.
func (t *TableWriter) AddRow(cells ...string) {
	t.rows = append(t.rows, cells)
}

// Flush computes the maximum visible width per column across all rows, then
// writes every row with cells padded to their column width plus inter-column
// padding. The last cell in each row is written without trailing padding.
// Returns an error if any row has a different number of cells than the first
// row, or if any write to the underlying writer fails.
func (t *TableWriter) Flush() error {
	if len(t.rows) == 0 {
		return nil
	}

	// Enforce uniform column count: every row must have the same number of
	// cells as the first row. A mismatch indicates a programming error in the
	// caller rather than a runtime condition, so a descriptive error is
	// preferable to silently producing ragged output.
	cols := len(t.rows[0])
	for i, row := range t.rows[1:] {
		if len(row) != cols {
			return fmt.Errorf("tablewriter: row %d has %d cells, want %d", i+1, len(row), cols)
		}
	}

	// Compute maximum visible width per column.
	widths := make([]int, cols)
	for _, row := range t.rows {
		for i, cell := range row {
			vw := visibleLen(cell)
			if vw > widths[i] {
				widths[i] = vw
			}
		}
	}

	// Write each row with padding.
	for _, row := range t.rows {
		for i, cell := range row {
			_, err := fmt.Fprint(t.w, cell)
			if err != nil {
				return err
			}
			// Pad all columns except the last one in the row.
			if i < cols-1 {
				pad := widths[i] - visibleLen(cell) + t.padding
				_, err = fmt.Fprint(t.w, strings.Repeat(" ", pad))
				if err != nil {
					return err
				}
			}
		}
		_, err := fmt.Fprint(t.w, "\n")
		if err != nil {
			return err
		}
	}
	return nil
}

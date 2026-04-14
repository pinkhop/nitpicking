package cmdutil

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// Column represents a single column in tabular issue list output. Each column
// has a display header and a function that renders the column value for a given
// issue list item. Columns are the building blocks of the configurable column
// selection model used by list, search, ready, blocked, and epic children
// commands.
type Column struct {
	// Name is the canonical, case-insensitive identifier used in --columns
	// flag values (e.g., "ID", "PRIORITY").
	Name string

	// Header is the all-caps string rendered in the header row.
	Header string

	// Render extracts and formats the column value from an IssueListItemDTO.
	// The RenderContext provides color scheme and terminal width information
	// needed by columns like STATE and TITLE that apply formatting.
	Render func(item driving.IssueListItemDTO, rc RenderContext) string
}

// RenderContext provides environmental information that column render functions
// need to format values correctly. It carries the color scheme for state
// colorization and the maximum title width for truncation.
type RenderContext struct {
	// ColorScheme controls ANSI coloring of state columns. Nil means no color.
	ColorScheme *iostreams.ColorScheme

	// MaxTitleWidth is the maximum number of columns the title may occupy.
	// Zero means no truncation (non-TTY).
	MaxTitleWidth int
}

// columnRegistry maps canonical uppercase column names to their Column
// definitions. This is the single source of truth for all available columns.
var columnRegistry = map[string]Column{
	"ID": {
		Name:   "ID",
		Header: "ID",
		Render: func(item driving.IssueListItemDTO, _ RenderContext) string {
			return item.ID
		},
	},
	"PRIORITY": {
		Name:   "PRIORITY",
		Header: "PRIORITY",
		Render: func(item driving.IssueListItemDTO, _ RenderContext) string {
			return item.Priority.String()
		},
	},
	"PARENT_ID": {
		Name:   "PARENT_ID",
		Header: "PARENT ID",
		Render: func(item driving.IssueListItemDTO, _ RenderContext) string {
			return item.ParentID
		},
	},
	"PARENT_CREATED": {
		Name:   "PARENT_CREATED",
		Header: "PARENT CREATED",
		Render: func(item driving.IssueListItemDTO, _ RenderContext) string {
			if item.ParentCreatedAt.IsZero() {
				return ""
			}
			return item.ParentCreatedAt.Format(time.DateTime)
		},
	},
	"CREATED": {
		Name:   "CREATED",
		Header: "CREATED",
		Render: func(item driving.IssueListItemDTO, _ RenderContext) string {
			return item.CreatedAt.Format(time.DateTime)
		},
	},
	"ROLE": {
		Name:   "ROLE",
		Header: "ROLE",
		Render: func(item driving.IssueListItemDTO, _ RenderContext) string {
			return item.Role.String()
		},
	},
	"STATE": {
		Name:   "STATE",
		Header: "STATE",
		Render: func(item driving.IssueListItemDTO, rc RenderContext) string {
			cs := rc.ColorScheme
			if cs == nil {
				cs = iostreams.NewColorScheme(false)
			}
			return FormatState(cs, item.State, item.SecondaryState)
		},
	},
	"TITLE": {
		Name:   "TITLE",
		Header: "TITLE",
		Render: func(item driving.IssueListItemDTO, rc RenderContext) string {
			title := item.Title
			if len(item.BlockerIDs) > 0 {
				title += " " + FormatBlockerSuffix(item.BlockerIDs)
			}
			return TruncateTitle(title, rc.MaxTitleWidth)
		},
	},
}

// columnOrder defines the iteration order for the column registry, ensuring
// ValidColumnNames returns a deterministic, readable list.
var columnOrder = []string{
	"ID", "PRIORITY", "PARENT_ID", "PARENT_CREATED",
	"CREATED", "ROLE", "STATE", "TITLE",
}

// DefaultColumns is the default column set and order for tabular issue list
// output. It matches the sort keys of OrderByPriority so the output is
// self-explaining.
var DefaultColumns = []Column{
	columnRegistry["ID"],
	columnRegistry["PRIORITY"],
	columnRegistry["PARENT_ID"],
	columnRegistry["PARENT_CREATED"],
	columnRegistry["CREATED"],
	columnRegistry["ROLE"],
	columnRegistry["STATE"],
	columnRegistry["TITLE"],
}

// ValidColumnNames returns a deterministic, comma-separated list of valid
// column names for use in error messages.
func ValidColumnNames() string {
	return strings.Join(columnOrder, ", ")
}

// ParseColumns parses a comma-separated string of column names and returns the
// corresponding Column slice in the order specified. Column names are
// case-insensitive. Returns an error listing valid column names when any name
// is unrecognized. An empty input string returns the DefaultColumns.
func ParseColumns(input string) ([]Column, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return DefaultColumns, nil
	}

	parts := strings.Split(trimmed, ",")
	columns := make([]Column, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		upper := strings.ToUpper(name)
		col, ok := columnRegistry[upper]
		if !ok {
			return nil, fmt.Errorf("unknown column %q; valid columns: %s", name, ValidColumnNames())
		}
		columns = append(columns, col)
	}
	return columns, nil
}

// ColumnsWithTimestamp returns a copy of the given columns with the CREATED
// column added if it is not already present. This supports the --timestamps
// flag as syntactic sugar for including CREATED in the column set.
func ColumnsWithTimestamp(cols []Column) []Column {
	for _, c := range cols {
		if c.Name == "CREATED" {
			return cols
		}
	}

	// Insert CREATED before TITLE (the last column by convention) when
	// possible, otherwise append at the end.
	result := make([]Column, 0, len(cols)+1)
	inserted := false
	for _, c := range cols {
		if c.Name == "TITLE" && !inserted {
			result = append(result, columnRegistry["CREATED"])
			inserted = true
		}
		result = append(result, c)
	}
	if !inserted {
		result = append(result, columnRegistry["CREATED"])
	}
	return result
}

// WriteColumnarHeader writes an all-caps header row for the given columns to a
// tabwriter. It provides the newer column-based header API, generalizing the
// older WriteListHeader approach that only supported a boolean timestamp
// toggle. Callers will migrate in follow-up tasks.
func WriteColumnarHeader(w io.Writer, cols []Column) {
	headers := make([]string, 0, len(cols))
	for _, c := range cols {
		headers = append(headers, c.Header)
	}
	_, _ = fmt.Fprintf(w, "%s\n", strings.Join(headers, "\t"))
}

// WriteColumnarRow writes a single data row for the given columns to a
// tabwriter. The render context provides color and truncation information.
func WriteColumnarRow(w io.Writer, item driving.IssueListItemDTO, cols []Column, rc RenderContext) {
	values := make([]string, 0, len(cols))
	for _, c := range cols {
		values = append(values, c.Render(item, rc))
	}
	_, _ = fmt.Fprintf(w, "%s\n", strings.Join(values, "\t"))
}

// ColumnByName looks up a column by its canonical name (case-insensitive).
// Returns the Column and true if found, or a zero Column and false if the name
// is not recognized.
func ColumnByName(name string) (Column, bool) {
	col, ok := columnRegistry[strings.ToUpper(strings.TrimSpace(name))]
	return col, ok
}

// ColumnNames returns the canonical names of the given columns in order.
func ColumnNames(cols []Column) []string {
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Name)
	}
	return names
}

// columnsContain reports whether the given columns include one with the
// specified canonical name.
func columnsContain(cols []Column, name string) bool {
	for _, c := range cols {
		if c.Name == name {
			return true
		}
	}
	return false
}

// OverheadForColumns estimates the non-title character overhead for title
// truncation based on the selected columns. Each column contributes its
// typical display width plus inter-column padding. The TITLE column itself
// is excluded since it is the one being truncated.
func OverheadForColumns(cols []Column) int {
	overhead := 0
	for _, c := range cols {
		if c.Name == "TITLE" {
			continue
		}
		// Approximate widths for each column type, including 2-char tab padding.
		switch c.Name {
		case "ID":
			overhead += 10 // e.g., "NP-a3bxr" + padding
		case "PRIORITY":
			overhead += 4 // "P2" + padding
		case "PARENT_ID":
			overhead += 10 // "NP-a3bxr" or empty + padding
		case "PARENT_CREATED":
			overhead += 21 // "2006-01-02 15:04:05" + padding
		case "CREATED":
			overhead += 21 // "2006-01-02 15:04:05" + padding
		case "ROLE":
			overhead += 6 // "task" or "epic" + padding
		case "STATE":
			overhead += 9 // "open" or "closed" + padding
		default:
			overhead += 10 // fallback
		}
	}
	return overhead
}

// HasColumn reports whether the given column set includes a column with the
// specified canonical name. This is a convenience wrapper around
// columnsContain for use by command packages.
func HasColumn(cols []Column, name string) bool {
	return columnsContain(cols, name)
}

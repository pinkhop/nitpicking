package cmdutil_test

import (
	"bytes"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

func TestTableWriter_PlainTextAlignment(t *testing.T) {
	t.Parallel()

	// Given: a table writer with 2-space padding and plain text rows.
	var buf bytes.Buffer
	tw := cmdutil.NewTableWriter(&buf, 2)
	tw.AddRow("ID", "STATE", "TITLE")
	tw.AddRow("NP-abc", "closed", "Short title")
	tw.AddRow("NP-longid", "open", "Another title")

	// When: the table is flushed.
	err := tw.Flush()
	// Then: columns are aligned by the widest cell in each column.
	if err != nil {
		t.Fatalf("Flush() returned error: %v", err)
	}

	expected := "" +
		"ID         STATE   TITLE\n" +
		"NP-abc     closed  Short title\n" +
		"NP-longid  open    Another title\n"

	if buf.String() != expected {
		t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), expected)
	}
}

func TestTableWriter_ANSIColoredCellsAlignWithPlainHeaders(t *testing.T) {
	t.Parallel()

	// Given: a table with a plain header row and data rows containing ANSI
	// color codes in the STATE column.
	var buf bytes.Buffer
	tw := cmdutil.NewTableWriter(&buf, 2)
	tw.AddRow("ID", "STATE", "TITLE")

	// Simulate Color256(246, "closed") + Color256(0, "")
	coloredClosed := "\033[38;5;246mclosed\033[0m\033[38;5;000m\033[0m"
	tw.AddRow("NP-abc", coloredClosed, "First issue")

	// Simulate Color256(71, "open") + " (" + Color256(71, "ready") + ")"
	coloredOpenReady := "\033[38;5;071mopen\033[0m (\033[38;5;071mready\033[0m)"
	tw.AddRow("NP-def", coloredOpenReady, "Second issue")

	// When: the table is flushed.
	err := tw.Flush()
	// Then: columns align based on visible width, not byte length.
	if err != nil {
		t.Fatalf("Flush() returned error: %v", err)
	}

	got := buf.String()

	// The STATE column's visible widths are: "STATE"=5, "closed"=6,
	// "open (ready)"=12. So "open (ready)" is the widest at 12 visible chars.
	// Verify the header line aligns: "STATE" should be padded to 12 + 2 = 14
	// visible chars from the start of the STATE column.
	//
	// Instead of matching exact ANSI bytes, verify alignment by checking that
	// the TITLE column starts at the same visible position in every line.
	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %q", len(lines), got)
	}

	// Find visible position of "TITLE" / "First issue" / "Second issue"
	// by stripping ANSI and finding the offset.
	positions := make([]int, 3)
	targets := []string{"TITLE", "First issue", "Second issue"}
	for i, line := range lines[:3] {
		stripped := stripANSI(string(line))
		pos := bytes.Index([]byte(stripped), []byte(targets[i]))
		if pos < 0 {
			t.Fatalf("line %d: could not find %q in stripped line %q", i, targets[i], stripped)
		}
		positions[i] = pos
	}

	if positions[0] != positions[1] || positions[1] != positions[2] {
		t.Errorf("TITLE column not aligned: header at %d, row1 at %d, row2 at %d",
			positions[0], positions[1], positions[2])
	}
}

func TestTableWriter_EmptyTable(t *testing.T) {
	t.Parallel()

	// Given: a table writer with no rows.
	var buf bytes.Buffer
	tw := cmdutil.NewTableWriter(&buf, 2)

	// When: the table is flushed.
	err := tw.Flush()
	// Then: nothing is written and no error occurs.
	if err != nil {
		t.Fatalf("Flush() returned error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestTableWriter_SingleColumn(t *testing.T) {
	t.Parallel()

	// Given: a table with a single column (no inter-column padding needed).
	var buf bytes.Buffer
	tw := cmdutil.NewTableWriter(&buf, 2)
	tw.AddRow("HEADER")
	tw.AddRow("value")

	// When: the table is flushed.
	err := tw.Flush()
	// Then: each value is on its own line with no trailing padding.
	if err != nil {
		t.Fatalf("Flush() returned error: %v", err)
	}
	expected := "HEADER\nvalue\n"
	if buf.String() != expected {
		t.Errorf("got %q, want %q", buf.String(), expected)
	}
}

func TestTableWriter_MultipleANSICodesPerRow(t *testing.T) {
	t.Parallel()

	// Given: rows where every cell has ANSI codes (like ready/blocked commands).
	var buf bytes.Buffer
	tw := cmdutil.NewTableWriter(&buf, 2)
	tw.AddRow("ID", "ROLE", "STATE", "PRI")

	boldID := "\033[1mNP-abc\033[0m"
	dimRole := "\033[2mtask\033[0m"
	colorState := "\033[38;5;071mopen\033[0m (\033[38;5;071mready\033[0m)"
	yellowPri := "\033[33mP4\033[0m"
	tw.AddRow(boldID, dimRole, colorState, yellowPri)

	// When: the table is flushed.
	err := tw.Flush()
	// Then: all columns align correctly despite ANSI in every cell.
	if err != nil {
		t.Fatalf("Flush() returned error: %v", err)
	}

	got := buf.String()
	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got: %q", got)
	}

	// Verify that the PRI column starts at the same visible position.
	headerStripped := stripANSI(string(lines[0]))
	dataStripped := stripANSI(string(lines[1]))

	headerPos := bytes.Index([]byte(headerStripped), []byte("PRI"))
	dataPos := bytes.Index([]byte(dataStripped), []byte("P4"))

	if headerPos != dataPos {
		t.Errorf("PRI column misaligned: header at visible pos %d, data at %d\nheader: %q\ndata:   %q",
			headerPos, dataPos, headerStripped, dataStripped)
	}
}

func TestTableWriter_RaggedRow_FlushReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a table writer whose second row has fewer cells than the first.
	var buf bytes.Buffer
	tw := cmdutil.NewTableWriter(&buf, 2)
	tw.AddRow("ID", "STATE", "TITLE")
	tw.AddRow("NP-abc", "open") // missing TITLE cell

	// When: the table is flushed.
	err := tw.Flush()

	// Then: Flush returns a non-nil error describing the mismatch.
	if err == nil {
		t.Fatal("Flush() returned nil; want a non-nil error for a ragged row")
	}
}

func TestTableWriter_RaggedRow_TooManyColumns_FlushReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a table writer whose second row has more cells than the first.
	var buf bytes.Buffer
	tw := cmdutil.NewTableWriter(&buf, 2)
	tw.AddRow("ID", "STATE")
	tw.AddRow("NP-abc", "open", "extra cell") // extra cell beyond column count

	// When: the table is flushed.
	err := tw.Flush()

	// Then: Flush returns a non-nil error describing the mismatch.
	if err == nil {
		t.Fatal("Flush() returned nil; want a non-nil error for a ragged row")
	}
}

// stripANSI removes ANSI escape sequences from a string for test assertions.
func stripANSI(s string) string {
	return cmdutil.StripANSI(s)
}

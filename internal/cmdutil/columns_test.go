package cmdutil_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- DefaultColumns ---

func TestDefaultColumns_ContainsAllExpectedColumns(t *testing.T) {
	t.Parallel()

	// Given — the expected default column names in order.
	want := []string{"ID", "PRIORITY", "PARENT_ID", "PARENT_CREATED", "CREATED", "ROLE", "STATE", "TITLE"}

	// When
	got := cmdutil.ColumnNames(cmdutil.DefaultColumns)

	// Then
	if len(got) != len(want) {
		t.Fatalf("DefaultColumns has %d columns, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("DefaultColumns[%d] = %q, want %q", i, got[i], name)
		}
	}
}

// --- ParseColumns ---

func TestParseColumns_EmptyInput_ReturnsDefaultColumns(t *testing.T) {
	t.Parallel()

	// Given
	input := ""

	// When
	cols, err := cmdutil.ParseColumns(input)
	// Then
	if err != nil {
		t.Fatalf("ParseColumns(%q) returned error: %v", input, err)
	}
	got := cmdutil.ColumnNames(cols)
	want := cmdutil.ColumnNames(cmdutil.DefaultColumns)
	if len(got) != len(want) {
		t.Fatalf("expected %d columns, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseColumns_CustomSelection_ReturnsInSpecifiedOrder(t *testing.T) {
	t.Parallel()

	// Given
	input := "TITLE,ID,PRIORITY"

	// When
	cols, err := cmdutil.ParseColumns(input)
	// Then
	if err != nil {
		t.Fatalf("ParseColumns(%q) returned error: %v", input, err)
	}
	got := cmdutil.ColumnNames(cols)
	want := []string{"TITLE", "ID", "PRIORITY"}
	if len(got) != len(want) {
		t.Fatalf("expected %d columns, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseColumns_CaseInsensitive_ParsesCorrectly(t *testing.T) {
	t.Parallel()

	// Given — mixed case input.
	input := "id,Priority,role"

	// When
	cols, err := cmdutil.ParseColumns(input)
	// Then
	if err != nil {
		t.Fatalf("ParseColumns(%q) returned error: %v", input, err)
	}
	got := cmdutil.ColumnNames(cols)
	want := []string{"ID", "PRIORITY", "ROLE"}
	if len(got) != len(want) {
		t.Fatalf("expected %d columns, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseColumns_InvalidColumnName_ReturnsErrorWithValidNames(t *testing.T) {
	t.Parallel()

	// Given
	input := "ID,BOGUS,TITLE"

	// When
	_, err := cmdutil.ParseColumns(input)

	// Then
	if err == nil {
		t.Fatal("expected error for invalid column name, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "BOGUS") {
		t.Errorf("error should mention invalid name %q, got: %s", "BOGUS", errMsg)
	}
	if !strings.Contains(errMsg, "valid columns") {
		t.Errorf("error should list valid columns, got: %s", errMsg)
	}
}

func TestParseColumns_WhitespaceAroundNames_TrimsCorrectly(t *testing.T) {
	t.Parallel()

	// Given — whitespace around names.
	input := " ID , TITLE "

	// When
	cols, err := cmdutil.ParseColumns(input)
	// Then
	if err != nil {
		t.Fatalf("ParseColumns(%q) returned error: %v", input, err)
	}
	got := cmdutil.ColumnNames(cols)
	want := []string{"ID", "TITLE"}
	if len(got) != len(want) {
		t.Fatalf("expected %d columns, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- WriteColumnarHeader ---

func TestWriteColumnarHeader_WritesTabSeparatedHeaders(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("ID,PRIORITY,TITLE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarHeader(&buf, cols)

	// Then
	got := buf.String()
	want := "ID\tPRIORITY\tTITLE\n"
	if got != want {
		t.Errorf("WriteColumnarHeader: got %q, want %q", got, want)
	}
}

func TestWriteColumnarHeader_DefaultColumns_AllUpperCase(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarHeader(&buf, cmdutil.DefaultColumns)

	// Then
	header := strings.TrimRight(buf.String(), "\n")
	for _, col := range strings.Split(header, "\t") {
		if col != strings.ToUpper(col) {
			t.Errorf("column header %q is not all-caps", col)
		}
	}
}

// --- WriteColumnarRow ---

func TestWriteColumnarRow_RendersSelectedColumns(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("ID,ROLE,PRIORITY")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	item := driving.IssueListItemDTO{
		ID:       "NP-abc12",
		Role:     domain.RoleTask,
		Priority: domain.P1,
	}
	rc := cmdutil.RenderContext{
		ColorScheme: iostreams.NewColorScheme(false),
	}
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarRow(&buf, item, cols, rc)

	// Then
	got := buf.String()
	want := "NP-abc12\ttask\tP1\n"
	if got != want {
		t.Errorf("WriteColumnarRow: got %q, want %q", got, want)
	}
}

func TestWriteColumnarRow_TitleColumnTruncates(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("TITLE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	item := driving.IssueListItemDTO{
		Title: "A very long title that should be truncated",
	}
	rc := cmdutil.RenderContext{
		ColorScheme:   iostreams.NewColorScheme(false),
		MaxTitleWidth: 10,
	}
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarRow(&buf, item, cols, rc)

	// Then — title truncated to 10 chars (9 + ellipsis).
	got := strings.TrimRight(buf.String(), "\n")
	if len([]rune(got)) > 10 {
		t.Errorf("expected title truncated to 10 runes, got %d runes: %q", len([]rune(got)), got)
	}
}

func TestWriteColumnarRow_TitleColumnIncludesBlockerSuffix(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("TITLE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	item := driving.IssueListItemDTO{
		Title:      "Task A",
		BlockerIDs: []string{"NP-blk01"},
	}
	rc := cmdutil.RenderContext{
		ColorScheme: iostreams.NewColorScheme(false),
	}
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarRow(&buf, item, cols, rc)

	// Then
	got := strings.TrimRight(buf.String(), "\n")
	if !strings.Contains(got, "NP-blk01") {
		t.Errorf("expected blocker suffix in output, got %q", got)
	}
}

func TestWriteColumnarRow_ParentCreatedColumn_FormatsTimestamp(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("PARENT_CREATED")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	parentTime := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	item := driving.IssueListItemDTO{
		ParentCreatedAt: parentTime,
	}
	rc := cmdutil.RenderContext{
		ColorScheme: iostreams.NewColorScheme(false),
	}
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarRow(&buf, item, cols, rc)

	// Then
	got := strings.TrimRight(buf.String(), "\n")
	want := "2026-03-15 10:30:00"
	if got != want {
		t.Errorf("PARENT_CREATED render: got %q, want %q", got, want)
	}
}

func TestWriteColumnarRow_ParentCreatedColumn_EmptyWhenNoParent(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("PARENT_CREATED")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	item := driving.IssueListItemDTO{} // zero ParentCreatedAt
	rc := cmdutil.RenderContext{
		ColorScheme: iostreams.NewColorScheme(false),
	}
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarRow(&buf, item, cols, rc)

	// Then
	got := strings.TrimRight(buf.String(), "\n")
	if got != "" {
		t.Errorf("PARENT_CREATED render: got %q, want empty string", got)
	}
}

func TestWriteColumnarRow_ParentIDColumn_RendersParentID(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("PARENT_ID")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	item := driving.IssueListItemDTO{
		ParentID: "NP-par01",
	}
	rc := cmdutil.RenderContext{
		ColorScheme: iostreams.NewColorScheme(false),
	}
	var buf bytes.Buffer

	// When
	cmdutil.WriteColumnarRow(&buf, item, cols, rc)

	// Then
	got := strings.TrimRight(buf.String(), "\n")
	if got != "NP-par01" {
		t.Errorf("PARENT_ID render: got %q, want %q", got, "NP-par01")
	}
}

// --- ColumnsWithTimestamp ---

func TestColumnsWithTimestamp_AddsCreatedBeforeTitle(t *testing.T) {
	t.Parallel()

	// Given — a column set without CREATED.
	cols, err := cmdutil.ParseColumns("ID,PRIORITY,TITLE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}

	// When
	result := cmdutil.ColumnsWithTimestamp(cols)

	// Then — CREATED should appear before TITLE.
	names := cmdutil.ColumnNames(result)
	if len(names) != 4 {
		t.Fatalf("expected 4 columns, got %d: %v", len(names), names)
	}
	if names[2] != "CREATED" {
		t.Errorf("expected CREATED at index 2, got %q", names[2])
	}
	if names[3] != "TITLE" {
		t.Errorf("expected TITLE at index 3, got %q", names[3])
	}
}

func TestColumnsWithTimestamp_AlreadyHasCreated_NoChange(t *testing.T) {
	t.Parallel()

	// Given — a column set that already includes CREATED.
	cols, err := cmdutil.ParseColumns("ID,CREATED,TITLE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}

	// When
	result := cmdutil.ColumnsWithTimestamp(cols)

	// Then — no duplicate CREATED column.
	names := cmdutil.ColumnNames(result)
	count := 0
	for _, n := range names {
		if n == "CREATED" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 CREATED column, got %d in %v", count, names)
	}
}

func TestColumnsWithTimestamp_NoTitle_AppendsCreated(t *testing.T) {
	t.Parallel()

	// Given — a column set without TITLE.
	cols, err := cmdutil.ParseColumns("ID,PRIORITY")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}

	// When
	result := cmdutil.ColumnsWithTimestamp(cols)

	// Then — CREATED appended at the end.
	names := cmdutil.ColumnNames(result)
	if len(names) != 3 {
		t.Fatalf("expected 3 columns, got %d: %v", len(names), names)
	}
	if names[2] != "CREATED" {
		t.Errorf("expected CREATED at index 2, got %q", names[2])
	}
}

// --- ValidColumnNames ---

func TestValidColumnNames_ContainsAllColumnNames(t *testing.T) {
	t.Parallel()

	// Given — the expected column names.
	expected := []string{"ID", "PRIORITY", "PARENT_ID", "PARENT_CREATED", "CREATED", "ROLE", "STATE", "TITLE"}

	// When
	got := cmdutil.ValidColumnNames()

	// Then — all names should appear in the output.
	for _, name := range expected {
		if !strings.Contains(got, name) {
			t.Errorf("ValidColumnNames() missing %q, got: %s", name, got)
		}
	}
}

// --- ColumnByName ---

func TestColumnByName_ValidName_ReturnsColumn(t *testing.T) {
	t.Parallel()

	// Given
	name := "priority"

	// When
	col, ok := cmdutil.ColumnByName(name)

	// Then
	if !ok {
		t.Fatalf("ColumnByName(%q) returned ok=false", name)
	}
	if col.Name != "PRIORITY" {
		t.Errorf("ColumnByName(%q).Name = %q, want %q", name, col.Name, "PRIORITY")
	}
}

func TestColumnByName_InvalidName_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given
	name := "BOGUS"

	// When
	_, ok := cmdutil.ColumnByName(name)

	// Then
	if ok {
		t.Errorf("ColumnByName(%q) returned ok=true, want false", name)
	}
}

// --- OverheadForColumns ---

func TestOverheadForColumns_ExcludesTitleColumn(t *testing.T) {
	t.Parallel()

	// Given — columns including TITLE.
	colsWithTitle, err := cmdutil.ParseColumns("ID,TITLE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}
	colsWithoutTitle, err := cmdutil.ParseColumns("ID")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}

	// When
	overheadWith := cmdutil.OverheadForColumns(colsWithTitle)
	overheadWithout := cmdutil.OverheadForColumns(colsWithoutTitle)

	// Then — TITLE should not add to the overhead.
	if overheadWith != overheadWithout {
		t.Errorf("TITLE should not add overhead: with=%d, without=%d", overheadWith, overheadWithout)
	}
}

func TestOverheadForColumns_ReturnsPositiveForNonTitleColumns(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("ID,PRIORITY,ROLE,STATE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}

	// When
	overhead := cmdutil.OverheadForColumns(cols)

	// Then
	if overhead <= 0 {
		t.Errorf("expected positive overhead, got %d", overhead)
	}
}

// --- HasColumn ---

func TestHasColumn_PresentColumn_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given
	cols := cmdutil.DefaultColumns

	// When
	got := cmdutil.HasColumn(cols, "ID")

	// Then
	if !got {
		t.Error("HasColumn(DefaultColumns, \"ID\") = false, want true")
	}
}

func TestHasColumn_AbsentColumn_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given
	cols, err := cmdutil.ParseColumns("ID,TITLE")
	if err != nil {
		t.Fatalf("precondition failed: %v", err)
	}

	// When
	got := cmdutil.HasColumn(cols, "PRIORITY")

	// Then
	if got {
		t.Error("HasColumn should return false for absent column")
	}
}

package cmdutil_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// --- DefaultColumns ---

func TestDefaultColumns_ContainsAllExpectedColumns(t *testing.T) {
	t.Parallel()

	// Given — the expected default column names in order.
	want := []string{"ID", "PRIORITY", "ROLE", "STATE", "TITLE"}

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

	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "all lowercase",
			input: "id,priority,title",
			want:  []string{"ID", "PRIORITY", "TITLE"},
		},
		{
			name:  "mixed case",
			input: "Id,pRiOrItY,Role",
			want:  []string{"ID", "PRIORITY", "ROLE"},
		},
		{
			name:  "all uppercase",
			input: "STATE,CREATED,PARENT_ID",
			want:  []string{"STATE", "CREATED", "PARENT_ID"},
		},
		{
			name:  "underscore column lowercase",
			input: "parent_id,parent_created",
			want:  []string{"PARENT_ID", "PARENT_CREATED"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given — input in various case forms.
			// When
			cols, err := cmdutil.ParseColumns(tc.input)
			// Then
			if err != nil {
				t.Fatalf("ParseColumns(%q) returned error: %v", tc.input, err)
			}
			got := cmdutil.ColumnNames(cols)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d columns, got %d: %v", len(tc.want), len(got), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("column[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseColumns_DuplicateColumnName_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	input := "ID,TITLE,ID"

	// When
	_, err := cmdutil.ParseColumns(input)

	// Then
	if err == nil {
		t.Fatal("expected error for duplicate column name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention 'duplicate', got: %s", err.Error())
	}
}

func TestParseColumns_DuplicateColumnDifferentCase_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — "id" and "ID" refer to the same column.
	input := "id,TITLE,ID"

	// When
	_, err := cmdutil.ParseColumns(input)

	// Then
	if err == nil {
		t.Fatal("expected error for case-insensitive duplicate column name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention 'duplicate', got: %s", err.Error())
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

func TestParseColumns_InvalidColumnLowercase_PreservesOriginalCaseInError(t *testing.T) {
	t.Parallel()

	// Given — a lowercase unknown column name.
	input := "id,bogus,title"

	// When
	_, err := cmdutil.ParseColumns(input)

	// Then — the error message should preserve the user's original casing.
	if err == nil {
		t.Fatal("expected error for invalid column name, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "bogus") {
		t.Errorf("error should preserve original casing %q, got: %s", "bogus", errMsg)
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

// --- ValidColumnNames ---

func TestValidColumnNames_ContainsAllColumnNames(t *testing.T) {
	t.Parallel()

	// Given — the expected column names.
	expected := []string{"ID", "CREATED", "PARENT_ID", "PARENT_CREATED", "PRIORITY", "ROLE", "STATE", "TITLE"}

	// When
	got := cmdutil.ValidColumnNames()

	// Then — all names should appear in the output.
	for _, name := range expected {
		if !strings.Contains(got, name) {
			t.Errorf("ValidColumnNames() missing %q, got: %s", name, got)
		}
	}
}

func TestValidColumnNames_CanonicalOrder(t *testing.T) {
	t.Parallel()

	// Given — the canonical order expected in help text.
	want := "ID, CREATED, PARENT_ID, PARENT_CREATED, PRIORITY, ROLE, STATE, TITLE"

	// When
	got := cmdutil.ValidColumnNames()

	// Then — the exact order must match.
	if got != want {
		t.Errorf("ValidColumnNames() = %q, want %q", got, want)
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

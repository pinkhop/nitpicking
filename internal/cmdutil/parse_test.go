package cmdutil_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- ParseOrderBy ---

func TestParseOrderBy_AllColumnValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  driving.OrderBy
	}{
		{name: "empty defaults to priority", input: "", want: driving.OrderByPriority},
		{name: "priority", input: "priority", want: driving.OrderByPriority},
		{name: "PRIORITY uppercase", input: "PRIORITY", want: driving.OrderByPriority},
		{name: "id", input: "id", want: driving.OrderByID},
		{name: "ID uppercase", input: "ID", want: driving.OrderByID},
		{name: "created", input: "created", want: driving.OrderByCreatedAt},
		{name: "CREATED uppercase", input: "CREATED", want: driving.OrderByCreatedAt},
		{name: "modified alias", input: "modified", want: driving.OrderByUpdatedAt},
		{name: "MODIFIED uppercase", input: "MODIFIED", want: driving.OrderByUpdatedAt},
		{name: "role", input: "role", want: driving.OrderByRole},
		{name: "ROLE uppercase", input: "ROLE", want: driving.OrderByRole},
		{name: "state", input: "state", want: driving.OrderByState},
		{name: "STATE uppercase", input: "STATE", want: driving.OrderByState},
		{name: "title", input: "title", want: driving.OrderByTitle},
		{name: "TITLE uppercase", input: "TITLE", want: driving.OrderByTitle},
		{name: "parent_id", input: "parent_id", want: driving.OrderByParentID},
		{name: "PARENT_ID uppercase", input: "PARENT_ID", want: driving.OrderByParentID},
		{name: "parent_created", input: "parent_created", want: driving.OrderByParentCreated},
		{name: "PARENT_CREATED uppercase", input: "PARENT_CREATED", want: driving.OrderByParentCreated},
		{name: "mixed case Priority", input: "Priority", want: driving.OrderByPriority},
		{name: "leading and trailing spaces", input: "  id  ", want: driving.OrderByID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got, dir, err := cmdutil.ParseOrderBy(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseOrderBy(%q) = %v, want %v", tc.input, got, tc.want)
			}
			if dir != driving.SortAscending {
				t.Errorf("ParseOrderBy(%q) direction = %v, want SortAscending", tc.input, dir)
			}
		})
	}
}

func TestParseOrderBy_AscSuffix_ReturnsAscending(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  driving.OrderBy
	}{
		{name: "priority:asc lowercase", input: "priority:asc", want: driving.OrderByPriority},
		{name: "PRIORITY:ASC uppercase", input: "PRIORITY:ASC", want: driving.OrderByPriority},
		{name: "id:asc", input: "id:asc", want: driving.OrderByID},
		{name: "created:Asc mixed case", input: "created:Asc", want: driving.OrderByCreatedAt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got, dir, err := cmdutil.ParseOrderBy(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseOrderBy(%q) orderBy = %v, want %v", tc.input, got, tc.want)
			}
			if dir != driving.SortAscending {
				t.Errorf("ParseOrderBy(%q) direction = %v, want SortAscending", tc.input, dir)
			}
		})
	}
}

func TestParseOrderBy_DescSuffix_ReturnsDescending(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  driving.OrderBy
	}{
		{name: "priority:desc lowercase", input: "priority:desc", want: driving.OrderByPriority},
		{name: "PRIORITY:DESC uppercase", input: "PRIORITY:DESC", want: driving.OrderByPriority},
		{name: "id:desc", input: "id:desc", want: driving.OrderByID},
		{name: "created:Desc mixed case", input: "created:Desc", want: driving.OrderByCreatedAt},
		{name: "title:desc", input: "title:desc", want: driving.OrderByTitle},
		{name: "role:desc", input: "role:desc", want: driving.OrderByRole},
		{name: "state:desc", input: "state:desc", want: driving.OrderByState},
		{name: "modified:desc", input: "modified:desc", want: driving.OrderByUpdatedAt},
		{name: "parent_id:desc", input: "parent_id:desc", want: driving.OrderByParentID},
		{name: "parent_created:desc", input: "parent_created:desc", want: driving.OrderByParentCreated},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got, dir, err := cmdutil.ParseOrderBy(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseOrderBy(%q) orderBy = %v, want %v", tc.input, got, tc.want)
			}
			if dir != driving.SortDescending {
				t.Errorf("ParseOrderBy(%q) direction = %v, want SortDescending", tc.input, dir)
			}
		})
	}
}

func TestParseOrderBy_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	_, _, err := cmdutil.ParseOrderBy("invalid")
	if err == nil {
		t.Error("expected error for invalid sort order")
	}
}

func TestParseOrderBy_InvalidValueWithSuffix_ReturnsError(t *testing.T) {
	t.Parallel()

	_, _, err := cmdutil.ParseOrderBy("invalid:desc")
	if err == nil {
		t.Error("expected error for invalid sort order with :desc suffix")
	}
}

func TestParseOrderBy_ErrorMessage_ListsValidValues(t *testing.T) {
	t.Parallel()

	// When
	_, _, err := cmdutil.ParseOrderBy("bogus")

	// Then
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, name := range []string{"ID", "CREATED", "PRIORITY", "ROLE", "STATE", "TITLE", "PARENT_ID", "PARENT_CREATED", "MODIFIED"} {
		if !strings.Contains(msg, name) {
			t.Errorf("error message %q does not mention valid value %q", msg, name)
		}
	}
}

func TestParseOrderBy_ErrorMessage_MentionsDirectionSuffixes(t *testing.T) {
	t.Parallel()

	// When
	_, _, err := cmdutil.ParseOrderBy("bogus")

	// Then
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, ":asc") || !strings.Contains(msg, ":desc") {
		t.Errorf("error message %q should mention :asc and :desc suffixes", msg)
	}
}

func TestValidOrderNames_ContainsAllColumnNamesAndModified(t *testing.T) {
	t.Parallel()

	// Given — the canonical column names plus MODIFIED
	expected := []string{"ID", "CREATED", "PARENT_ID", "PARENT_CREATED", "PRIORITY", "ROLE", "STATE", "TITLE", "MODIFIED"}

	// When
	names := cmdutil.ValidOrderNames()

	// Then
	for _, name := range expected {
		if !strings.Contains(names, name) {
			t.Errorf("ValidOrderNames() = %q, missing %q", names, name)
		}
	}
}

// --- ResolveFlatListOrderBy ---

func TestResolveFlatListOrderBy_PriorityVariants_MapToPriorityCreated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantDir driving.SortDirection
	}{
		{name: "empty defaults to priority ascending", input: "", wantDir: driving.SortAscending},
		{name: "priority bare", input: "priority", wantDir: driving.SortAscending},
		{name: "PRIORITY uppercase", input: "PRIORITY", wantDir: driving.SortAscending},
		{name: "mixed case Priority", input: "Priority", wantDir: driving.SortAscending},
		{name: "priority with leading/trailing spaces", input: "  priority  ", wantDir: driving.SortAscending},
		{name: "priority:asc explicit ascending", input: "priority:asc", wantDir: driving.SortAscending},
		{name: "PRIORITY:ASC uppercase", input: "PRIORITY:ASC", wantDir: driving.SortAscending},
		{name: "priority:desc descending", input: "priority:desc", wantDir: driving.SortDescending},
		{name: "PRIORITY:DESC uppercase", input: "PRIORITY:DESC", wantDir: driving.SortDescending},
		{name: "mixed case Priority:Desc", input: "Priority:Desc", wantDir: driving.SortDescending},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got, dir, err := cmdutil.ResolveFlatListOrderBy(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != driving.OrderByPriorityCreated {
				t.Errorf("ResolveFlatListOrderBy(%q) orderBy = %v, want OrderByPriorityCreated", tc.input, got)
			}
			if dir != tc.wantDir {
				t.Errorf("ResolveFlatListOrderBy(%q) direction = %v, want %v", tc.input, dir, tc.wantDir)
			}
		})
	}
}

func TestResolveFlatListOrderBy_NonPriorityKeys_PassThrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantOrder driving.OrderBy
		wantDir   driving.SortDirection
	}{
		{name: "id ascending", input: "id", wantOrder: driving.OrderByID, wantDir: driving.SortAscending},
		{name: "created descending", input: "created:desc", wantOrder: driving.OrderByCreatedAt, wantDir: driving.SortDescending},
		{name: "modified alias", input: "modified", wantOrder: driving.OrderByUpdatedAt, wantDir: driving.SortAscending},
		{name: "title descending", input: "title:desc", wantOrder: driving.OrderByTitle, wantDir: driving.SortDescending},
		{name: "parent_id", input: "parent_id", wantOrder: driving.OrderByParentID, wantDir: driving.SortAscending},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got, dir, err := cmdutil.ResolveFlatListOrderBy(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantOrder {
				t.Errorf("ResolveFlatListOrderBy(%q) orderBy = %v, want %v", tc.input, got, tc.wantOrder)
			}
			if dir != tc.wantDir {
				t.Errorf("ResolveFlatListOrderBy(%q) direction = %v, want %v", tc.input, dir, tc.wantDir)
			}
		})
	}
}

func TestResolveFlatListOrderBy_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// When
	_, _, err := cmdutil.ResolveFlatListOrderBy("bogus:asc")

	// Then
	if err == nil {
		t.Error("expected error for invalid sort order")
	}
}

// --- ParseLabels ---

func TestParseLabels_ValidKeyValue_Succeeds(t *testing.T) {
	t.Parallel()

	labels, err := cmdutil.ParseLabels([]string{"kind:bug", "team:backend"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("got %d labels, want 2", len(labels))
	}
	if labels[0].Key != "kind" || labels[0].Value != "bug" {
		t.Errorf("labels[0]: got %+v", labels[0])
	}
}

func TestParseLabels_MissingColon_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := cmdutil.ParseLabels([]string{"invalid"})
	if err == nil {
		t.Error("expected error for missing colon")
	}
}

func TestParseLabels_EmptySlice_ReturnsNil(t *testing.T) {
	t.Parallel()

	labels, err := cmdutil.ParseLabels(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if labels != nil {
		t.Errorf("expected nil, got %v", labels)
	}
}

// --- ParseLabelFilters ---

func TestParseLabelFilters_ValidKeyValue_Succeeds(t *testing.T) {
	t.Parallel()

	filters, err := cmdutil.ParseLabelFilters([]string{"kind:bug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filters) != 1 {
		t.Fatalf("got %d filters, want 1", len(filters))
	}
	if filters[0].Key != "kind" || filters[0].Value != "bug" || filters[0].Negate {
		t.Errorf("got %+v", filters[0])
	}
}

func TestParseLabelFilters_Wildcard_OmitsValue(t *testing.T) {
	t.Parallel()

	filters, err := cmdutil.ParseLabelFilters([]string{"kind:*"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filters[0].Value != "" {
		t.Errorf("expected empty value for wildcard, got %q", filters[0].Value)
	}
}

func TestParseLabelFilters_Negate_SetsFlagTrue(t *testing.T) {
	t.Parallel()

	filters, err := cmdutil.ParseLabelFilters([]string{"!kind:bug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filters[0].Negate {
		t.Error("expected Negate=true")
	}
	if filters[0].Key != "kind" || filters[0].Value != "bug" {
		t.Errorf("got key=%q value=%q", filters[0].Key, filters[0].Value)
	}
}

func TestParseLabelFilters_MissingColon_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := cmdutil.ParseLabelFilters([]string{"invalid"})
	if err == nil {
		t.Error("expected error")
	}
}

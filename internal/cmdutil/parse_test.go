package cmdutil_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- ParseOrderBy ---

func TestParseOrderBy_EmptyDefault_ReturnsPriority(t *testing.T) {
	t.Parallel()

	// When
	order, err := cmdutil.ParseOrderBy("")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order != driving.OrderByPriority {
		t.Errorf("got %v, want OrderByPriority", order)
	}
}

func TestParseOrderBy_Created_ReturnsCreatedAt(t *testing.T) {
	t.Parallel()

	order, err := cmdutil.ParseOrderBy("created")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order != driving.OrderByCreatedAt {
		t.Errorf("got %v, want OrderByCreatedAt", order)
	}
}

func TestParseOrderBy_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := cmdutil.ParseOrderBy("invalid")
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

package ticket_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestNewFacet_ValidKeyValue_Succeeds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"simple", "kind", "feat"},
		{"with symbols", "my-key!", "val_1"},
		{"unicode value", "tag", "böb"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			f, err := ticket.NewFacet(tc.key, tc.value)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Key() != tc.key {
				t.Errorf("expected key %q, got %q", tc.key, f.Key())
			}
			if f.Value() != tc.value {
				t.Errorf("expected value %q, got %q", tc.value, f.Value())
			}
		})
	}
}

func TestNewFacet_InvalidKey_Fails(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"too long", strings.Repeat("a", 65)},
		{"whitespace", "my key"},
		{"no alphanumeric", "---"},
		{"non-ascii", "café"},
		{"tab", "my\tkey"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := ticket.NewFacet(tc.key, "valid")

			// Then
			if err == nil {
				t.Errorf("expected error for key %q", tc.key)
			}
		})
	}
}

func TestNewFacet_InvalidValue_Fails(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"too long", strings.Repeat("a", 257)},
		{"whitespace", "val ue"},
		{"no alphanumeric", "---"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := ticket.NewFacet("kind", tc.value)

			// Then
			if err == nil {
				t.Errorf("expected error for value %q", tc.value)
			}
		})
	}
}

func TestFacetSet_SetAndGet(t *testing.T) {
	t.Parallel()

	// Given
	f, _ := ticket.NewFacet("kind", "feat")
	fs := ticket.NewFacetSet()

	// When
	fs = fs.Set(f)

	// Then
	v, ok := fs.Get("kind")
	if !ok {
		t.Fatal("expected facet to exist")
	}
	if v != "feat" {
		t.Errorf("expected feat, got %s", v)
	}
}

func TestFacetSet_SetOverwrites(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := ticket.NewFacet("kind", "feat")
	f2, _ := ticket.NewFacet("kind", "fix")
	fs := ticket.NewFacetSet().Set(f1)

	// When
	fs = fs.Set(f2)

	// Then
	v, _ := fs.Get("kind")
	if v != "fix" {
		t.Errorf("expected overwritten value fix, got %s", v)
	}
	if fs.Len() != 1 {
		t.Errorf("expected 1 facet, got %d", fs.Len())
	}
}

func TestFacetSet_Remove(t *testing.T) {
	t.Parallel()

	// Given
	f, _ := ticket.NewFacet("kind", "feat")
	fs := ticket.NewFacetSet().Set(f)

	// When
	fs = fs.Remove("kind")

	// Then
	if fs.Len() != 0 {
		t.Error("expected empty set after remove")
	}
	_, ok := fs.Get("kind")
	if ok {
		t.Error("expected facet to not exist after remove")
	}
}

func TestFacetSet_RemoveNonexistent_NoOp(t *testing.T) {
	t.Parallel()

	// Given
	fs := ticket.NewFacetSet()

	// When
	fs2 := fs.Remove("nonexistent")

	// Then
	if fs2.Len() != 0 {
		t.Error("expected no change")
	}
}

func TestFacetSet_Immutability(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := ticket.NewFacet("kind", "feat")
	original := ticket.NewFacetSet().Set(f1)

	// When — modify the "copy"
	f2, _ := ticket.NewFacet("priority", "high")
	_ = original.Set(f2)

	// Then — original is unchanged
	if original.Len() != 1 {
		t.Errorf("expected original to have 1 facet, got %d", original.Len())
	}
}

func TestFacetSetFrom_BuildsFromSlice(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := ticket.NewFacet("kind", "feat")
	f2, _ := ticket.NewFacet("area", "backend")

	// When
	fs := ticket.FacetSetFrom([]ticket.Facet{f1, f2})

	// Then
	if fs.Len() != 2 {
		t.Errorf("expected 2 facets, got %d", fs.Len())
	}
}

func TestFacetSet_All_IteratesAllFacets(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := ticket.NewFacet("kind", "feat")
	f2, _ := ticket.NewFacet("area", "backend")
	fs := ticket.NewFacetSet().Set(f1).Set(f2)

	// When
	count := 0
	for range fs.All() {
		count++
	}

	// Then
	if count != 2 {
		t.Errorf("expected 2 iterations, got %d", count)
	}
}

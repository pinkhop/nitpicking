package issue_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestNewLabel_ValidKeyValue_Succeeds(t *testing.T) {
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
			f, err := issue.NewLabel(tc.key, tc.value)
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

func TestNewLabel_InvalidKey_Fails(t *testing.T) {
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
			_, err := issue.NewLabel(tc.key, "valid")

			// Then
			if err == nil {
				t.Errorf("expected error for key %q", tc.key)
			}
		})
	}
}

func TestNewLabel_InvalidValue_Fails(t *testing.T) {
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
			_, err := issue.NewLabel("kind", tc.value)

			// Then
			if err == nil {
				t.Errorf("expected error for value %q", tc.value)
			}
		})
	}
}

func TestLabelSet_SetAndGet(t *testing.T) {
	t.Parallel()

	// Given
	f, _ := issue.NewLabel("kind", "feat")
	fs := issue.NewLabelSet()

	// When
	fs = fs.Set(f)

	// Then
	v, ok := fs.Get("kind")
	if !ok {
		t.Fatal("expected dimension to exist")
	}
	if v != "feat" {
		t.Errorf("expected feat, got %s", v)
	}
}

func TestLabelSet_SetOverwrites(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := issue.NewLabel("kind", "feat")
	f2, _ := issue.NewLabel("kind", "fix")
	fs := issue.NewLabelSet().Set(f1)

	// When
	fs = fs.Set(f2)

	// Then
	v, _ := fs.Get("kind")
	if v != "fix" {
		t.Errorf("expected overwritten value fix, got %s", v)
	}
	if fs.Len() != 1 {
		t.Errorf("expected 1 dimension, got %d", fs.Len())
	}
}

func TestLabelSet_Remove(t *testing.T) {
	t.Parallel()

	// Given
	f, _ := issue.NewLabel("kind", "feat")
	fs := issue.NewLabelSet().Set(f)

	// When
	fs = fs.Remove("kind")

	// Then
	if fs.Len() != 0 {
		t.Error("expected empty set after remove")
	}
	_, ok := fs.Get("kind")
	if ok {
		t.Error("expected dimension to not exist after remove")
	}
}

func TestLabelSet_RemoveNonexistent_NoOp(t *testing.T) {
	t.Parallel()

	// Given
	fs := issue.NewLabelSet()

	// When
	fs2 := fs.Remove("nonexistent")

	// Then
	if fs2.Len() != 0 {
		t.Error("expected no change")
	}
}

func TestLabelSet_Immutability(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := issue.NewLabel("kind", "feat")
	original := issue.NewLabelSet().Set(f1)

	// When — modify the "copy"
	f2, _ := issue.NewLabel("priority", "high")
	_ = original.Set(f2)

	// Then — original is unchanged
	if original.Len() != 1 {
		t.Errorf("expected original to have 1 dimension, got %d", original.Len())
	}
}

func TestLabelSetFrom_BuildsFromSlice(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := issue.NewLabel("kind", "feat")
	f2, _ := issue.NewLabel("area", "backend")

	// When
	fs := issue.LabelSetFrom([]issue.Label{f1, f2})

	// Then
	if fs.Len() != 2 {
		t.Errorf("expected 2 dimensions, got %d", fs.Len())
	}
}

func TestLabelSet_All_IteratesAllDimensions(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := issue.NewLabel("kind", "feat")
	f2, _ := issue.NewLabel("area", "backend")
	fs := issue.NewLabelSet().Set(f1).Set(f2)

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

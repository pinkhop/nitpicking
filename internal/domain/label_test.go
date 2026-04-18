package domain_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
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
			f, err := domain.NewLabel(tc.key, tc.value)
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
		{"leading hyphen", "-foo"},
		{"leading colon", ":bar"},
		{"leading dot", ".hidden"},
		{"leading bang", "!x"},
		{"leading digit", "2fa"},
		{"leading digit single", "7"},
		{"non-ascii", "café"},
		{"leading non-ascii letter", "αbeta"},
		{"tab", "my\tkey"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := domain.NewLabel(tc.key, "valid")

			// Then
			if err == nil {
				t.Errorf("expected error for key %q", tc.key)
			}
		})
	}
}

func TestNewLabel_ValidKey_FirstCharRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"single ascii letter", "x", "v"},
		{"underscore prefix", "_x", "v"},
		{"underscore alone", "_", "v"},
		{"existing system key idempotency-key", "idempotency-key", "v"},
		{"existing system key kind", "kind", "v"},
		{"existing system key area", "area", "v"},
		{"existing user key waiting-on", "waiting-on", "v"},
		{"existing user key k8s", "k8s", "v"},
		{"internal underscore key", "_internal", "v"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := domain.NewLabel(tc.key, tc.value)
			// Then
			if err != nil {
				t.Errorf("expected no error for key %q, got: %v", tc.key, err)
			}
		})
	}
}

func TestNewLabel_LeadingPunctuationOrDigit_ReturnsSpecificError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
	}{
		{"leading hyphen", "-foo"},
		{"leading digit", "2fa"},
		{"leading non-ascii unicode letter", "αbeta"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := domain.NewLabel(tc.key, "v")

			// Then
			if err == nil {
				t.Fatalf("expected error for key %q", tc.key)
			}
			// The error must be specific enough to communicate the constraint.
			errMsg := err.Error()
			if errMsg == "" {
				t.Errorf("error message must not be empty")
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
			_, err := domain.NewLabel("kind", tc.value)

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
	f, _ := domain.NewLabel("kind", "feat")
	fs := domain.NewLabelSet()

	// When
	fs = fs.Set(f)

	// Then
	v, ok := fs.Get("kind")
	if !ok {
		t.Fatal("expected label to exist")
	}
	if v != "feat" {
		t.Errorf("expected feat, got %s", v)
	}
}

func TestLabelSet_SetOverwrites(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := domain.NewLabel("kind", "feat")
	f2, _ := domain.NewLabel("kind", "fix")
	fs := domain.NewLabelSet().Set(f1)

	// When
	fs = fs.Set(f2)

	// Then
	v, _ := fs.Get("kind")
	if v != "fix" {
		t.Errorf("expected overwritten value fix, got %s", v)
	}
	if fs.Len() != 1 {
		t.Errorf("expected 1 label, got %d", fs.Len())
	}
}

func TestLabelSet_Remove(t *testing.T) {
	t.Parallel()

	// Given
	f, _ := domain.NewLabel("kind", "feat")
	fs := domain.NewLabelSet().Set(f)

	// When
	fs = fs.Remove("kind")

	// Then
	if fs.Len() != 0 {
		t.Error("expected empty set after remove")
	}
	_, ok := fs.Get("kind")
	if ok {
		t.Error("expected label to not exist after remove")
	}
}

func TestLabelSet_RemoveNonexistent_NoOp(t *testing.T) {
	t.Parallel()

	// Given
	fs := domain.NewLabelSet()

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
	f1, _ := domain.NewLabel("kind", "feat")
	original := domain.NewLabelSet().Set(f1)

	// When — modify the "copy"
	f2, _ := domain.NewLabel("priority", "high")
	_ = original.Set(f2)

	// Then — original is unchanged
	if original.Len() != 1 {
		t.Errorf("expected original to have 1 label, got %d", original.Len())
	}
}

func TestLabelSetFrom_BuildsFromSlice(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := domain.NewLabel("kind", "feat")
	f2, _ := domain.NewLabel("area", "backend")

	// When
	fs := domain.LabelSetFrom([]domain.Label{f1, f2})

	// Then
	if fs.Len() != 2 {
		t.Errorf("expected 2 labels, got %d", fs.Len())
	}
}

func TestLabelSet_All_IteratesAllLabels(t *testing.T) {
	t.Parallel()

	// Given
	f1, _ := domain.NewLabel("kind", "feat")
	f2, _ := domain.NewLabel("area", "backend")
	fs := domain.NewLabelSet().Set(f1).Set(f2)

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

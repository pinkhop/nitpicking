package issue

import (
	"fmt"
	"iter"
	"maps"
	"unicode"
)

// Dimension represents a validated key–value pair attached to an issue for
// filtering and agent coordination.
type Label struct {
	key   string
	value string
}

// NewLabel creates a Dimension after validating both key and value.
//
// Key rules: 1–64 bytes, ASCII printable (0x21–0x7E), no whitespace, at
// least one alphanumeric character.
//
// Value rules: 1–256 bytes, free-form UTF-8, no whitespace, at least one
// alphanumeric character.
func NewLabel(key, value string) (Label, error) {
	if err := validateLabelKey(key); err != nil {
		return Label{}, err
	}
	if err := validateLabelValue(value); err != nil {
		return Label{}, err
	}
	return Label{key: key, value: value}, nil
}

// Key returns the dimension key.
func (f Label) Key() string { return f.key }

// Value returns the dimension value.
func (f Label) Value() string { return f.value }

// LabelSet is an ordered collection of dimensions with unique keys. Setting an
// existing key overwrites the previous value. LabelSet is immutable — all
// mutation methods return a new LabelSet.
type LabelSet struct {
	labels map[string]string
}

// NewLabelSet creates an empty LabelSet.
func NewLabelSet() LabelSet {
	return LabelSet{labels: make(map[string]string)}
}

// LabelSetFrom creates a LabelSet from a slice of Dimensions. If duplicate keys
// appear, the last value wins.
func LabelSetFrom(dimensions []Label) LabelSet {
	m := make(map[string]string, len(dimensions))
	for _, f := range dimensions {
		m[f.key] = f.value
	}
	return LabelSet{labels: m}
}

// Set returns a new LabelSet with the given dimension added or overwritten.
func (fs LabelSet) Set(f Label) LabelSet {
	next := maps.Clone(fs.labels)
	if next == nil {
		next = make(map[string]string)
	}
	next[f.key] = f.value
	return LabelSet{labels: next}
}

// Remove returns a new LabelSet with the given key removed. If the key does
// not exist, the returned set is identical.
func (fs LabelSet) Remove(key string) LabelSet {
	if _, ok := fs.labels[key]; !ok {
		return fs
	}
	next := maps.Clone(fs.labels)
	delete(next, key)
	return LabelSet{labels: next}
}

// Get returns the value for the given key and whether it exists.
func (fs LabelSet) Get(key string) (string, bool) {
	v, ok := fs.labels[key]
	return v, ok
}

// Len returns the number of dimensions in the set.
func (fs LabelSet) Len() int { return len(fs.labels) }

// All returns an iterator over all dimensions in the set. Iteration order is
// not guaranteed.
func (fs LabelSet) All() iter.Seq2[string, string] {
	return maps.All(fs.labels)
}

// validateLabelKey checks that a dimension key is 1–64 bytes of ASCII printable
// characters (0x21–0x7E) with at least one alphanumeric character.
func validateLabelKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("dimension key must not be empty")
	}
	if len(key) > 64 {
		return fmt.Errorf("dimension key must be at most 64 bytes, got %d", len(key))
	}

	hasAlphanumeric := false
	for _, r := range key {
		if r < 0x21 || r > 0x7E {
			return fmt.Errorf("dimension key contains non-printable or whitespace character %q", r)
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanumeric = true
		}
	}

	if !hasAlphanumeric {
		return fmt.Errorf("dimension key must contain at least one alphanumeric character")
	}
	return nil
}

// validateLabelValue checks that a dimension value is 1–256 bytes of UTF-8 text
// with no whitespace and at least one alphanumeric character.
func validateLabelValue(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("dimension value must not be empty")
	}
	if len(value) > 256 {
		return fmt.Errorf("dimension value must be at most 256 bytes, got %d", len(value))
	}

	hasAlphanumeric := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			return fmt.Errorf("dimension value must not contain whitespace")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanumeric = true
		}
	}

	if !hasAlphanumeric {
		return fmt.Errorf("dimension value must contain at least one alphanumeric character")
	}
	return nil
}

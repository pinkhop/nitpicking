package domain

import (
	"fmt"
	"iter"
	"maps"
	"unicode"
)

// Virtual label keys — convention: system/internal labels use hyphens
//
// Virtual labels are backed by dedicated columns on the issues table, not
// by rows in the labels table. They appear in label output and support
// filtering, but reads and writes are redirected to their columns.
//
// Naming convention: system/internal labels use hyphen-separated keys
// (e.g., "idempotency-key") to distinguish them from user-defined labels,
// which conventionally use colon-separated key:value pairs with short,
// alphanumeric keys (e.g., "kind:bug", "area:auth"). Hyphens are valid
// label key characters but are uncommon in user-defined keys, reducing
// collision risk. New virtual labels should follow the same pattern.
const VirtualKeyIdempotency = "idempotency-key"

// IsVirtualLabelKey reports whether the given label key is backed by a
// column on the issues table rather than the labels table.
func IsVirtualLabelKey(key string) bool {
	return key == VirtualKeyIdempotency
}

// Label represents a validated key–value pair attached to an issue for
// filtering and agent coordination.
type Label struct {
	key   string
	value string
}

// NewLabel creates a Label after validating both key and value.
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

// Key returns the label key.
func (f Label) Key() string { return f.key }

// Value returns the label value.
func (f Label) Value() string { return f.value }

// LabelSet is an ordered collection of labels with unique keys. Setting an
// existing key overwrites the previous value. LabelSet is immutable — all
// mutation methods return a new LabelSet.
type LabelSet struct {
	labels map[string]string
}

// NewLabelSet creates an empty LabelSet.
func NewLabelSet() LabelSet {
	return LabelSet{labels: make(map[string]string)}
}

// LabelSetFrom creates a LabelSet from a slice of Labels. If duplicate keys
// appear, the last value wins.
func LabelSetFrom(labels []Label) LabelSet {
	m := make(map[string]string, len(labels))
	for _, f := range labels {
		m[f.key] = f.value
	}
	return LabelSet{labels: m}
}

// Set returns a new LabelSet with the given label added or overwritten.
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

// Len returns the number of labels in the set.
func (fs LabelSet) Len() int { return len(fs.labels) }

// All returns an iterator over all labels in the set. Iteration order is
// not guaranteed.
func (fs LabelSet) All() iter.Seq2[string, string] {
	return maps.All(fs.labels)
}

// validateLabelKey checks that a label key is 1–64 bytes of ASCII printable
// characters (0x21–0x7E) with at least one alphanumeric character.
func validateLabelKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("label key must not be empty")
	}
	if len(key) > 64 {
		return fmt.Errorf("label key must be at most 64 bytes, got %d", len(key))
	}

	hasAlphanumeric := false
	for _, r := range key {
		if r < 0x21 || r > 0x7E {
			return fmt.Errorf("label key contains non-printable or whitespace character %q", r)
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanumeric = true
		}
	}

	if !hasAlphanumeric {
		return fmt.Errorf("label key must contain at least one alphanumeric character")
	}
	return nil
}

// validateLabelValue checks that a label value is 1–256 bytes of UTF-8 text
// with no whitespace and at least one alphanumeric character.
func validateLabelValue(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("label value must not be empty")
	}
	if len(value) > 256 {
		return fmt.Errorf("label value must be at most 256 bytes, got %d", len(value))
	}

	hasAlphanumeric := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			return fmt.Errorf("label value must not contain whitespace")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanumeric = true
		}
	}

	if !hasAlphanumeric {
		return fmt.Errorf("label value must contain at least one alphanumeric character")
	}
	return nil
}

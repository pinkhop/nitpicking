package domain

import (
	"fmt"
	"iter"
	"maps"
	"unicode"
)

// Label represents a validated key–value pair attached to an issue for
// filtering and agent coordination.
type Label struct {
	key   string
	value string
}

// NewLabel creates a Label after validating both key and value.
//
// Key rules: 1–64 bytes, ASCII printable (0x21–0x7E), no whitespace; the
// first character must be an ASCII letter (A-Z or a-z) or underscore (_).
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

// String returns the canonical "key:value" representation of the label.
func (f Label) String() string { return f.key + ":" + f.value }

// LabelCount pairs a label key-value combination with the number of non-deleted
// issues that carry it. It is used by the repository layer to communicate
// label usage frequency to the service layer so the service can compute
// per-key popularity rankings without embedding that logic in the storage layer.
type LabelCount struct {
	label Label
	count int
}

// NewLabelCount creates a LabelCount. The count must be positive; values of
// zero or below indicate a data inconsistency in the storage layer.
func NewLabelCount(key, value string, count int) (LabelCount, error) {
	lbl, err := NewLabel(key, value)
	if err != nil {
		return LabelCount{}, err
	}
	return LabelCount{label: lbl, count: count}, nil
}

// Key returns the label key.
func (lc LabelCount) Key() string { return lc.label.Key() }

// Value returns the label value.
func (lc LabelCount) Value() string { return lc.label.Value() }

// Count returns the number of non-deleted issues carrying this key-value pair.
func (lc LabelCount) Count() int { return lc.count }

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
// characters (0x21–0x7E) whose first character is an ASCII letter (A-Z or
// a-z) or an underscore (_). Interior and trailing characters may be any
// ASCII printable non-whitespace character, which preserves keys like
// "waiting-on" that use hyphens.
//
// The stricter first-character rule — letter or underscore only — prevents
// collision with CLI filter grammar (leading "!" means negation in
// ParseLabelFilters), avoids confusion with flag-style arguments, and aligns
// label keys with the C-style identifier convention that developers intuitively
// recognize as a "name".
func validateLabelKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("label key must not be empty")
	}
	if len(key) > 64 {
		return fmt.Errorf("label key must be at most 64 bytes, got %d", len(key))
	}

	// Validate first character: must be an ASCII letter or underscore.
	first := rune(key[0])
	if !isASCIILetter(first) && first != '_' {
		return fmt.Errorf("label key must start with a letter or underscore")
	}

	// Validate remaining characters: must be ASCII printable (0x21–0x7E).
	for _, r := range key[1:] {
		if r < 0x21 || r > 0x7E {
			return fmt.Errorf("label key contains non-printable or whitespace character %q", r)
		}
	}

	return nil
}

// isASCIILetter reports whether r is an ASCII letter (A-Z or a-z). It is
// intentionally stricter than unicode.IsLetter, which would accept non-ASCII
// letters such as "α". Only ASCII letters are valid as the first character of
// a label key to keep keys unambiguous across CLI grammars and encodings.
func isASCIILetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
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

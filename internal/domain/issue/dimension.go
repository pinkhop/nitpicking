package issue

import (
	"fmt"
	"iter"
	"maps"
	"unicode"
)

// Dimension represents a validated key–value pair attached to an issue for
// filtering and agent coordination.
type Dimension struct {
	key   string
	value string
}

// NewDimension creates a Dimension after validating both key and value.
//
// Key rules: 1–64 bytes, ASCII printable (0x21–0x7E), no whitespace, at
// least one alphanumeric character.
//
// Value rules: 1–256 bytes, free-form UTF-8, no whitespace, at least one
// alphanumeric character.
func NewDimension(key, value string) (Dimension, error) {
	if err := validateDimensionKey(key); err != nil {
		return Dimension{}, err
	}
	if err := validateDimensionValue(value); err != nil {
		return Dimension{}, err
	}
	return Dimension{key: key, value: value}, nil
}

// Key returns the dimension key.
func (f Dimension) Key() string { return f.key }

// Value returns the dimension value.
func (f Dimension) Value() string { return f.value }

// DimensionSet is an ordered collection of dimensions with unique keys. Setting an
// existing key overwrites the previous value. DimensionSet is immutable — all
// mutation methods return a new DimensionSet.
type DimensionSet struct {
	dimensions map[string]string
}

// NewDimensionSet creates an empty DimensionSet.
func NewDimensionSet() DimensionSet {
	return DimensionSet{dimensions: make(map[string]string)}
}

// DimensionSetFrom creates a DimensionSet from a slice of Dimensions. If duplicate keys
// appear, the last value wins.
func DimensionSetFrom(dimensions []Dimension) DimensionSet {
	m := make(map[string]string, len(dimensions))
	for _, f := range dimensions {
		m[f.key] = f.value
	}
	return DimensionSet{dimensions: m}
}

// Set returns a new DimensionSet with the given dimension added or overwritten.
func (fs DimensionSet) Set(f Dimension) DimensionSet {
	next := maps.Clone(fs.dimensions)
	if next == nil {
		next = make(map[string]string)
	}
	next[f.key] = f.value
	return DimensionSet{dimensions: next}
}

// Remove returns a new DimensionSet with the given key removed. If the key does
// not exist, the returned set is identical.
func (fs DimensionSet) Remove(key string) DimensionSet {
	if _, ok := fs.dimensions[key]; !ok {
		return fs
	}
	next := maps.Clone(fs.dimensions)
	delete(next, key)
	return DimensionSet{dimensions: next}
}

// Get returns the value for the given key and whether it exists.
func (fs DimensionSet) Get(key string) (string, bool) {
	v, ok := fs.dimensions[key]
	return v, ok
}

// Len returns the number of dimensions in the set.
func (fs DimensionSet) Len() int { return len(fs.dimensions) }

// All returns an iterator over all dimensions in the set. Iteration order is
// not guaranteed.
func (fs DimensionSet) All() iter.Seq2[string, string] {
	return maps.All(fs.dimensions)
}

// validateDimensionKey checks that a dimension key is 1–64 bytes of ASCII printable
// characters (0x21–0x7E) with at least one alphanumeric character.
func validateDimensionKey(key string) error {
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

// validateDimensionValue checks that a dimension value is 1–256 bytes of UTF-8 text
// with no whitespace and at least one alphanumeric character.
func validateDimensionValue(value string) error {
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

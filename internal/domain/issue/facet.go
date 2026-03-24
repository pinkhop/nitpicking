package issue

import (
	"fmt"
	"iter"
	"maps"
	"unicode"
)

// Facet represents a validated key–value pair attached to an issue for
// filtering and agent coordination.
type Facet struct {
	key   string
	value string
}

// NewFacet creates a Facet after validating both key and value.
//
// Key rules: 1–64 bytes, ASCII printable (0x21–0x7E), no whitespace, at
// least one alphanumeric character.
//
// Value rules: 1–256 bytes, free-form UTF-8, no whitespace, at least one
// alphanumeric character.
func NewFacet(key, value string) (Facet, error) {
	if err := validateFacetKey(key); err != nil {
		return Facet{}, err
	}
	if err := validateFacetValue(value); err != nil {
		return Facet{}, err
	}
	return Facet{key: key, value: value}, nil
}

// Key returns the facet key.
func (f Facet) Key() string { return f.key }

// Value returns the facet value.
func (f Facet) Value() string { return f.value }

// FacetSet is an ordered collection of facets with unique keys. Setting an
// existing key overwrites the previous value. FacetSet is immutable — all
// mutation methods return a new FacetSet.
type FacetSet struct {
	facets map[string]string
}

// NewFacetSet creates an empty FacetSet.
func NewFacetSet() FacetSet {
	return FacetSet{facets: make(map[string]string)}
}

// FacetSetFrom creates a FacetSet from a slice of Facets. If duplicate keys
// appear, the last value wins.
func FacetSetFrom(facets []Facet) FacetSet {
	m := make(map[string]string, len(facets))
	for _, f := range facets {
		m[f.key] = f.value
	}
	return FacetSet{facets: m}
}

// Set returns a new FacetSet with the given facet added or overwritten.
func (fs FacetSet) Set(f Facet) FacetSet {
	next := maps.Clone(fs.facets)
	if next == nil {
		next = make(map[string]string)
	}
	next[f.key] = f.value
	return FacetSet{facets: next}
}

// Remove returns a new FacetSet with the given key removed. If the key does
// not exist, the returned set is identical.
func (fs FacetSet) Remove(key string) FacetSet {
	if _, ok := fs.facets[key]; !ok {
		return fs
	}
	next := maps.Clone(fs.facets)
	delete(next, key)
	return FacetSet{facets: next}
}

// Get returns the value for the given key and whether it exists.
func (fs FacetSet) Get(key string) (string, bool) {
	v, ok := fs.facets[key]
	return v, ok
}

// Len returns the number of facets in the set.
func (fs FacetSet) Len() int { return len(fs.facets) }

// All returns an iterator over all facets in the set. Iteration order is
// not guaranteed.
func (fs FacetSet) All() iter.Seq2[string, string] {
	return maps.All(fs.facets)
}

// validateFacetKey checks that a facet key is 1–64 bytes of ASCII printable
// characters (0x21–0x7E) with at least one alphanumeric character.
func validateFacetKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("facet key must not be empty")
	}
	if len(key) > 64 {
		return fmt.Errorf("facet key must be at most 64 bytes, got %d", len(key))
	}

	hasAlphanumeric := false
	for _, r := range key {
		if r < 0x21 || r > 0x7E {
			return fmt.Errorf("facet key contains non-printable or whitespace character %q", r)
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanumeric = true
		}
	}

	if !hasAlphanumeric {
		return fmt.Errorf("facet key must contain at least one alphanumeric character")
	}
	return nil
}

// validateFacetValue checks that a facet value is 1–256 bytes of UTF-8 text
// with no whitespace and at least one alphanumeric character.
func validateFacetValue(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("facet value must not be empty")
	}
	if len(value) > 256 {
		return fmt.Errorf("facet value must be at most 256 bytes, got %d", len(value))
	}

	hasAlphanumeric := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			return fmt.Errorf("facet value must not contain whitespace")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanumeric = true
		}
	}

	if !hasAlphanumeric {
		return fmt.Errorf("facet value must contain at least one alphanumeric character")
	}
	return nil
}

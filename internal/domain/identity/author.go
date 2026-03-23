package identity

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// maxAuthorRunes is the maximum number of Unicode runes allowed in an author
// string, measured after NFC normalization.
const maxAuthorRunes = 64

// Author represents a validated, NFC-normalized author identifier. Authors
// are case-sensitive — "alice" and "Alice" are distinct. An Author is
// immutable after construction.
type Author struct {
	value string
}

// NewAuthor validates and NFC-normalizes the given string into an Author.
//
// Validation rules (per §4.8):
//   - At least one alphanumeric character.
//   - Maximum 64 Unicode runes (measured after normalization).
//   - No whitespace — no Unicode whitespace characters.
func NewAuthor(s string) (Author, error) {
	normalized := norm.NFC.String(s)

	if utf8.RuneCountInString(normalized) > maxAuthorRunes {
		return Author{}, fmt.Errorf("author must be at most %d runes, got %d", maxAuthorRunes, utf8.RuneCountInString(normalized))
	}

	hasAlphanumeric := false
	for _, r := range normalized {
		if unicode.IsSpace(r) {
			return Author{}, fmt.Errorf("author must not contain whitespace")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphanumeric = true
		}
	}

	if !hasAlphanumeric {
		return Author{}, fmt.Errorf("author must contain at least one alphanumeric character")
	}

	return Author{value: normalized}, nil
}

// String returns the NFC-normalized author string.
func (a Author) String() string { return a.value }

// IsZero reports whether the Author is the zero value (uninitialized).
func (a Author) IsZero() bool { return a.value == "" }

// Equal reports whether two authors are identical. Comparison is
// case-sensitive on the NFC-normalized form.
func (a Author) Equal(other Author) bool { return a.value == other.value }

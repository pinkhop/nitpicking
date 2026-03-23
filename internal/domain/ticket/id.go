package ticket

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"unicode"
)

// crockfordAlphabet is the lowercase Crockford Base32 encoding alphabet.
// It excludes i, l, o, u to avoid visual ambiguity and accidental profanity.
const crockfordAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// randomPartLength is the number of random Crockford Base32 characters in a
// ticket ID.
const randomPartLength = 5

// idSpace is the total number of possible random portions (32^5 = 33,554,432).
const idSpace = 32 * 32 * 32 * 32 * 32

// ID represents a ticket identifier in the form PREFIX-random (e.g., "NP-a3bxr").
// The prefix is uppercase ASCII letters; the random portion is lowercase
// Crockford Base32 characters. IDs are immutable after construction.
type ID struct {
	prefix string
	random string
}

// ParseID parses a string of the form "PREFIX-random" into an ID.
// It validates that the prefix is 1–10 uppercase ASCII letters and the
// random portion is exactly 5 lowercase Crockford Base32 characters.
func ParseID(s string) (ID, error) {
	idx := strings.IndexByte(s, '-')
	if idx < 0 {
		return ID{}, fmt.Errorf("invalid ticket ID %q: missing separator", s)
	}

	prefix := s[:idx]
	random := s[idx+1:]

	if err := ValidatePrefix(prefix); err != nil {
		return ID{}, fmt.Errorf("invalid ticket ID %q: %w", s, err)
	}

	if err := validateRandom(random); err != nil {
		return ID{}, fmt.Errorf("invalid ticket ID %q: %w", s, err)
	}

	return ID{prefix: prefix, random: random}, nil
}

// GenerateID creates a new random ticket ID with the given prefix. The
// collisionCheck callback returns true if the generated ID already exists;
// on collision, the function regenerates and retries up to maxRetries times.
func GenerateID(prefix string, collisionCheck func(ID) (bool, error)) (ID, error) {
	if err := ValidatePrefix(prefix); err != nil {
		return ID{}, err
	}

	const maxRetries = 10
	for range maxRetries {
		random := generateRandom()
		id := ID{prefix: prefix, random: random}

		if collisionCheck != nil {
			exists, err := collisionCheck(id)
			if err != nil {
				return ID{}, fmt.Errorf("checking ID collision: %w", err)
			}
			if exists {
				continue
			}
		}

		return id, nil
	}

	return ID{}, fmt.Errorf("failed to generate unique ticket ID after %d attempts", maxRetries)
}

// String returns the canonical string representation: PREFIX-random.
func (id ID) String() string {
	return id.prefix + "-" + id.random
}

// Prefix returns the uppercase prefix portion of the ID.
func (id ID) Prefix() string { return id.prefix }

// Random returns the lowercase random portion of the ID.
func (id ID) Random() string { return id.random }

// IsZero reports whether the ID is the zero value (uninitialized).
func (id ID) IsZero() bool { return id.prefix == "" && id.random == "" }

// ValidatePrefix checks that a prefix is 1–10 uppercase ASCII letters.
func ValidatePrefix(prefix string) error {
	if len(prefix) == 0 {
		return fmt.Errorf("prefix must not be empty")
	}
	if len(prefix) > 10 {
		return fmt.Errorf("prefix must be at most 10 characters, got %d", len(prefix))
	}
	for _, r := range prefix {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("prefix must contain only uppercase ASCII letters, got %q", r)
		}
	}
	return nil
}

// validateRandom checks that a random portion is exactly 5 lowercase Crockford
// Base32 characters.
func validateRandom(random string) error {
	if len(random) != randomPartLength {
		return fmt.Errorf("random portion must be exactly %d characters, got %d", randomPartLength, len(random))
	}
	for _, r := range random {
		if !isCrockfordChar(r) {
			return fmt.Errorf("random portion contains invalid character %q", r)
		}
	}
	return nil
}

// isCrockfordChar reports whether r is a valid lowercase Crockford Base32
// character.
func isCrockfordChar(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'z' {
		// Crockford excludes: i, l, o, u
		return r != 'i' && r != 'l' && r != 'o' && r != 'u'
	}
	return false
}

// generateRandom produces a cryptographically random 5-character Crockford
// Base32 string. Uses crypto/rand/v2 which panics on CSPRNG failure — the
// correct behavior since a broken random source should halt the process.
func generateRandom() string {
	val := rand.N(idSpace)
	buf := make([]byte, randomPartLength)
	for i := randomPartLength - 1; i >= 0; i-- {
		buf[i] = crockfordAlphabet[val%32]
		val /= 32
	}
	return string(buf)
}

// containsAlphanumeric reports whether s contains at least one Unicode letter
// or digit. Used for title and author validation.
func containsAlphanumeric(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

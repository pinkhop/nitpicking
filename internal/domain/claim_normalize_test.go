package domain_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestHashClaimID_NormalizesConfusableCharacters(t *testing.T) {
	t.Parallel()

	// Given — a canonical token and a confusable variant that should
	// normalize to the same value before hashing.
	canonical := "10a10b00c11d0e1f0g1h0j1k10"
	confusable := "LOaLObOOcLLdOeLfOgLhOjLkLO" // L→1, O→0, uppercase letters→lowercase

	// When
	canonicalHash := domain.HashClaimID(canonical)
	confusableHash := domain.HashClaimID(confusable)

	// Then — both hashes must be identical because normalization maps them
	// to the same canonical form before hashing.
	if canonicalHash != confusableHash {
		t.Errorf("expected identical hashes:\n  canonical:  %s\n  confusable: %s", canonicalHash, confusableHash)
	}
}

func TestHashClaimID_UppercaseToken_MatchesLowercaseHash(t *testing.T) {
	t.Parallel()

	// Given
	lower := "0a1b2c3d4e5f6g7h8j9k0a1b2c"
	upper := "0A1B2C3D4E5F6G7H8J9K0A1B2C"

	// When
	lowerHash := domain.HashClaimID(lower)
	upperHash := domain.HashClaimID(upper)

	// Then
	if lowerHash != upperHash {
		t.Errorf("expected case-insensitive hashing:\n  lower: %s\n  upper: %s", lowerHash, upperHash)
	}
}

func TestNewClaim_TokenHashMatchesNormalizedHash(t *testing.T) {
	t.Parallel()

	// Given — a freshly created claim
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — hash the token with confusable substitutions applied to a copy.
	// Since generated tokens are already canonical, uppercasing some letters
	// and then hashing should still match because HashClaimID normalizes.
	token := c.Token()
	// Uppercase the entire token to simulate user providing uppercase input.
	uppercased := make([]byte, len(token))
	for i, b := range []byte(token) {
		if b >= 'a' && b <= 'z' {
			uppercased[i] = b - 32 // ASCII uppercase
		} else {
			uppercased[i] = b
		}
	}

	normalizedHash := domain.HashClaimID(string(uppercased))

	// Then — must match the claim's stored ID
	if c.ID() != normalizedHash {
		t.Errorf("uppercase token hash %q does not match claim ID %q", normalizedHash, c.ID())
	}
}

package domain

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"time"
)

// DefaultStaleThreshold is the default duration after which a claim becomes
// stale and eligible for stealing.
const DefaultStaleThreshold = 2 * time.Hour

// MaxStaleThreshold is the maximum allowed stale threshold.
const MaxStaleThreshold = 24 * time.Hour

// claimIDLength is the number of Crockford Base32 characters in a claim ID.
// Each character encodes 5 bits; 26 characters encode 130 bits, of which
// 128 are random (the leading character uses only 3 of its 5 bits).
// 128 bits matches UUID-level uniqueness.
const claimIDLength = 26

// Claim represents active ownership of an issue. Claims are immutable value
// objects — "extending" or "updating stale deadline" produces a new Claim.
//
// A claim carries two identifiers: the token (Crockford Base32 string
// returned to the caller) and the hash ID (SHA-512 hash of the token's
// raw bytes, hex-encoded, used for database storage and lookups). For
// freshly created claims, both are populated. For claims reconstructed
// from the database, only the hash ID is available — the plaintext
// token is not recoverable.
type Claim struct {
	id        string // SHA-512 hash of the token bytes (hex-encoded).
	token     string // Crockford Base32 token; empty for reconstructed claims.
	issueID   ID
	author    Author
	claimedAt time.Time
	staleAt   time.Time
}

// NewClaimParams holds the parameters for creating a new claim.
type NewClaimParams struct {
	IssueID       ID
	Author        Author
	StaleDuration time.Duration
	// StaleAt is an optional absolute timestamp at which the claim becomes
	// stale. When non-zero, it takes precedence over StaleDuration. The
	// caller is responsible for ensuring StaleAt is in the future and within
	// MaxStaleThreshold of Now.
	StaleAt time.Time
	Now     time.Time
}

// NewClaim creates a new claim with a randomly generated claim ID.
func NewClaim(p NewClaimParams) (Claim, error) {
	if p.IssueID.IsZero() {
		return Claim{}, fmt.Errorf("claim requires an issue ID")
	}
	if p.Author.IsZero() {
		return Claim{}, fmt.Errorf("claim requires an author")
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now()
	}

	var staleAt time.Time
	if !p.StaleAt.IsZero() {
		// Absolute stale-at takes precedence over duration. The caller
		// is responsible for validating that StaleAt is in the future
		// and within MaxStaleThreshold of now; the domain enforces only
		// the max-distance invariant.
		if p.StaleAt.Sub(now) > MaxStaleThreshold {
			return Claim{}, fmt.Errorf("stale-at %v is more than %v from now", p.StaleAt.Format(time.RFC3339), MaxStaleThreshold)
		}
		staleAt = p.StaleAt
	} else {
		duration := p.StaleDuration
		if duration == 0 {
			duration = DefaultStaleThreshold
		}
		if duration > MaxStaleThreshold {
			return Claim{}, fmt.Errorf("stale duration %v exceeds maximum %v", duration, MaxStaleThreshold)
		}
		staleAt = now.Add(duration)
	}

	token := generateClaimID()
	hashID := HashClaimID(token)

	return Claim{
		id:        hashID,
		token:     token,
		issueID:   p.IssueID,
		author:    p.Author,
		claimedAt: now,
		staleAt:   staleAt,
	}, nil
}

// ReconstructClaim rebuilds a Claim from persisted data without generating
// a new ID. Used by the storage layer when loading claims from the database.
func ReconstructClaim(id string, issueID ID, author Author, claimedAt time.Time, staleAt time.Time) Claim {
	return Claim{
		id:        id,
		issueID:   issueID,
		author:    author,
		claimedAt: claimedAt,
		staleAt:   staleAt,
	}
}

// ID returns the SHA-512 hash of the claim token, hex-encoded. This is the
// value stored in the database and used for all persistence operations.
func (c Claim) ID() string { return c.id }

// Token returns the Crockford Base32 claim token. This is the bearer
// credential returned to the caller who created the claim. For claims
// reconstructed from the database, Token returns an empty string because
// the plaintext is not stored.
func (c Claim) Token() string { return c.token }

// IssueID returns the ID of the claimed issue.
func (c Claim) IssueID() ID { return c.issueID }

// Author returns the author who holds the claim.
func (c Claim) Author() Author { return c.author }

// IsStale reports whether the claim is stale at the given time.
func (c Claim) IsStale(now time.Time) bool {
	return now.After(c.staleAt)
}

// ClaimedAt returns the timestamp when this claim was created.
func (c Claim) ClaimedAt() time.Time { return c.claimedAt }

// StaleAt returns the timestamp at which this claim becomes stale.
func (c Claim) StaleAt() time.Time {
	return c.staleAt
}

// WithStaleAt returns a new Claim with the staleAt timestamp updated.
// Used by adapters to extend claim lifetimes (e.g., when updating an issue
// pushes the stale deadline forward).
func (c Claim) WithStaleAt(t time.Time) Claim {
	c.staleAt = t
	return c
}

// HashClaimID computes the SHA-512 hash of a claim token after Crockford
// Base32 normalization. The token is normalized (lowercased, with I/L→1 and
// O→0 substitutions applied) before hashing, so that confusable variants of
// the same token produce the same hash. Returns the hex-encoded hash string.
func HashClaimID(token string) string {
	normalized := NormalizeCrockford(token)
	hash := sha512.Sum512([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// generateClaimID produces a cryptographically random claim ID encoded as
// lowercase Crockford Base32. Uses math/rand/v2 which is backed by
// crypto/rand by default in Go 1.22+. Each character encodes 5 bits of
// entropy; 26 characters encode 130 bits, of which 128 are random (the
// leading character uses only 3 of its 5 bits).
func generateClaimID() string {
	hi := rand.Uint64() // #nosec G404 -- math/rand/v2 is backed by crypto/rand by default in Go 1.22+
	lo := rand.Uint64() // #nosec G404 -- see comment above

	buf := make([]byte, claimIDLength)

	// Extract 5-bit groups right-to-left from lo (12 full groups = 60 bits),
	// then the boundary group spanning lo and hi, then from hi.
	for i := claimIDLength - 1; i >= 14; i-- {
		buf[i] = crockfordAlphabet[lo&0x1f]
		lo >>= 5
	}
	// lo has 4 remaining bits. Combine with 1 bit from hi for character 13.
	buf[13] = crockfordAlphabet[(lo&0x0f)|((hi&0x01)<<4)]
	hi >>= 1

	// hi now has 63 bits. Extract 12 full 5-bit groups (60 bits).
	for i := 12; i >= 1; i-- {
		buf[i] = crockfordAlphabet[hi&0x1f]
		hi >>= 5
	}
	// hi has 3 remaining bits for the leading character.
	buf[0] = crockfordAlphabet[hi&0x07]

	return string(buf)
}
